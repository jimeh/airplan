// Command updatedemos refreshes README demo links when any published object in
// origin storage differs from the repository's upload-mode fixtures. Reading
// through the storage API avoids false staleness from CDN transforms.
package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	inputPaths []string
	pageName   string
	args       []string
	objects    []expectedDemoObject
}

type demoContent struct {
	objects []demoObject
}

type expectedDemoObject struct {
	name string
	path string
}

type demoObject struct {
	name string
	body []byte
}

var repositoryDemos = []demo{
	{
		id:         "implementation plan",
		reference:  "airplan-demo-implementation-plan",
		inputPaths: []string{"airplan/testdata/implementation-plan.md"},
		pageName:   "implementation-plan.html",
		args: []string{
			"--repo", "https://github.com/octo-org/identity-platform",
		},
		objects: []expectedDemoObject{
			{
				name: "implementation-plan.html",
				path: "airplan/testdata/TestRenderMarkdownGolden/" +
					"implementation_plan.html",
			},
			{
				name: "implementation-plan.md",
				path: "airplan/testdata/implementation-plan.md",
			},
		},
	},
	{
		id:         "architecture overview",
		reference:  "airplan-demo-how-it-works",
		inputPaths: []string{"airplan/testdata/how-airplan-works.md"},
		pageName:   "how-airplan-works.html",
		args: []string{
			"--repo", "https://github.com/jimeh/airplan",
		},
		objects: []expectedDemoObject{
			{
				name: "how-airplan-works.html",
				path: "airplan/testdata/TestRenderMarkdownGolden/" +
					"how_airplan_works.html",
			},
			{
				name: "how-airplan-works.md",
				path: "airplan/testdata/how-airplan-works.md",
			},
		},
	},
	{
		id:         "Go API example",
		reference:  "airplan-demo-go-api",
		inputPaths: []string{"airplan/testdata/upload-example.go"},
		pageName:   "upload-example.html",
		args: []string{
			"--title", "Upload with airplan's Go API",
		},
		objects: []expectedDemoObject{
			{
				name: "upload-example.html",
				path: "airplan/testdata/TestRenderMarkdownGolden/" +
					"upload_example_go.html",
			},
			{
				name: "upload-example.go",
				path: "airplan/testdata/upload-example.go",
			},
		},
	},
	{
		id:        "collection evidence",
		reference: "airplan-demo-collection",
		inputPaths: []string{
			"airplan/testdata/collection-demo/verification-summary.svg",
			"airplan/testdata/collection-demo/checks.json",
			"airplan/testdata/collection-demo/release-notes.txt",
		},
		pageName: "index.html",
		args: []string{
			"--title", "Release verification evidence",
			"--repo", "https://github.com/jimeh/airplan",
		},
		objects: []expectedDemoObject{
			{
				name: "index.html",
				path: "airplan/testdata/TestRenderCollectionGolden/" +
					"release_verification_evidence.html",
			},
			{
				name: "verification-summary.svg",
				path: "airplan/testdata/collection-demo/" +
					"verification-summary.svg",
			},
			{
				name: "checks.json",
				path: "airplan/testdata/collection-demo/checks.json",
			},
			{
				name: "release-notes.txt",
				path: "airplan/testdata/collection-demo/release-notes.txt",
			},
		},
	},
	{
		id:        "API rollout bundle",
		reference: "airplan-demo-document-bundle",
		inputPaths: []string{
			"airplan/testdata/bundle-demo/implementation-plan.md",
			"airplan/testdata/bundle-demo/docs/design.md",
			"airplan/testdata/bundle-demo/examples/server.go",
			"airplan/testdata/bundle-demo/images/request-flow.svg",
		},
		pageName: "implementation-plan.html",
		args: []string{
			"--repo", "https://github.com/jimeh/airplan",
		},
		objects: []expectedDemoObject{
			{
				name: "implementation-plan.html",
				path: "airplan/testdata/TestRenderBundleDemoGolden/" +
					"implementation_plan.html",
			},
			{
				name: "implementation-plan.md",
				path: "airplan/testdata/bundle-demo/implementation-plan.md",
			},
			{
				name: "docs/design.html",
				path: "airplan/testdata/TestRenderBundleDemoGolden/design.html",
			},
			{
				name: "docs/design.md",
				path: "airplan/testdata/bundle-demo/docs/design.md",
			},
			{
				name: "examples/server.go.html",
				path: "airplan/testdata/TestRenderBundleDemoGolden/server_go.html",
			},
			{
				name: "examples/server.go",
				path: "airplan/testdata/bundle-demo/examples/server.go",
			},
			{
				name: "images/request-flow.svg",
				path: "airplan/testdata/bundle-demo/images/request-flow.svg",
			},
		},
	},
}

type fetcher interface {
	Fetch(context.Context, string) ([]byte, error)
}

type airplanFetcher struct {
	client *airplan.Client
}

func (f airplanFetcher) Fetch(
	ctx context.Context, objectURL string,
) ([]byte, error) {
	result, err := f.client.GetUpload(
		ctx, objectURL, airplan.GetOptions{},
	)
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

type publishedResult struct {
	URL    string                `json:"url"`
	Pages  []airplan.PageResult  `json:"pages"`
	Assets []airplan.AssetResult `json:"assets"`
	Files  []airplan.FileResult  `json:"files"`
}

func (p commandPublisher) Publish(ctx context.Context, d demo) (string, error) {
	args := append([]string{"--json"}, d.args...)
	args = append(args, d.inputPaths...)
	cmd := exec.CommandContext(ctx, p.path, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = p.stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("upload %s: %w", d.id, err)
	}
	var result publishedResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return "", fmt.Errorf("upload %s: decode result: %w", d.id, err)
	}
	if err := validatePublishedResult(result, d); err != nil {
		return "", fmt.Errorf("upload %s: %w", d.id, err)
	}
	return result.URL, nil
}

func validatePublishedResult(result publishedResult, d demo) error {
	if err := validatePageURL(result.URL, d); err != nil {
		return err
	}
	content, err := loadDemoContent(d)
	if err != nil {
		return err
	}
	expected := make(map[string]string, len(content.objects))
	for _, object := range content.objects {
		objectURL, objectErr := demoObjectURL(
			result.URL, d.pageName, object.name,
		)
		if objectErr != nil {
			return fmt.Errorf("expected object %q: %w", object.name, objectErr)
		}
		expected[objectURL] = object.name
	}

	var inventory []string
	switch {
	case len(result.Files) != 0:
		if len(result.Pages) != 0 || len(result.Assets) != 0 {
			return errors.New("published result mixes collection and document objects")
		}
		inventory = append(inventory, result.URL)
		for _, file := range result.Files {
			inventory = append(inventory, file.URL)
		}
	case len(result.Pages) != 0:
		for _, page := range result.Pages {
			inventory = append(inventory, page.URL)
			if page.SourceURL != "" {
				inventory = append(inventory, page.SourceURL)
			}
		}
		for _, asset := range result.Assets {
			inventory = append(inventory, asset.URL)
		}
	default:
		return errors.New("published result is missing object inventory")
	}

	seen := make(map[string]bool, len(inventory))
	for _, objectURL := range inventory {
		if err := validatePublishedObjectURL(result.URL, objectURL); err != nil {
			return err
		}
		if seen[objectURL] {
			return fmt.Errorf("published result contains duplicate object URL %q", objectURL)
		}
		seen[objectURL] = true
		if _, ok := expected[objectURL]; !ok {
			return fmt.Errorf("published result contains unexpected object URL %q", objectURL)
		}
	}
	for objectURL, name := range expected {
		if !seen[objectURL] {
			return fmt.Errorf("published result is missing object %q", name)
		}
	}
	return nil
}

func validatePublishedObjectURL(pageURL, objectURL string) error {
	page, err := url.Parse(pageURL)
	if err != nil {
		return err
	}
	object, err := url.Parse(objectURL)
	if err != nil {
		return fmt.Errorf("malformed published object URL %q: %w", objectURL, err)
	}
	if object.Scheme != "https" || object.Host == "" || object.User != nil {
		return fmt.Errorf("published object URL %q must be absolute HTTPS", objectURL)
	}
	if object.RawQuery != "" || object.Fragment != "" {
		return fmt.Errorf(
			"published object URL %q must not contain a query or fragment",
			objectURL,
		)
	}
	if object.Scheme != page.Scheme || object.Host != page.Host {
		return fmt.Errorf("published object URL %q has a different origin", objectURL)
	}
	root := strings.TrimSuffix(path.Dir(page.Path), "/") + "/"
	clean := path.Clean(object.Path)
	if !strings.HasPrefix(clean, root) {
		return fmt.Errorf(
			"published object URL %q is outside the upload directory", objectURL,
		)
	}
	return nil
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
		if !force {
			selected = selectFresh(
				ctx, fetch, d, content, current[d.reference], "current", log,
			)
		}
		if !force && selected == "" && candidates != nil {
			selected = selectFresh(
				ctx, fetch, d, content, candidates[d.reference],
				"candidate", log,
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
	for _, object := range content.objects {
		objectURL, err := demoObjectURL(pageURL, d.pageName, object.name)
		if err != nil {
			return false, object.name + ": " + err.Error()
		}
		matches, err := remoteMatches(ctx, fetch, objectURL, object.body)
		if err != nil {
			return false, object.name + ": " + err.Error()
		}
		if !matches {
			return false, object.name + " bytes differ"
		}
	}
	return true, ""
}

func loadDemoContent(d demo) (demoContent, error) {
	content := demoContent{objects: make([]demoObject, 0, len(d.objects))}
	seen := make(map[string]bool, len(d.objects))
	hasPage := false
	for _, object := range d.objects {
		if err := airplan.ValidateBundlePath(object.name); err != nil {
			return demoContent{}, fmt.Errorf(
				"%s has invalid object name %q: %w", d.id, object.name, err,
			)
		}
		if seen[object.name] {
			return demoContent{}, fmt.Errorf(
				"%s has duplicate object name %q", d.id, object.name,
			)
		}
		seen[object.name] = true
		hasPage = hasPage || object.name == d.pageName
		body, err := os.ReadFile(object.path)
		if err != nil {
			return demoContent{}, fmt.Errorf(
				"read %s object %q from %q: %w",
				d.id, object.name, object.path, err,
			)
		}
		content.objects = append(content.objects, demoObject{
			name: object.name, body: body,
		})
	}
	if !hasPage {
		return demoContent{}, fmt.Errorf(
			"%s does not declare entry page object %q", d.id, d.pageName,
		)
	}
	return content, nil
}

func remoteMatches(
	ctx context.Context,
	fetch fetcher,
	objectURL string,
	expected []byte,
) (bool, error) {
	actual, err := fetch.Fetch(ctx, objectURL)
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

func demoObjectURL(pageURL, pageName, objectName string) (string, error) {
	if err := airplan.ValidateBundlePath(pageName); err != nil {
		return "", fmt.Errorf("invalid entry page name %q: %w", pageName, err)
	}
	if err := airplan.ValidateBundlePath(objectName); err != nil {
		return "", fmt.Errorf("invalid object name %q: %w", objectName, err)
	}
	if objectName == pageName {
		return pageURL, nil
	}
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	parsed.Path = path.Join(path.Dir(parsed.Path), objectName)
	parsed.RawPath = ""
	return parsed.String(), nil
}

func validatePageURL(raw string, d demo) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return errors.New("demo URL must be absolute HTTPS")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("demo URL must not contain a query or fragment")
	}
	if path.Base(parsed.Path) != d.pageName {
		return fmt.Errorf("demo URL page does not match %q", d.pageName)
	}
	return nil
}
