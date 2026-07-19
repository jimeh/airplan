package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

const versionFile = ".go-version"

func main() {
	if err := checkGoVersion(versionFile, "go.mod", "mise.lock"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkGoVersion(versionPath, modulePath, lockPath string) error {
	versionData, err := os.ReadFile(versionPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", versionPath, err)
	}

	version := strings.TrimSuffix(string(versionData), "\n")
	if version == "" || string(versionData) != version+"\n" {
		return fmt.Errorf("%s must contain one version followed by a newline", versionPath)
	}

	moduleData, err := os.ReadFile(modulePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", modulePath, err)
	}

	var moduleVersion string
	scanner := bufio.NewScanner(strings.NewReader(string(moduleData)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 && fields[0] == "toolchain" {
			return fmt.Errorf(
				"%s contains a toolchain directive; use %s instead",
				modulePath,
				versionPath,
			)
		}
		if len(fields) == 2 && fields[0] == "go" {
			if moduleVersion != "" {
				return fmt.Errorf("%s contains multiple go directives", modulePath)
			}
			moduleVersion = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", modulePath, err)
	}
	if moduleVersion == "" {
		return fmt.Errorf("%s does not contain a go directive", modulePath)
	}
	if moduleVersion != version {
		return fmt.Errorf(
			"%s contains %s, but %s requires %s",
			modulePath,
			moduleVersion,
			versionPath,
			version,
		)
	}

	var lock struct {
		Tools struct {
			Go []struct {
				Version string `toml:"version"`
			} `toml:"go"`
		} `toml:"tools"`
	}
	if _, err := toml.DecodeFile(lockPath, &lock); err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	if len(lock.Tools.Go) != 1 {
		return fmt.Errorf(
			"%s must contain exactly one locked Go version",
			lockPath,
		)
	}
	if lock.Tools.Go[0].Version != version {
		return fmt.Errorf(
			"%s contains Go %s, but %s requires %s",
			lockPath,
			lock.Tools.Go[0].Version,
			versionPath,
			version,
		)
	}

	return nil
}
