package cli

import "testing"

func TestParseSyncArgs_Defaults(t *testing.T) {
	a, err := parseSyncArgs([]string{})
	if err != nil {
		t.Fatalf("parseSyncArgs: %v", err)
	}
	if a.repo != "" || a.target != "" {
		t.Fatalf("unexpected repo/target defaults: repo=%q target=%q", a.repo, a.target)
	}
	if !a.audit {
		t.Fatalf("expected audit default true")
	}
	if a.auditRemote {
		t.Fatalf("expected auditRemote default false")
	}
}

func TestParseSyncArgs_AuditDisabled(t *testing.T) {
	a, err := parseSyncArgs([]string{"--audit=false"})
	if err != nil {
		t.Fatalf("parseSyncArgs: %v", err)
	}
	if a.audit {
		t.Fatalf("expected audit false")
	}
}

func TestParseSyncArgs_AuditRemoteImpliesAudit(t *testing.T) {
	a, err := parseSyncArgs([]string{"--audit=false", "--audit-remote"})
	if err != nil {
		t.Fatalf("parseSyncArgs: %v", err)
	}
	if !a.audit {
		t.Fatalf("expected audit true when audit-remote is set")
	}
	if !a.auditRemote {
		t.Fatalf("expected auditRemote true")
	}
}
