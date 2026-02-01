package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var stdin = bufio.NewReader(os.Stdin)

func promptString(message string, def string, required bool) (string, error) {
	for {
		if def != "" {
			fmt.Printf("%s [%s]: ", message, def)
		} else {
			fmt.Printf("%s: ", message)
		}
		line, err := stdin.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			line = def
		}
		if required && strings.TrimSpace(line) == "" {
			fmt.Println("Value is required.")
			continue
		}
		return line, nil
	}
}

func promptConfirm(message string, def bool) (bool, error) {
	defStr := "y/N"
	if def {
		defStr = "Y/n"
	}
	for {
		fmt.Printf("%s [%s]: ", message, defStr)
		line, err := stdin.ReadString('\n')
		if err != nil {
			return false, err
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			return def, nil
		}
		if line == "y" || line == "yes" {
			return true, nil
		}
		if line == "n" || line == "no" {
			return false, nil
		}
		fmt.Println("Please answer y or n.")
	}
}

func promptSelect(message string, options []string, defIdx int) (string, error) {
	if defIdx < 0 || defIdx >= len(options) {
		defIdx = 0
	}
	for {
		fmt.Println(message)
		for i, opt := range options {
			m := " "
			if i == defIdx {
				m = "*"
			}
			fmt.Printf("  %s %d) %s\n", m, i+1, opt)
		}
		fmt.Printf("Select [default %d]: ", defIdx+1)
		line, err := stdin.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return options[defIdx], nil
		}
		n, err := strconv.Atoi(line)
		if err != nil || n < 1 || n > len(options) {
			fmt.Println("Invalid selection.")
			continue
		}
		return options[n-1], nil
	}
}

// Not hidden; prefer env vars.
func promptSecret(message string, required bool) (string, error) {
	return promptString(message, "", required)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
