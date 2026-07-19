// Command updatedemos refreshes README demo links when their origin storage
// bytes differ from the repository's upload-mode golden pages and source files.
// Reading through the storage API avoids false staleness from CDN transforms.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/jimeh/airplan/airplan"
)

const defaultReadmePath = "README.md"

type demo struct {
	id         string
	reference  string
	sourcePath string
	goldenPath string
	slug       string
	args       []string
}

type demoContent struct {
	page   []byte
	source []byte
}

var repositoryDemos = []demo{
	{
		id:         "implementation plan",
		reference:  "airplan-demo-implementation-plan",
		sourcePath: "airplan/testdata/implementation-plan.md",
		goldenPath: "airplan/testdata/TestRenderMarkdownGolden/" +
			"implementation_plan.html",
		slug: "implementation-plan",
		args: []string{
			"--repo", "https://github.com/octo-org/identity-platform",
		},
	},
	{
		id:         "architecture overview",
		reference:  "airplan-demo-how-it-works",
		sourcePath: "airplan/testdata/how-airplan-works.md",
		goldenPath: "airplan/testdata/TestRenderMarkdownGolden/" +
			"how_airplan_works.html",
		slug: "how-airplan-works",
		args: []string{
			"--repo", "https://github.com/jimeh/airplan",
		},
	},
	{
		id:         "Go API example",
		reference:  "airplan-demo-go-api",
		sourcePath: "airplan/testdata/upload-example.go",
		goldenPath: "airplan/testdata/TestRenderMarkdownGolden/" +
			"upload_example_go.html",
		slug: "upload-example",
		args: []string{
			"--title", "Upload with airplan's Go API",
		},
	},
}

type fetcher interface {
	Fetch(context.Context, string, bool) ([]byte, error)
}

type airplanFetcher struct {
	client *airplan.Client
}

func (f airplanFetcher) Fetch(
	ctx context.Context, pageURL string, source bool,
) ([]byte, error) {
	target := pageURL
	if source {
		var err error
		target, err = uploadDirectoryURL(pageURL)
		if err != nil {
			return nil, err
		}
	}
	result, err := f.client.GetUpload(ctx, target, airplan.GetOptions{
		Source: source,
	})
	if err != nil {
		return nil, err
	}
	return result.Body, nil
}

type publisher interface {
	Publish(context.Context, demo) (string, error)
}

type commandPublisher struct {
	path   string
	stderr io.Writer
}

func (p commandPublisher) Publish(ctx context.Context, d demo) (string, error) {
	args := append([]string(nil), d.args...)
	args = append(args, d.sourcePath)
	cmd := exec.CommandContext(ctx, p.path, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = p.stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("upload %s: %w", d.id, err)
	}
	result := strings.TrimSpace(stdout.String())
	if err := validatePageURL(result, d); err != nil {
		return "", fmt.Errorf("upload %s: %w", d.id, err)
	}
	return result, nil
}

func main() {
	readmePath := flag.String(
		"readme", defaultReadmePath, "README file containing demo references",
	)
	candidatePath := flag.String(
		"candidate-readme", "",
		"README from an open automation PR whose demo URLs may be reused",
	)
	airplanPath := flag.String(
		"airplan", "./bin/airplan", "airplan executable used for uploads",
	)
	force := flag.Bool("force", false, "upload fresh copies of every demo")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "update demos:", err)
		os.Exit(1)
	}
	client, err := airplan.New(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "update demos:", err)
		os.Exit(1)
	}
	fetch := airplanFetcher{client: client}
	publisher := commandPublisher{path: *airplanPath, stderr: os.Stderr}
	if err := updateReadme(
		ctx, fetch, publisher, repositoryDemos, *readmePath,
		*candidatePath, *force, os.Stderr,
	); err != nil {
		fmt.Fprintln(os.Stderr, "update demos:", err)
		os.Exit(1)
	}
}

func updateReadme(
	ctx context.Context,
	fetch fetcher,
	publisher publisher,
	demos []demo,
	readmePath string,
	candidatePath string,
	force bool,
	log io.Writer,
) error {
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	current, err := demoURLs(readme, demos)
	if err != nil {
		return fmt.Errorf("read current demo URLs: %w", err)
	}

	var candidates map[string]string
	if candidatePath != "" {
		candidateReadme, readErr := os.ReadFile(candidatePath)
		if readErr != nil {
			return readErr
		}
		candidates, err = availableDemoURLs(candidateReadme, demos)
		if err != nil {
			return fmt.Errorf("read candidate demo URLs: %w", err)
		}
	}

	updated := append([]byte(nil), readme...)
	for _, d := range demos {
		content, loadErr := loadDemoContent(d)
		if loadErr != nil {
			return loadErr
		}
		selected := ""
		if !force && candidates != nil {
			selected = selectFresh(
				ctx, fetch, d, content, candidates[d.reference],
				"candidate", log,
			)
		}
		if !force && selected == "" {
			selected = selectFresh(
				ctx, fetch, d, content, current[d.reference], "current", log,
			)
		}
		if selected == "" {
			fmt.Fprintf(log, "%s: uploading a fresh demo\n", d.id)
			selected, err = publisher.Publish(ctx, d)
			if err != nil {
				return err
			}
			fmt.Fprintf(log, "%s: uploaded %s\n", d.id, selected)
		}
		updated, err = replaceDemoURL(
			updated, d.reference, current[d.reference], selected,
		)
		if err != nil {
			return err
		}
	}

	if bytes.Equal(readme, updated) {
		fmt.Fprintln(log, "README demo links are current")
		return nil
	}
	info, err := os.Stat(readmePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(readmePath, updated, info.Mode().Perm()); err != nil {
		return err
	}
	fmt.Fprintln(log, "updated README demo links")
	return nil
}

func selectFresh(
	ctx context.Context,
	fetch fetcher,
	d demo,
	content demoContent,
	pageURL string,
	kind string,
	log io.Writer,
) string {
	if pageURL == "" {
		return ""
	}
	fresh, reason := demoIsFresh(ctx, fetch, d, content, pageURL)
	if fresh {
		fmt.Fprintf(log, "%s: reusing %s demo\n", d.id, kind)
		return pageURL
	}
	fmt.Fprintf(log, "%s: %s demo is stale: %s\n", d.id, kind, reason)
	return ""
}

func demoIsFresh(
	ctx context.Context,
	fetch fetcher,
	d demo,
	content demoContent,
	pageURL string,
) (bool, string) {
	if err := validatePageURL(pageURL, d); err != nil {
		return false, err.Error()
	}
	pageMatches, err := remoteMatches(
		ctx, fetch, pageURL, false, content.page,
	)
	if err != nil {
		return false, "page: " + err.Error()
	}
	if !pageMatches {
		return false, "page bytes differ"
	}
	sourceMatches, err := remoteMatches(
		ctx, fetch, pageURL, true, content.source,
	)
	if err != nil {
		return false, "source: " + err.Error()
	}
	if !sourceMatches {
		return false, "source bytes differ"
	}
	return true, ""
}

func loadDemoContent(d demo) (demoContent, error) {
	page, err := os.ReadFile(d.goldenPath)
	if err != nil {
		return demoContent{}, fmt.Errorf("read %s golden: %w", d.id, err)
	}
	source, err := os.ReadFile(d.sourcePath)
	if err != nil {
		return demoContent{}, fmt.Errorf("read %s source: %w", d.id, err)
	}
	return demoContent{page: page, source: source}, nil
}

func remoteMatches(
	ctx context.Context,
	fetch fetcher,
	pageURL string,
	source bool,
	expected []byte,
) (bool, error) {
	actual, err := fetch.Fetch(ctx, pageURL, source)
	if err != nil {
		return false, err
	}
	return bytes.Equal(actual, expected), nil
}

func demoURLs(readme []byte, demos []demo) (map[string]string, error) {
	result, err := availableDemoURLs(readme, demos)
	if err != nil {
		return nil, err
	}
	for _, d := range demos {
		if result[d.reference] == "" {
			return nil, fmt.Errorf("missing reference %q", d.reference)
		}
	}
	return result, nil
}

func availableDemoURLs(
	readme []byte,
	demos []demo,
) (map[string]string, error) {
	result := make(map[string]string, len(demos))
	lines := strings.Split(string(readme), "\n")
	for _, d := range demos {
		prefix := "[" + d.reference + "]: "
		for _, line := range lines {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			if _, exists := result[d.reference]; exists {
				return nil, fmt.Errorf("duplicate reference %q", d.reference)
			}
			result[d.reference] = strings.TrimSpace(
				strings.TrimPrefix(line, prefix),
			)
		}
	}
	return result, nil
}

func replaceDemoURL(
	readme []byte,
	reference string,
	oldURL string,
	newURL string,
) ([]byte, error) {
	oldLine := []byte("[" + reference + "]: " + oldURL)
	newLine := []byte("[" + reference + "]: " + newURL)
	if bytes.Count(readme, oldLine) != 1 {
		return nil, fmt.Errorf("reference %q is not unique", reference)
	}
	return bytes.Replace(readme, oldLine, newLine, 1), nil
}

func uploadDirectoryURL(pageURL string) (string, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	parsed.Path = path.Dir(parsed.Path)
	parsed.RawPath = ""
	return parsed.String(), nil
}

func validatePageURL(raw string, d demo) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("demo URL must be absolute HTTPS")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("demo URL must not contain a query or fragment")
	}
	if path.Base(parsed.Path) != d.slug+".html" {
		return fmt.Errorf("demo URL page does not match slug %q", d.slug)
	}
	return nil
}
