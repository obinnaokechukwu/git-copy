package cli

import (
	"flag"
	"io"
)

type syncArgs struct {
	repo        string
	target      string
	audit       bool
	auditRemote bool
}

func parseSyncArgs(args []string) (syncArgs, error) {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var s syncArgs
	fs.StringVar(&s.repo, "repo", "", "path to repo (default: current directory)")
	fs.StringVar(&s.target, "target", "", "sync only this target label")
	fs.BoolVar(&s.audit, "audit", true, "audit the scrubbed output after a successful sync")
	fs.BoolVar(&s.auditRemote, "audit-remote", false, "also audit the remote mirror by cloning it (implies --audit)")

	if err := fs.Parse(args); err != nil {
		return syncArgs{}, err
	}
	if s.auditRemote {
		s.audit = true
	}
	return s, nil
}
