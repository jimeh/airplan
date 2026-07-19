package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const versionFile = ".go-version"

func main() {
	if err := checkGoVersion(versionFile, "go.mod"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func checkGoVersion(versionPath, modulePath string) error {
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

	return nil
}
