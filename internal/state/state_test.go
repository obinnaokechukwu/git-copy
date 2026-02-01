package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestState_LoadMissingReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	s, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.Targets) != 0 {
		t.Fatalf("expected empty targets, got %#v", s.Targets)
	}
}

func TestState_SaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	s := RepoState{
		Targets: map[string]*TargetState{
			"t1": {LastSyncAt: time.Now(), LastError: "", LastPrivateRefs: "abc"},
		},
	}
	if err := Save(tmp, s); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s2.Targets["t1"] == nil || s2.Targets["t1"].LastPrivateRefs != "abc" {
		t.Fatalf("roundtrip mismatch: %#v", s2.Targets["t1"])
	}
	if filepath.Base(StatePath(tmp)) != "state.json" {
		t.Fatalf("unexpected state path: %s", StatePath(tmp))
	}
}
