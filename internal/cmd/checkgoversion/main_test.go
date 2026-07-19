package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckGoVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		versionFile string
		moduleFile  string
		lockFile    string
		wantErr     bool
	}{
		{
			name:        "matching versions",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.5\n",
			lockFile:    "[[tools.go]]\nversion = \"1.26.5\"\n",
		},
		{
			name:        "different versions",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.4\n",
			lockFile:    "[[tools.go]]\nversion = \"1.26.5\"\n",
			wantErr:     true,
		},
		{
			name:        "version file has extra content",
			versionFile: "1.26.5\n1.26.4\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.5\n",
			lockFile:    "[[tools.go]]\nversion = \"1.26.5\"\n",
			wantErr:     true,
		},
		{
			name:        "module has no go directive",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n",
			lockFile:    "[[tools.go]]\nversion = \"1.26.5\"\n",
			wantErr:     true,
		},
		{
			name:        "module has duplicate go directives",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.5\ngo 1.26.5\n",
			lockFile:    "[[tools.go]]\nversion = \"1.26.5\"\n",
			wantErr:     true,
		},
		{
			name:        "module has toolchain directive",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.5\ntoolchain go1.27.0\n",
			lockFile:    "[[tools.go]]\nversion = \"1.26.5\"\n",
			wantErr:     true,
		},
		{
			name:        "lock has different version",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.5\n",
			lockFile:    "[[tools.go]]\nversion = \"1.26.4\"\n",
			wantErr:     true,
		},
		{
			name:        "lock has no go version",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.5\n",
			lockFile:    "[tools]\n",
			wantErr:     true,
		},
		{
			name:        "lock has duplicate go versions",
			versionFile: "1.26.5\n",
			moduleFile:  "module example.com/test\n\ngo 1.26.5\n",
			lockFile: "[[tools.go]]\nversion = \"1.26.5\"\n" +
				"[[tools.go]]\nversion = \"1.26.5\"\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			versionPath := filepath.Join(dir, versionFile)
			modulePath := filepath.Join(dir, "go.mod")
			lockPath := filepath.Join(dir, "mise.lock")
			if err := os.WriteFile(versionPath, []byte(tt.versionFile), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(modulePath, []byte(tt.moduleFile), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(lockPath, []byte(tt.lockFile), 0o600); err != nil {
				t.Fatal(err)
			}

			err := checkGoVersion(versionPath, modulePath, lockPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkGoVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
