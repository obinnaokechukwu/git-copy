package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func cmdInstall(uninstall bool) error {
	if uninstall {
		return doUninstall()
	}
	return doInstall()
}

func doInstall() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	switch runtime.GOOS {
	case "linux":
		return installLinuxSystemd(exePath)
	case "darwin":
		return installMacOSLaunchd(exePath)
	default:
		return fmt.Errorf("automatic installation not supported on %s; run 'git-copy serve' manually", runtime.GOOS)
	}
}

func doUninstall() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallLinuxSystemd()
	case "darwin":
		return uninstallMacOSLaunchd()
	default:
		return fmt.Errorf("automatic uninstallation not supported on %s", runtime.GOOS)
	}
}

const systemdServiceName = "git-copy.service"
const systemdUserDir = ".config/systemd/user"

func installLinuxSystemd(exePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	serviceDir := filepath.Join(home, systemdUserDir)
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return fmt.Errorf("failed to create systemd user directory: %w", err)
	}

	servicePath := filepath.Join(serviceDir, systemdServiceName)
	serviceContent := fmt.Sprintf(`[Unit]
Description=git-copy daemon - scrubbed git repo sync
After=network.target

[Service]
Type=simple
ExecStart=%s serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`, exePath)

	if err := os.WriteFile(servicePath, []byte(serviceContent), 0o644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	fmt.Printf("Created systemd user service: %s\n", servicePath)

	// Reload and enable
	if err := runCmd("systemctl", "--user", "daemon-reload"); err != nil {
		fmt.Printf("Warning: failed to reload systemd: %v\n", err)
	}
	if err := runCmd("systemctl", "--user", "enable", systemdServiceName); err != nil {
		fmt.Printf("Warning: failed to enable service: %v\n", err)
	}
	if err := runCmd("systemctl", "--user", "start", systemdServiceName); err != nil {
		fmt.Printf("Warning: failed to start service: %v\n", err)
	}

	fmt.Println()
	fmt.Println("git-copy daemon installed and started.")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  systemctl --user status git-copy    # Check status")
	fmt.Println("  systemctl --user stop git-copy      # Stop daemon")
	fmt.Println("  systemctl --user restart git-copy   # Restart daemon")
	fmt.Println("  journalctl --user -u git-copy -f    # View logs")
	fmt.Println()
	fmt.Println("To uninstall: git-copy install --uninstall")

	return nil
}

func uninstallLinuxSystemd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	servicePath := filepath.Join(home, systemdUserDir, systemdServiceName)

	// Stop and disable
	_ = runCmd("systemctl", "--user", "stop", systemdServiceName)
	_ = runCmd("systemctl", "--user", "disable", systemdServiceName)

	if err := os.Remove(servicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	_ = runCmd("systemctl", "--user", "daemon-reload")

	fmt.Println("git-copy daemon uninstalled.")
	return nil
}

const launchdPlistName = "com.obinnaokechukwu.git-copy.plist"

func installMacOSLaunchd(exePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	plistPath := filepath.Join(launchAgentsDir, launchdPlistName)
	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.obinnaokechukwu.git-copy</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>serve</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s/Library/Logs/git-copy.log</string>
    <key>StandardErrorPath</key>
    <string>%s/Library/Logs/git-copy.err</string>
</dict>
</plist>
`, exePath, home, home)

	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		return fmt.Errorf("failed to write plist file: %w", err)
	}

	fmt.Printf("Created launchd plist: %s\n", plistPath)

	// Load the service
	if err := runCmd("launchctl", "load", plistPath); err != nil {
		fmt.Printf("Warning: failed to load service: %v\n", err)
	}

	fmt.Println()
	fmt.Println("git-copy daemon installed and started.")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  launchctl list | grep git-copy              # Check if running")
	fmt.Println("  launchctl unload ~/Library/LaunchAgents/" + launchdPlistName + "  # Stop")
	fmt.Println("  launchctl load ~/Library/LaunchAgents/" + launchdPlistName + "    # Start")
	fmt.Println("  tail -f ~/Library/Logs/git-copy.log         # View logs")
	fmt.Println()
	fmt.Println("To uninstall: git-copy install --uninstall")

	return nil
}

func uninstallMacOSLaunchd() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchdPlistName)

	// Unload the service
	_ = runCmd("launchctl", "unload", plistPath)

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist file: %w", err)
	}

	fmt.Println("git-copy daemon uninstalled.")
	return nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func isRoot() bool {
	return os.Geteuid() == 0
}

func which(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func hasSystemd() bool {
	return which("systemctl") != "" && strings.Contains(runCmdOutput("ps", "-p", "1", "-o", "comm="), "systemd")
}

func runCmdOutput(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}
