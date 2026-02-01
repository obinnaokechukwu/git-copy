package notify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNotify_Error_BestEffort(t *testing.T) {
	tmp := t.TempDir()
	if runtime.GOOS == "linux" {
		script := filepath.Join(tmp, "notify-send")
		out := filepath.Join(tmp, "called.txt")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s %s\n' \"$1\" \"$2\" >> \""+out+"\"\n"), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		t.Setenv("PATH", tmp)
		Error("title", "message")
		if _, err := os.Stat(out); err != nil {
			t.Fatalf("expected notify-send to be invoked; stat: %v", err)
		}
	} else {
		Error("title", "message")
	}
}
