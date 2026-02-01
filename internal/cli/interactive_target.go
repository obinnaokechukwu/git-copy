package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/obinnaokechukwu/git-copy/internal/config"
	"github.com/obinnaokechukwu/git-copy/internal/provider"
)

func interactiveTargetSetup(cfg config.RepoConfig) (config.Target, error) {
	provChoice, err := promptSelect("Target hosting provider:", []string{
		"github", "gitlab", "gitea/forgejo", "custom (existing repo)",
	}, 0)
	if err != nil {
		return config.Target{}, err
	}

	label, err := promptString("Target label (alias used in commands)", "", true)
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
	repoName, err := promptString("Target repo name", "", true)
	if err != nil {
		return config.Target{}, err
	}
	description, _ := promptString("Repo description (optional)", "", false)

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
	case "custom (existing repo)":
		provName = "custom"
		auth = config.AuthRef{Method: "none"}
		repoURL, _ = promptString("Existing target repo git URL (SSH or HTTPS)", "", true)
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
	pubEmail, _ := promptString("Public author email (optional)", replacement+"@example.invalid", false)

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
