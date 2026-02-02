package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type RepoState struct {
	UpdatedAt time.Time               `json:"updated_at"`
	Targets   map[string]*TargetState `json:"targets"`
}

type TargetState struct {
	LastSyncAt      time.Time `json:"last_sync_at"`
	LastError       string    `json:"last_error,omitempty"`
	LastPrivateRefs string    `json:"last_private_refs,omitempty"` // hash of refs snapshot
	LastPublicPush  string    `json:"last_public_push,omitempty"`  // hash of refs snapshot from scrubbed repo
	LastConfigHash  string    `json:"last_config_hash,omitempty"`  // hash of config affecting scrubbing/push
}

func StatePath(repoPath string) string {
	return filepath.Join(repoPath, ".git-copy", "state.json")
}

func Load(repoPath string) (RepoState, error) {
	p := StatePath(repoPath)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return RepoState{Targets: map[string]*TargetState{}}, nil
		}
		return RepoState{}, err
	}
	var s RepoState
	if err := json.Unmarshal(b, &s); err != nil {
		return RepoState{}, err
	}
	if s.Targets == nil {
		s.Targets = map[string]*TargetState{}
	}
	return s, nil
}

func Save(repoPath string, s RepoState) error {
	s.UpdatedAt = time.Now()
	if s.Targets == nil {
		s.Targets = map[string]*TargetState{}
	}
	p := StatePath(repoPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(&s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}
