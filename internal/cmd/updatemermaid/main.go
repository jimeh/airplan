// Command updatemermaid safely updates the repository's Mermaid pin.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	manifestPath        = "internal/deps/mermaid.json"
	renderGoldenPattern = "airplan/testdata/TestRenderMarkdownGolden/*.html"
	minimumAge          = 72 * time.Hour
)

type manifest struct {
	Package    string `json:"package"`
	Version    string `json:"version"`
	ReleasedAt string `json:"released_at"`
}

type registryDocument struct {
	DistTags map[string]string `json:"dist-tags"`
	Time     map[string]string `json:"time"`
}

type version struct{ major, minor, patch int }

func main() {
	dryRun := flag.Bool(
		"dry-run",
		false,
		"report the eligible update without writing files",
	)
	flag.Parse()
	client := &http.Client{Timeout: 15 * time.Second}
	if err := update(time.Now().UTC(), client, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "update Mermaid:", err)
		os.Exit(1)
	}
}

func update(now time.Time, client *http.Client, dryRun bool) (retErr error) {
	renderGoldenPaths, err := findRenderGoldens()
	if err != nil {
		return err
	}
	trackedPaths := append([]string{
		manifestPath,
		"airplan/mermaid_generated.go",
	}, renderGoldenPaths...)
	originals := make(map[string][]byte, len(trackedPaths))
	for _, path := range trackedPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		originals[path] = data
	}
	original := originals[manifestPath]
	var current manifest
	if err := json.Unmarshal(original, &current); err != nil {
		return err
	}
	currentVersion, err := parseVersion(current.Version)
	if err != nil {
		return err
	}

	registry, err := fetchRegistry(client)
	if err != nil {
		return err
	}
	latest, err := parseVersion(registry.DistTags["latest"])
	if err != nil {
		return fmt.Errorf("invalid or missing latest dist-tag: %w", err)
	}
	if latest.major > currentVersion.major {
		fmt.Fprintf(os.Stderr, "new Mermaid major available: %d\n", latest.major)
	}
	latestTime, ok := registry.Time[registry.DistTags["latest"]]
	if !ok {
		return errors.New("npm registry time is missing for latest release")
	}
	if _, err := time.Parse(time.RFC3339Nano, latestTime); err != nil {
		return fmt.Errorf("malformed registry time for latest release: %w", err)
	}

	type candidate struct {
		value version
		raw   string
		time  time.Time
	}
	var eligible []candidate
	for raw, publishedRaw := range registry.Time {
		v, parseErr := parseVersion(raw)
		if parseErr != nil || v.major != currentVersion.major {
			continue
		}
		published, parseErr := time.Parse(time.RFC3339Nano, publishedRaw)
		if parseErr != nil {
			return fmt.Errorf("malformed registry time for %s: %w", raw, parseErr)
		}
		if !published.After(now.Add(-minimumAge)) {
			eligible = append(eligible, candidate{v, raw, published})
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		return compareVersion(eligible[i].value, eligible[j].value) > 0
	})
	if len(eligible) == 0 ||
		compareVersion(eligible[0].value, currentVersion) <= 0 {
		fmt.Println("Mermaid is current; no eligible update")
		return nil
	}
	next := eligible[0]
	url := "https://cdn.jsdelivr.net/npm/mermaid@" + next.raw +
		"/dist/mermaid.esm.min.mjs"
	if err := probe(client, url); err != nil {
		return err
	}
	if dryRun {
		fmt.Printf("would update Mermaid %s -> %s\n", current.Version, next.raw)
		return nil
	}

	updated := manifest{
		Package: "mermaid", Version: next.raw,
		ReleasedAt: next.time.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	rollback := true
	defer func() {
		if rollback {
			if err := restoreFiles(originals); err != nil {
				retErr = errors.Join(
					retErr,
					fmt.Errorf("restore Mermaid update files: %w", err),
				)
			}
		}
	}()
	if err := os.WriteFile(manifestPath, encoded, 0o644); err != nil {
		return err
	}
	commands := [][]string{
		{
			"go", "run", "./internal/cmd/genmermaid",
			manifestPath, "airplan/mermaid_generated.go",
		},
		{"go", "test", "./airplan", "-run", "TestRenderMarkdownGolden"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GOLDEN_UPDATE=1")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("run %s: %w", strings.Join(args, " "), err)
		}
	}
	rollback = false
	fmt.Printf("updated Mermaid %s -> %s\n", current.Version, next.raw)
	return nil
}

func findRenderGoldens() ([]string, error) {
	paths, err := globRenderGoldens()
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf(
			"no render golden files match %q", renderGoldenPattern,
		)
	}
	return paths, nil
}

func restoreFiles(originals map[string][]byte) error {
	var restoreErr error
	currentGoldens, err := globRenderGoldens()
	if err != nil {
		restoreErr = errors.Join(
			restoreErr,
			fmt.Errorf("find current render goldens: %w", err),
		)
	} else {
		for _, path := range currentGoldens {
			if _, existed := originals[path]; existed {
				continue
			}
			if err := os.Remove(path); err != nil {
				restoreErr = errors.Join(
					restoreErr,
					fmt.Errorf("remove %s: %w", path, err),
				)
			}
		}
	}
	for path, data := range originals {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			restoreErr = errors.Join(
				restoreErr,
				fmt.Errorf("restore %s: %w", path, err),
			)
		}
	}
	return restoreErr
}

func globRenderGoldens() ([]string, error) {
	return filepath.Glob(filepath.FromSlash(renderGoldenPattern))
}

func fetchRegistry(client *http.Client) (registryDocument, error) {
	var doc registryDocument
	req, err := http.NewRequest(http.MethodGet,
		"https://registry.npmjs.org/mermaid", nil)
	if err != nil {
		return doc, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return doc, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return doc, fmt.Errorf("npm registry returned %s", resp.Status)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	if err := dec.Decode(&doc); err != nil {
		return doc, err
	}
	if len(doc.DistTags) == 0 || len(doc.Time) == 0 {
		return doc, errors.New("npm registry metadata is incomplete")
	}
	return doc, nil
}

func probe(client *http.Client, url string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("jsDelivr probe returned %s", resp.Status)
	}
	return nil
}

func parseVersion(raw string) (version, error) {
	if strings.ContainsAny(raw, "+-") {
		return version{}, fmt.Errorf("%q is not a stable version", raw)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return version{}, fmt.Errorf("%q is not semantic version", raw)
	}
	values := [3]int{}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return version{}, fmt.Errorf("%q is not canonical", raw)
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, fmt.Errorf("%q is not semantic version", raw)
		}
		values[i] = n
	}
	return version{values[0], values[1], values[2]}, nil
}

func compareVersion(a, b version) int {
	for _, pair := range [][2]int{
		{a.major, b.major},
		{a.minor, b.minor},
		{a.patch, b.patch},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
