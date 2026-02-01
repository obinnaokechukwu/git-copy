package notify

import (
	"os/exec"
	"runtime"
)

func Error(title, message string) {
	if runtime.GOOS != "linux" {
		return
	}
	if _, err := exec.LookPath("notify-send"); err != nil {
		return
	}
	// Best-effort
	_ = exec.Command("notify-send", title, message).Run()
}
