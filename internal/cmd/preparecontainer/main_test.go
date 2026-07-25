package main

import (
	"archive/tar"
	"compress/gzip"
	"debug/elf"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestPrepareContainerContext(t *testing.T) {
	fixture := newPrepareFixture(t)
	if err := run(fixture.options()); err != nil {
		t.Fatal(err)
	}

	var paths []string
	err := filepath.Walk(fixture.output, func(
		path string, info os.FileInfo, err error,
	) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(fixture.output, path)
		if err != nil {
			return err
		}
		if relative != "." {
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	want := []string{
		"Dockerfile",
		"linux",
		"linux/amd64",
		"linux/amd64/airplan",
		"linux/arm64",
		"linux/arm64/airplan",
		"state",
		"state/.keep",
	}
	if len(paths) != len(want) {
		t.Fatalf("context paths = %#v", paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("context paths = %#v", paths)
		}
	}
	for _, path := range []string{
		"linux/amd64/airplan", "linux/arm64/airplan",
	} {
		info, err := os.Stat(filepath.Join(fixture.output, path))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o555 {
			t.Fatalf("%s mode = %o", path, info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Join(fixture.output, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}
}

func TestPrepareContainerArtifactOrderingDoesNotMatter(t *testing.T) {
	fixture := newPrepareFixture(t)
	fixture.artifacts[0], fixture.artifacts[1] = fixture.artifacts[1], fixture.artifacts[0]
	fixture.writeArtifacts()
	if err := run(fixture.options()); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareContainerRejectsArtifactSelectionErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*prepareFixture)
	}{
		{
			name: "missing architecture",
			mutate: func(f *prepareFixture) {
				f.artifacts = f.artifacts[:1]
			},
		},
		{
			name: "duplicate architecture",
			mutate: func(f *prepareFixture) {
				f.artifacts = append(f.artifacts, f.artifacts[0])
			},
		},
		{
			name: "wrong build ID",
			mutate: func(f *prepareFixture) {
				f.artifacts[0].Extra.ID = "other"
			},
		},
		{
			name: "wrong OS",
			mutate: func(f *prepareFixture) {
				f.artifacts[0].GOOS = "darwin"
			},
		},
		{
			name: "unexpected architecture",
			mutate: func(f *prepareFixture) {
				candidate := artifact{
					Name: "airplan", Path: "/unused", GOOS: "linux",
					GOARCH: "386", Type: "Binary",
				}
				candidate.Extra.ID = buildID
				f.artifacts = append(f.artifacts, candidate)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrepareFixture(t)
			test.mutate(fixture)
			fixture.writeArtifacts()
			if err := run(fixture.options()); err == nil {
				t.Fatal("preparation succeeded")
			}
		})
	}
}

func TestPrepareContainerRejectsInvalidBinaries(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *prepareFixture)
	}{
		{
			name: "missing",
			mutate: func(t *testing.T, f *prepareFixture) {
				t.Helper()
				if err := os.Remove(f.artifacts[0].Path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "empty",
			mutate: func(t *testing.T, f *prepareFixture) {
				t.Helper()
				if err := os.WriteFile(f.artifacts[0].Path, nil, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, f *prepareFixture) {
				t.Helper()
				path := f.artifacts[0].Path
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(f.artifacts[1].Path, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "mismatched ELF",
			mutate: func(t *testing.T, f *prepareFixture) {
				t.Helper()
				writeELF(t, f.artifacts[0].Path, elf.EM_AARCH64)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPrepareFixture(t)
			test.mutate(t, fixture)
			if err := run(fixture.options()); err == nil {
				t.Fatal("preparation succeeded")
			}
		})
	}
}

func TestPrepareContainerReplacesStaleOutputOnlyAfterValidation(t *testing.T) {
	fixture := newPrepareFixture(t)
	if err := os.MkdirAll(fixture.output, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(fixture.output, "stale")
	if err := os.WriteFile(stale, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}

	writeELF(t, fixture.artifacts[0].Path, elf.EM_AARCH64)
	if err := run(fixture.options()); err == nil {
		t.Fatal("invalid preparation succeeded")
	}
	if body, err := os.ReadFile(stale); err != nil ||
		string(body) != "preserve" {
		t.Fatalf("stale output changed after failure: %q, %v", body, err)
	}

	writeELF(t, fixture.artifacts[0].Path, elf.EM_X86_64)
	if err := run(fixture.options()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale output remains: %v", err)
	}
}

func TestPrepareContainerVerifiesReleaseArchives(t *testing.T) {
	fixture := newPrepareFixture(t)
	for _, binary := range fixture.artifacts {
		path := filepath.Join(
			fixture.directory, binary.GOARCH+".tar.gz",
		)
		body, err := os.ReadFile(binary.Path)
		if err != nil {
			t.Fatal(err)
		}
		writeArchive(t, path, map[string][]byte{
			"airplan":   body,
			"README.md": []byte("documentation"),
		})
		archive := artifact{
			Name: binary.GOARCH + ".tar.gz",
			Path: path, GOOS: "linux", GOARCH: binary.GOARCH,
			Type: "Archive",
		}
		archive.Extra.ID = buildID
		fixture.artifacts = append(fixture.artifacts, archive)
	}
	fixture.writeArtifacts()
	opts := fixture.options()
	opts.verifyArchive = true
	if err := run(opts); err != nil {
		t.Fatal(err)
	}

	writeArchive(t, fixture.artifacts[2].Path, map[string][]byte{
		"airplan": []byte("different"),
	})
	if err := run(opts); err == nil {
		t.Fatal("archive mismatch was accepted")
	}
}

type prepareFixture struct {
	t          *testing.T
	directory  string
	artifacts  []artifact
	metadata   string
	dockerfile string
	output     string
}

func newPrepareFixture(t *testing.T) *prepareFixture {
	t.Helper()
	directory := t.TempDir()
	fixture := &prepareFixture{
		t: t, directory: directory,
		metadata:   filepath.Join(directory, "artifacts.json"),
		dockerfile: filepath.Join(directory, "Dockerfile.release"),
		output:     filepath.Join(directory, "container"),
	}
	if err := os.WriteFile(
		fixture.dockerfile, []byte("FROM scratch\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	for _, architecture := range supportedArchitectures {
		machine := supportedMachines[architecture]
		path := filepath.Join(directory, architecture, "airplan")
		writeELF(t, path, machine)
		candidate := artifact{
			Name: "airplan", Path: path, GOOS: "linux",
			GOARCH: architecture, Type: "Binary",
		}
		candidate.Extra.ID = buildID
		fixture.artifacts = append(fixture.artifacts, candidate)
	}
	fixture.writeArtifacts()
	return fixture
}

func (f *prepareFixture) options() options {
	return options{
		artifacts: f.metadata, dockerfile: f.dockerfile, output: f.output,
	}
}

func (f *prepareFixture) writeArtifacts() {
	f.t.Helper()
	body, err := json.Marshal(f.artifacts)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.metadata, body, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func writeELF(t *testing.T, path string, machine elf.Machine) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 64)
	copy(header, []byte{0x7f, 'E', 'L', 'F'})
	header[4] = byte(elf.ELFCLASS64)
	header[5] = byte(elf.ELFDATA2LSB)
	header[6] = byte(elf.EV_CURRENT)
	binary.LittleEndian.PutUint16(header[16:18], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(header[18:20], uint16(machine))
	binary.LittleEndian.PutUint32(header[20:24], uint32(elf.EV_CURRENT))
	binary.LittleEndian.PutUint16(header[52:54], 64)
	if err := os.WriteFile(path, header, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeArchive(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	for name, body := range files {
		if err := archive.WriteHeader(&tar.Header{
			Name: name, Mode: 0o755, Size: int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
