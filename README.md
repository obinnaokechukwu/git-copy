# git-copy

Scrubbed one-way replication from private Git repos to public targets.

## Overview

`git-copy` is a CLI tool that safely synchronizes Git repositories from private sources to public targets (GitHub, GitLab, Gitea, etc.) while automatically scrubbing sensitive information. It rewrites Git history to replace private usernames, exclude sensitive files, and apply custom text replacements.

## Features

- **Automatic Scrubbing**: Replaces private usernames and sensitive strings throughout Git history
- **File Exclusion**: Exclude files by pattern (e.g., `.env`, `secrets/**`, etc.)
- **Opt-In Override**: Selectively include files that would otherwise be excluded
- **Author Rewriting**: Replace commit author information with public identities
- **Multi-Target**: Sync to multiple destinations (GitHub, GitLab, Gitea)
- **Daemon Mode**: Automatic syncing across multiple repositories
- **Safe by Default**: Validates scrubbed repos before pushing (blocks `.env`, `CLAUDE.md`, etc.)
- **Efficient**: Uses `git fast-export`/`fast-import` for fast history rewriting

## Installation

```bash
go install github.com/obinnaokechukwu/git-copy/cmd/git-copy@latest
```

Or build from source:

```bash
git clone https://github.com/obinnaokechukwu/git-copy
cd git-copy
go build -o git-copy ./cmd/git-copy
```

## Quick Start

### 1. Initialize a Repository

```bash
cd /path/to/your/private/repo
git-copy init
```

This creates a `.git-copy/config.json` file with your scrubbing rules.

### 2. Add a Sync Target

```bash
git-copy add-target
```

Follow the interactive prompts to configure:
- Target label (e.g., "github-public")
- Provider (github, gitlab, gitea)
- Account/organization name
- Repository name
- Authentication credentials

### 3. Sync to Target

```bash
git-copy sync
```

This will:
1. Export your Git history
2. Apply scrubbing rules (replace usernames, exclude files)
3. Validate the scrubbed repo
4. Push to the configured target(s)

## Configuration

The `.git-copy/config.json` file controls scrubbing behavior:

```json
{
  "private_username": "myPrivateUsername",
  "defaults": {
    "exclude": [
      ".env",
      "secrets/**",
      "*.key"
    ],
    "opt_in": [],
    "extra_replacements": {
      "company-internal.example.com": "public.example.com"
    }
  },
  "targets": [
    {
      "label": "github-public",
      "provider": "github",
      "account": "my-public-account",
      "repo_name": "my-public-repo",
      "replacement": "PublicName",
      "public_author_name": "Public Name",
      "public_author_email": "public@example.com"
    }
  ]
}
```

### Configuration Fields

- **`private_username`**: Your private username to be replaced in all text/commits
- **`defaults.exclude`**: File patterns to exclude (glob syntax, `**` supported)
- **`defaults.opt_in`**: Override exclusions for specific files
- **`defaults.extra_replacements`**: Additional string replacements (old → new)
- **`targets[].label`**: Unique identifier for this sync target
- **`targets[].provider`**: `github`, `gitlab`, or `gitea`
- **`targets[].account`**: Target account/organization
- **`targets[].repo_name`**: Target repository name
- **`targets[].replacement`**: String to replace `private_username` with
- **`targets[].public_author_name`**: Name for rewritten commits
- **`targets[].public_author_email`**: Email for rewritten commits

## Commands

### Repository Commands

```bash
# Initialize git-copy in current repo
git-copy init [--repo PATH]

# Add a new sync target interactively
git-copy add-target [--repo PATH]

# Remove a sync target
git-copy remove-target <label> [--repo PATH]

# List configured targets
git-copy list-targets [--repo PATH]

# Sync to all targets (or specific target)
git-copy sync [--repo PATH] [--target LABEL]

# Show sync status
git-copy status [--repo PATH]
```

### Daemon Commands

The daemon watches multiple repositories and syncs automatically:

```bash
# Add a root directory to watch
git-copy roots add /path/to/repos

# Remove a root directory
git-copy roots remove /path/to/repos

# List watched roots
git-copy roots list

# List discovered repositories
git-copy repos

# Start the daemon
git-copy serve
```

## How It Works

1. **Fast Export**: Uses `git fast-export` to stream the entire Git history
2. **Streaming Filter**: Processes each commit, blob, and ref in the stream
3. **Scrubbing**:
   - Replaces `private_username` with `replacement` in all text
   - Applies `extra_replacements`
   - Excludes files matching `exclude` patterns (unless in `opt_in`)
   - Rewrites author/committer information
4. **Fast Import**: Imports the scrubbed stream into a temporary bare repo
5. **Validation**: Checks for leaked private username or forbidden files
6. **Push Mirror**: Force-pushes all refs to the target repository

## Safety Features

- **Validation**: Automatically validates scrubbed repos for:
  - Presence of private username in any file
  - Forbidden files (`.env`, `CLAUDE.md` by default)
- **Non-negotiable Exclusions**: `.git-copy/**` is always excluded
- **Opt-In Override**: Files in `opt_in` bypass `exclude` patterns
- **Author Protection**: Rewrites commit authors to prevent identity leakage
- **Atomic Updates**: Uses temporary repos and atomic rename for safe caching

## Use Cases

- **Open-sourcing Private Repos**: Scrub internal references before making code public
- **Multi-Account Publishing**: Maintain one private repo, sync to multiple public accounts
- **Compliance**: Ensure sensitive files never reach public repositories
- **Brand Consistency**: Replace internal names with public branding
- **Personal Privacy**: Separate work identity from public contributions

## Development

```bash
# Build
go build -o git-copy ./cmd/git-copy

# Run tests
go test ./...

# Install locally
go install ./cmd/git-copy
```

## Project Structure

```
git-copy/
├── cmd/git-copy/          # Main CLI entry point
├── internal/
│   ├── cli/               # Command implementations
│   ├── config/            # Configuration handling
│   ├── daemon/            # Daemon mode (auto-sync)
│   ├── git/               # Git operations wrapper
│   ├── notify/            # File system watching
│   ├── provider/          # GitHub/GitLab/Gitea clients
│   ├── repo/              # Repository discovery
│   ├── scrub/             # History scrubbing logic
│   ├── state/             # Sync state tracking
│   └── sync/              # Sync orchestration
└── README.md
```

## License

[Specify your license here]

## Contributing

Contributions welcome! Please open an issue or submit a pull request.

## Security

**Important**: Always review the scrubbed repository before syncing to ensure no sensitive data leaks. While `git-copy` includes validation, it's not foolproof.

- Test with `--repo` flag on a copy first
- Review `.git-copy/config.json` carefully
- Check excluded patterns cover all sensitive paths
- Verify `extra_replacements` catches domain-specific secrets
