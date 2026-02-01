package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	"github.com/obinnaokechukwu/git-copy/internal/provider"
)

func interactiveTargetSetup(cfg config.RepoConfig, repoPath string) (config.Target, error) {
	// Load global preferences for account email defaults
	globalPrefs := config.LoadGlobalPrefs()

	provChoice, err := promptSelect("Target hosting provider:", []string{
		"github", "gitlab", "gitea/forgejo", "custom (existing repo)",
	}, 0)
	if err != nil {
		return config.Target{}, err
	}

	// Default label to provider name
	defaultLabel := strings.Split(provChoice, "/")[0] // "gitea/forgejo" -> "gitea"
	if defaultLabel == "custom (existing repo)" {
		defaultLabel = "custom"
	}
	label, err := promptString("Target label (alias used in commands)", defaultLabel, true)
	if err != nil {
		return config.Target{}, err
	}
	label = normalizeLabel(label)
	for _, t := range cfg.Targets {
		if t.Label == label {
			return config.Target{}, fmt.Errorf("target label already exists: %s", label)
		}
	}

	account, err := promptString("Target account/namespace (e.g. org or username)", "", true)
	if err != nil {
		return config.Target{}, err
	}

	// Default repo name to current repo name
	defaultRepoName := getOriginRepoName(repoPath)
	repoName, err := promptString("Target repo name", defaultRepoName, true)
	if err != nil {
		return config.Target{}, err
	}

	// Get description, try to fetch from current repo (works with gh cli if available)
	defaultDesc := getRepoDescription(repoPath)
	description, _ := promptString("Repo description (optional)", defaultDesc, false)

	// Get topics/tags, try to fetch from current repo
	defaultTopics := getRepoTopics(repoPath)
	defaultTopicsStr := strings.Join(defaultTopics, ", ")
	topicsStr, _ := promptString("Repo topics/tags (comma-separated, optional)", defaultTopicsStr, false)
	var topics []string
	for _, t := range strings.Split(topicsStr, ",") {
		if t = strings.TrimSpace(t); t != "" {
			topics = append(topics, t)
		}
	}

	var urls provider.RepoURLs
	var repoURL string
	var auth config.AuthRef
	provName := ""

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch provChoice {
	case "github":
		provName = "github"
		useGH := ghAvailable()
		if useGH {
			useGH, _ = promptConfirm("Use gh CLI if available/authenticated?", true)
		}
		var token, tokenEnv string
		method := "gh"
		if !useGH {
			method = "token_env"
			tokenEnv, _ = promptString("GitHub token env var name (recommended)", "GITHUB_TOKEN", true)
			token = strings.TrimSpace(os.Getenv(tokenEnv))
			if token == "" {
				token, _ = promptSecret("GitHub token (used only now; not stored)", true)
			}
		}
		gh := provider.GitHubProvider{UseGHCLI: useGH, Token: token}

		for {
			exists, err := gh.RepoExists(ctx, account, repoName)
			if err != nil {
				return config.Target{}, err
			}
			if !exists {
				break
			}
			repoName, _ = promptString("Repo already exists. Pick a different repo name", "", true)
		}
		urls2, err := gh.CreatePrivateRepo(ctx, account, repoName, description)
		if err != nil {
			return config.Target{}, err
		}
		urls = urls2
		auth = config.AuthRef{Method: method, TokenEnv: tokenEnv}
		// Set topics if any were specified
		if len(topics) > 0 {
			if err := setGitHubRepoTopics(ctx, account, repoName, topics); err != nil {
				fmt.Printf("Warning: failed to set topics: %v\n", err)
			}
		}
	case "gitlab":
		provName = "gitlab"
		baseURL, _ := promptString("GitLab base URL", "https://gitlab.com", true)
		tokenEnv, _ := promptString("GitLab token env var name (recommended)", "GITLAB_TOKEN", true)
		token := strings.TrimSpace(os.Getenv(tokenEnv))
		if token == "" {
			token, _ = promptSecret("GitLab token (used only now; not stored)", true)
		}
		gl := provider.GitLabProvider{BaseURL: baseURL, Token: token}
		// conflict check best-effort
		if exists, _ := gl.RepoExists(ctx, account, repoName); exists {
			repoName, _ = promptString("Repo may already exist. Pick a different repo name", "", true)
		}
		urls2, err := gl.CreatePrivateRepo(ctx, account, repoName, description)
		if err != nil {
			return config.Target{}, err
		}
		urls = urls2
		auth = config.AuthRef{Method: "token_env", TokenEnv: tokenEnv, BaseURL: baseURL}
		// Set topics if any were specified
		if len(topics) > 0 {
			if err := gl.SetRepoTopics(ctx, account, repoName, topics); err != nil {
				fmt.Printf("Warning: failed to set topics: %v\n", err)
			}
		}
	case "gitea/forgejo":
		provName = "gitea"
		baseURL, _ := promptString("Gitea/Forgejo base URL (e.g. https://git.example.com)", "", true)
		tokenEnv, _ := promptString("Gitea token env var name (recommended)", "GITEA_TOKEN", true)
		token := strings.TrimSpace(os.Getenv(tokenEnv))
		if token == "" {
			token, _ = promptSecret("Gitea token (used only now; not stored)", true)
		}
		gt := provider.GiteaProvider{BaseURL: baseURL, Token: token}
		if exists, _ := gt.RepoExists(ctx, account, repoName); exists {
			repoName, _ = promptString("Repo may already exist. Pick a different repo name", "", true)
		}
		urls2, err := gt.CreatePrivateRepo(ctx, account, repoName, description)
		if err != nil {
			return config.Target{}, err
		}
		urls = urls2
		auth = config.AuthRef{Method: "token_env", TokenEnv: tokenEnv, BaseURL: baseURL}
		// Set topics if any were specified
		if len(topics) > 0 {
			if err := gt.SetRepoTopics(ctx, account, repoName, topics); err != nil {
				fmt.Printf("Warning: failed to set topics: %v\n", err)
			}
		}
	case "custom (existing repo)":
		provName = "custom"
		auth = config.AuthRef{Method: "none"}
		// Try to auto-generate URL for known providers
		customHost, _ := promptSelect("Where is the existing repo hosted?", []string{
			"github.com", "gitlab.com", "other",
		}, 0)
		if customHost == "other" {
			repoURL, _ = promptString("Existing target repo git URL (SSH or HTTPS)", "", true)
		} else {
			urlType, _ := promptSelect("Git URL type:", []string{"ssh (recommended)", "https"}, 0)
			if strings.HasPrefix(urlType, "https") {
				repoURL = fmt.Sprintf("https://%s/%s/%s.git", customHost, account, repoName)
			} else {
				repoURL = fmt.Sprintf("git@%s:%s/%s.git", customHost, account, repoName)
			}
			fmt.Printf("Using URL: %s\n", repoURL)
		}
	default:
		return config.Target{}, provider.ErrUnsupportedProvider(provChoice)
	}

	if provName != "custom" {
		urlType, _ := promptSelect("Git URL to use for pushing:", []string{"ssh", "https"}, 0)
		if urlType == "https" && urls.HTTPS != "" {
			repoURL = urls.HTTPS
		} else {
			repoURL = urls.SSH
		}
	}

	replacement, _ := promptString("Replacement string (default: account name)", account, true)
	addExcludes, _ := promptString("Additional excluded paths/globs for this target (comma-separated, optional)", "", false)
	ex := splitCSV(addExcludes)

	optInEnv, _ := promptConfirm("Opt-in to replicate .env for this target?", false)
	optIn := []string{}
	if optInEnv {
		optIn = append(optIn, ".env")
	}

	pubName, _ := promptString("Public author name (optional)", replacement, false)

	// Use saved email for this account if available, otherwise use default
	defaultEmail := globalPrefs.GetAccountEmail(account)
	if defaultEmail == "" {
		defaultEmail = replacement + "@example.invalid"
	}
	pubEmail, _ := promptString("Public author email (optional)", defaultEmail, false)

	// Save the email for future use with this account
	if pubEmail != "" && pubEmail != replacement+"@example.invalid" {
		globalPrefs.SetAccountEmail(account, pubEmail)
		_ = globalPrefs.Save() // Best effort, don't fail init if this fails
	}

	hm, _ := promptSelect("Initial history mode:", []string{"full (replay full history)", "future (start from now)"}, 0)
	mode := "full"
	if strings.HasPrefix(hm, "future") {
		mode = "future"
	}

	return config.Target{
		Label:              label,
		Provider:           provName,
		Account:            account,
		RepoName:           repoName,
		RepoURL:            repoURL,
		Description:        description,
		Topics:             topics,
		Replacement:        replacement,
		PublicAuthorName:   pubName,
		PublicAuthorEmail:  pubEmail,
		Exclude:            ex,
		OptIn:              optIn,
		Auth:               auth,
		InitialHistoryMode: mode,
		InitialSyncAt:      time.Now().Format(time.RFC3339),
	}, nil
}

func ghAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// setGitHubRepoTopics sets topics on a GitHub repo using gh CLI.
func setGitHubRepoTopics(ctx context.Context, account, repoName string, topics []string) error {
	if len(topics) == 0 {
		return nil
	}
	full := fmt.Sprintf("%s/%s", account, repoName)
	args := []string{"repo", "edit", full}
	for _, t := range topics {
		args = append(args, "--add-topic", t)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	// Use correct token for multi-account
	if token := provider.GHTokenForAccount(account); token != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+token)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh repo edit failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
