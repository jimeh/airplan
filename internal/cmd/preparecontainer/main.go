// Command preparecontainer creates a deterministic Docker build context from
// GoReleaser artifact metadata.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const buildID = "airplan"

var supportedMachines = map[string]elf.Machine{
	"amd64": elf.EM_X86_64,
	"arm64": elf.EM_AARCH64,
}

var supportedArchitectures = []string{"amd64", "arm64"}

type artifact struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	Type   string `json:"type"`
	Extra  struct {
		ID string `json:"ID"`
	} `json:"extra"`
}

type options struct {
	artifacts     string
	dockerfile    string
	output        string
	verifyArchive bool
}

func main() {
	var opts options
	flag.StringVar(&opts.artifacts, "artifacts", "dist/artifacts.json",
		"GoReleaser artifacts.json path")
	flag.StringVar(&opts.dockerfile, "dockerfile", "Dockerfile.release",
		"release Dockerfile path")
	flag.StringVar(&opts.output, "output", "dist/container",
		"generated container context path")
	flag.BoolVar(&opts.verifyArchive, "verify-archives", false,
		"require staged binaries to match GoReleaser Linux archives")
	flag.Parse()
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(opts options) error {
	artifacts, err := readArtifacts(opts.artifacts)
	if err != nil {
		return err
	}
	binaries, err := selectArtifacts(artifacts, "Binary")
	if err != nil {
		return err
	}
	if err := requireRegular(opts.dockerfile); err != nil {
		return fmt.Errorf("airplan: release Dockerfile: %w", err)
	}

	output, err := filepath.Abs(opts.output)
	if err != nil {
		return fmt.Errorf("airplan: resolve container output: %w", err)
	}
	parent := filepath.Dir(output)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("airplan: create container output parent: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, "."+filepath.Base(output)+".tmp-")
	if err != nil {
		return fmt.Errorf("airplan: create temporary container context: %w", err)
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	if err := populateContext(temporary, opts.dockerfile, binaries); err != nil {
		return err
	}
	if opts.verifyArchive {
		archives, err := selectArtifacts(artifacts, "Archive")
		if err != nil {
			return err
		}
		if err := verifyArchives(temporary, archives); err != nil {
			return err
		}
	}
	if err := replaceDirectory(output, temporary); err != nil {
		return err
	}
	return nil
}

func readArtifacts(path string) ([]artifact, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("airplan: read GoReleaser artifacts: %w", err)
	}
	var artifacts []artifact
	if err := json.Unmarshal(body, &artifacts); err != nil {
		return nil, fmt.Errorf("airplan: parse GoReleaser artifacts: %w", err)
	}
	return artifacts, nil
}

func selectArtifacts(
	artifacts []artifact,
	artifactType string,
) (map[string]artifact, error) {
	selected := map[string]artifact{}
	for _, candidate := range artifacts {
		if candidate.Type != artifactType ||
			candidate.Extra.ID != buildID ||
			candidate.GOOS != "linux" {
			continue
		}
		if _, supported := supportedMachines[candidate.GOARCH]; !supported {
			return nil, fmt.Errorf(
				"airplan: unexpected GoReleaser %s artifact for linux/%s",
				strings.ToLower(artifactType), candidate.GOARCH,
			)
		}
		if _, exists := selected[candidate.GOARCH]; exists {
			return nil, fmt.Errorf(
				"airplan: duplicate GoReleaser %s artifact for linux/%s",
				strings.ToLower(artifactType), candidate.GOARCH,
			)
		}
		selected[candidate.GOARCH] = candidate
	}
	for _, architecture := range supportedArchitectures {
		if _, exists := selected[architecture]; !exists {
			return nil, fmt.Errorf(
				"airplan: missing GoReleaser %s artifact for linux/%s",
				strings.ToLower(artifactType), architecture,
			)
		}
	}
	return selected, nil
}

func populateContext(
	destination, dockerfile string,
	binaries map[string]artifact,
) error {
	if err := copyFile(
		dockerfile, filepath.Join(destination, "Dockerfile"), 0o644,
	); err != nil {
		return fmt.Errorf("airplan: stage release Dockerfile: %w", err)
	}
	for _, architecture := range supportedArchitectures {
		machine := supportedMachines[architecture]
		source := binaries[architecture].Path
		if err := requireRegular(source); err != nil {
			return fmt.Errorf(
				"airplan: validate linux/%s binary: %w", architecture, err,
			)
		}
		if err := validateELF(source, machine); err != nil {
			return fmt.Errorf(
				"airplan: validate linux/%s binary: %w", architecture, err,
			)
		}
		target := filepath.Join(
			destination, "linux", architecture, "airplan",
		)
		if err := copyFile(source, target, 0o555); err != nil {
			return fmt.Errorf(
				"airplan: stage linux/%s binary: %w", architecture, err,
			)
		}
	}
	state := filepath.Join(destination, "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		return fmt.Errorf("airplan: create state context: %w", err)
	}
	if err := os.WriteFile(filepath.Join(state, ".keep"), nil, 0o600); err != nil {
		return fmt.Errorf("airplan: create state placeholder: %w", err)
	}
	return nil
}

func requireRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s is empty", path)
	}
	return nil
}

func validateELF(path string, want elf.Machine) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	binary, err := elf.NewFile(file)
	if err != nil {
		return fmt.Errorf("parse ELF header: %w", err)
	}
	if binary.Machine != want {
		return fmt.Errorf("ELF machine is %s, want %s", binary.Machine, want)
	}
	return nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	if err := requireRegular(source); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(
		destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode,
	)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return nil
}

func verifyArchives(context string, archives map[string]artifact) error {
	for _, architecture := range supportedArchitectures {
		staged, err := fileDigest(
			filepath.Join(context, "linux", architecture, "airplan"),
		)
		if err != nil {
			return err
		}
		archived, err := archiveBinaryDigest(archives[architecture].Path)
		if err != nil {
			return fmt.Errorf(
				"airplan: verify linux/%s release archive: %w",
				architecture, err,
			)
		}
		if staged != archived {
			return fmt.Errorf(
				"airplan: linux/%s staged binary does not match release archive",
				architecture,
			)
		}
	}
	return nil
}

func fileDigest(path string) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	file, err := os.Open(path)
	if err != nil {
		return empty, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return empty, err
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func archiveBinaryDigest(path string) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	if err := requireRegular(path); err != nil {
		return empty, err
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, err
	}
	defer func() { _ = file.Close() }()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return empty, err
	}
	defer func() { _ = compressed.Close() }()
	reader := tar.NewReader(compressed)
	var result [sha256.Size]byte
	count := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return empty, err
		}
		if filepath.ToSlash(header.Name) != "airplan" {
			continue
		}
		if !header.FileInfo().Mode().IsRegular() {
			return empty, errors.New("root airplan entry is not a regular file")
		}
		count++
		hash := sha256.New()
		if _, err := io.Copy(hash, reader); err != nil {
			return empty, err
		}
		copy(result[:], hash.Sum(nil))
	}
	if count != 1 {
		return empty, fmt.Errorf(
			"archive contains %d root airplan executables, want 1", count,
		)
	}
	return result, nil
}

func replaceDirectory(destination, temporary string) error {
	backup := temporary + ".old"
	exists := false
	if _, err := os.Lstat(destination); err == nil {
		exists = true
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("airplan: preserve stale container context: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("airplan: inspect container output: %w", err)
	}
	if err := os.Rename(temporary, destination); err != nil {
		if exists {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("airplan: install container context: %w", err)
	}
	if exists {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("airplan: remove stale container context: %w", err)
		}
	}
	return nil
}
