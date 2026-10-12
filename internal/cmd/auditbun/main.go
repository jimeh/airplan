// Command auditbun runs `bun audit` with the repository's advisory ignores.
// Every ignore carries an expiry date. From that date the command fails
// without running the audit, so an ignore cannot outlive its review. It
// rejects `--ignore` arguments, which would bypass that expiry.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const expiryLayout = "2006-01-02"

// advisoryPattern matches the identifiers `bun audit --ignore` accepts: a
// GHSA ID or a numeric advisory ID.
var advisoryPattern = regexp.MustCompile(
	`^(GHSA(-[0-9a-z]{4}){3}|[0-9]+)$`,
)

type ignore struct {
	Advisory string `json:"advisory"`
	Package  string `json:"package"`
	Reason   string `json:"reason"`
	Expires  string `json:"expires"`

	expires time.Time
}

func main() {
	os.Exit(run(os.Args[1:], time.Now(), os.Stderr, runBun))
}

// run returns the process exit status. audit runs `bun` with the given
// arguments and returns its exit status.
func run(
	args []string, now time.Time, stderr io.Writer,
	audit func(args []string) (int, error),
) int {
	if len(args) < 1 {
		fmt.Fprintln(
			stderr,
			"usage: auditbun <ignores.json> [bun audit arguments...]",
		)
		return 2
	}
	path, extra := args[0], args[1:]
	for _, arg := range extra {
		if arg == "--ignore" || strings.HasPrefix(arg, "--ignore=") {
			fmt.Fprintf(
				stderr,
				"%s is not accepted: list the advisory in %s, where it "+
					"needs a reason and an expiry date\n",
				arg, path,
			)
			return 2
		}
	}

	ignores, err := loadIgnores(path)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := checkExpiry(path, ignores, now); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	status, err := audit(auditArgs(ignores, extra))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return status
}

func runBun(args []string) (int, error) {
	cmd := exec.Command("bun", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

func loadIgnores(path string) ([]ignore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var file struct {
		Ignores []ignore `json:"ignores"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	seen := make(map[string]bool, len(file.Ignores))
	for index := range file.Ignores {
		entry := &file.Ignores[index]
		if !advisoryPattern.MatchString(entry.Advisory) {
			return nil, fmt.Errorf(
				"%s: ignore %d has invalid advisory %q",
				path, index, entry.Advisory,
			)
		}
		if seen[entry.Advisory] {
			return nil, fmt.Errorf(
				"%s: advisory %s is listed more than once",
				path, entry.Advisory,
			)
		}
		seen[entry.Advisory] = true
		if strings.TrimSpace(entry.Package) == "" {
			return nil, fmt.Errorf(
				"%s: ignore for %s names no package", path, entry.Advisory,
			)
		}
		if strings.TrimSpace(entry.Reason) == "" {
			return nil, fmt.Errorf(
				"%s: ignore for %s gives no reason", path, entry.Advisory,
			)
		}
		expires, err := time.Parse(expiryLayout, entry.Expires)
		if err != nil {
			return nil, fmt.Errorf(
				"%s: ignore for %s has invalid expiry %q, want YYYY-MM-DD",
				path, entry.Advisory, entry.Expires,
			)
		}
		entry.expires = expires
	}
	return file.Ignores, nil
}

// checkExpiry reports every ignore whose expiry date has arrived. An ignore
// applies through the end of the UTC day before its expiry date.
func checkExpiry(path string, ignores []ignore, now time.Time) error {
	var expired []string
	for _, entry := range ignores {
		if now.Before(entry.expires) {
			continue
		}
		expired = append(expired, fmt.Sprintf(
			"bun audit ignore for %s (%s) expired on %s: %s",
			entry.Advisory, entry.Package, entry.Expires, entry.Reason,
		))
	}
	if len(expired) == 0 {
		return nil
	}
	return fmt.Errorf(
		"%s\ncheck whether a fixed release or a withdrawn advisory "+
			"resolves each one, then remove its entry from %s or set a "+
			"new expiry",
		strings.Join(expired, "\n"), path,
	)
}

func auditArgs(ignores []ignore, extra []string) []string {
	args := []string{"audit"}
	args = append(args, extra...)
	for _, entry := range ignores {
		args = append(args, "--ignore="+entry.Advisory)
	}
	return args
}
