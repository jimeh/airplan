package airplan

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var scpRepository = regexp.MustCompile(
	`^git@([A-Za-z0-9.-]+):([^/\s]+)/([^/\s]+)$`,
)

// NormalizeRepositoryURL converts a supported Git remote URL to the
// canonical HTTPS repository URL used by rendered links.
func NormalizeRepositoryURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("airplan: repository URL is empty")
	}

	host, owner, repo := "", "", ""
	if match := scpRepository.FindStringSubmatch(raw); match != nil {
		host, owner, repo = match[1], match[2], match[3]
	} else {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("airplan: invalid repository %q: %w", raw, err)
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return "", fmt.Errorf(
				"airplan: invalid repository %q: query and fragment are not allowed",
				raw,
			)
		}
		switch u.Scheme {
		case "https":
			if u.User != nil {
				return "", fmt.Errorf(
					"airplan: invalid repository %q: credentials are not allowed", raw,
				)
			}
			if u.Port() != "" {
				return "", fmt.Errorf(
					"airplan: invalid repository %q: HTTPS ports are not allowed", raw,
				)
			}
		case "ssh":
			hasPassword := false
			if u.User != nil {
				_, hasPassword = u.User.Password()
			}
			if u.User == nil || u.User.Username() != "git" || hasPassword {
				return "", fmt.Errorf(
					"airplan: invalid repository %q: SSH user must be git without a password",
					raw,
				)
			}
		default:
			return "", fmt.Errorf(
				"airplan: invalid repository %q: HTTPS or SSH URL required", raw,
			)
		}
		if u.Hostname() == "" {
			return "", fmt.Errorf(
				"airplan: invalid repository %q: host required", raw,
			)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 2 {
			return "", fmt.Errorf(
				"airplan: invalid repository %q: owner/repository path required", raw,
			)
		}
		host, owner, repo = u.Hostname(), parts[0], parts[1]
	}

	repo = strings.TrimSuffix(repo, ".git")
	if host == "" || owner == "" || repo == "" ||
		strings.ContainsAny(owner+repo, "?#@:\\") ||
		owner == "." || owner == ".." || repo == "." || repo == ".." {
		return "", fmt.Errorf(
			"airplan: invalid repository %q: owner and repository are required", raw,
		)
	}
	return "https://" + strings.ToLower(host) + "/" + owner + "/" + repo, nil
}

func resolveRepository(
	ctx context.Context,
	setting string,
	name string,
	workingDirectory string,
) (string, error) {
	return resolveRepositoryWithGit(
		ctx, setting, name, workingDirectory, gitOutput,
	)
}

type gitCommand func(
	context.Context,
	string,
	...string,
) (string, error)

func resolveRepositoryWithGit(
	ctx context.Context,
	setting string,
	name string,
	workingDirectory string,
	runGit gitCommand,
) (string, error) {
	if setting == "" || setting == "none" {
		return "", nil
	}
	if setting != "auto" {
		return NormalizeRepositoryURL(setting)
	}
	if workingDirectory == "" {
		var err error
		workingDirectory, err = os.Getwd()
		if err != nil {
			return "", nil
		}
	}

	dir := workingDirectory
	if name != "" {
		absolute := name
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(workingDirectory, absolute)
		}
		dir = filepath.Dir(filepath.Clean(absolute))
		inside, err := runGit(ctx, dir, "rev-parse", "--is-inside-work-tree")
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		switch {
		case err == nil && inside == "true":
		case err != nil && strings.Contains(inside, "not a git repository"):
			dir = workingDirectory
		default:
			return "", nil
		}
	}

	remote, err := runGit(ctx, dir, "remote", "get-url", "origin")
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", nil
	}
	canonical, err := NormalizeRepositoryURL(remote)
	if err != nil {
		return "", nil
	}
	u, err := url.Parse(canonical)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return "", nil
	}
	return canonical, nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
