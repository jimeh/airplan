// Command checkspecsync detects contract changes that are missing the
// corresponding specification version updates.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const defaultBase = "origin/main"

var (
	specVersionPattern = regexp.MustCompile(
		`(?m)^\*\*Spec version:\s*([0-9]+\.[0-9]+\.[0-9]+)\*\*\s*$`,
	)
	implementationVersionPattern = regexp.MustCompile(
		`(?m)Targets spec version\s+([0-9]+\.[0-9]+\.[0-9]+)\.`,
	)
)

type gitRunner interface {
	run(args ...string) (string, error)
}

type commandGit struct {
	dir string
}

func (g commandGit) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), detail)
	}
	return stdout.String(), nil
}

type versions struct {
	baseSpec       string
	currentSpec    string
	implementation string
}

func main() {
	os.Exit(run(commandGit{dir: "."}, os.Getenv, os.Stderr))
}

func run(git gitRunner, getenv func(string) string, stderr io.Writer) int {
	base := getenv("SPEC_SYNC_BASE")
	if base == "" {
		base = defaultBase
	}

	findings, err := inspect(git, base)
	if err != nil {
		fmt.Fprintf(stderr, "spec sync check failed: %v\n", err)
		return 1
	}
	if len(findings) == 0 {
		return 0
	}

	githubActions := getenv("GITHUB_ACTIONS") != ""
	for _, finding := range findings {
		if githubActions {
			fmt.Fprintf(stderr, "::warning title=SPEC sync::%s\n", escapeAnnotation(finding))
		} else {
			fmt.Fprintf(stderr, "spec sync warning: %s\n", finding)
		}
	}
	if getenv("SPEC_SYNC_WARN_ONLY") == "1" {
		return 0
	}
	return 1
}

func inspect(git gitRunner, base string) ([]string, error) {
	mergeBase, err := git.run("merge-base", base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve merge base for %q: %w", base, err)
	}
	mergeBase = strings.TrimSpace(mergeBase)
	if mergeBase == "" {
		return nil, errors.New("git merge-base returned an empty revision")
	}

	changedOutput, err := git.run(
		"diff", "--name-only", "-z", "--no-renames", "--diff-filter=ACMRD",
		mergeBase, "HEAD", "--",
	)
	if err != nil {
		return nil, fmt.Errorf("list changed paths: %w", err)
	}

	baseSpec, err := git.run("show", mergeBase+":SPEC.md")
	if err != nil {
		return nil, fmt.Errorf("read SPEC.md at merge base: %w", err)
	}
	currentSpec, err := git.run("show", "HEAD:SPEC.md")
	if err != nil {
		return nil, fmt.Errorf("read current SPEC.md: %w", err)
	}
	implementation, err := git.run("show", "HEAD:IMPLEMENTATION.md")
	if err != nil {
		return nil, fmt.Errorf("read current IMPLEMENTATION.md: %w", err)
	}

	parsedVersions, err := parseVersions(baseSpec, currentSpec, implementation)
	if err != nil {
		return nil, err
	}
	return analyze(splitPaths(changedOutput), parsedVersions), nil
}

func splitPaths(output string) []string {
	if output == "" {
		return nil
	}
	paths := strings.Split(output, "\x00")
	if paths[len(paths)-1] == "" {
		paths = paths[:len(paths)-1]
	}
	return paths
}

func parseVersions(baseSpec, currentSpec, implementation string) (versions, error) {
	baseVersion, err := parseSingleVersion("base SPEC.md", baseSpec, specVersionPattern)
	if err != nil {
		return versions{}, err
	}
	currentVersion, err := parseSingleVersion(
		"current SPEC.md", currentSpec, specVersionPattern,
	)
	if err != nil {
		return versions{}, err
	}
	implementationVersion, err := parseSingleVersion(
		"current IMPLEMENTATION.md", implementation,
		implementationVersionPattern,
	)
	if err != nil {
		return versions{}, err
	}
	return versions{
		baseSpec:       baseVersion,
		currentSpec:    currentVersion,
		implementation: implementationVersion,
	}, nil
}

func parseSingleVersion(name, content string, pattern *regexp.Regexp) (string, error) {
	matches := pattern.FindAllStringSubmatch(content, -1)
	if len(matches) != 1 {
		return "", fmt.Errorf(
			"parse %s version: expected exactly one declaration, found %d",
			name, len(matches),
		)
	}
	return matches[0][1], nil
}

func analyze(paths []string, v versions) []string {
	var sensitive []string
	for _, path := range paths {
		path = filepath.ToSlash(path)
		if isContractSensitive(path) {
			sensitive = append(sensitive, path)
		}
	}
	sort.Strings(sensitive)

	var findings []string
	if len(sensitive) > 0 && v.currentSpec == v.baseSpec {
		findings = append(findings, fmt.Sprintf(
			"contract-sensitive paths changed without a SPEC.md version bump: %s",
			strings.Join(sensitive, ", "),
		))
	}
	if v.currentSpec != v.baseSpec && v.implementation != v.currentSpec {
		findings = append(findings, fmt.Sprintf(
			"IMPLEMENTATION.md targets spec version %s, but SPEC.md is version %s",
			v.implementation, v.currentSpec,
		))
	}
	return findings
}

func isContractSensitive(path string) bool {
	if path == "main.go" || path == "schema/airplan.schema.json" {
		return true
	}
	if strings.HasPrefix(path, "airplan/assets/") {
		return true
	}
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	if strings.Contains(path, "/testdata/") {
		return false
	}
	return strings.HasPrefix(path, "airplan/") || strings.HasPrefix(path, "cli/")
}

func escapeAnnotation(message string) string {
	replacer := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	return replacer.Replace(message)
}
