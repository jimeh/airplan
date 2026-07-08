package airplan

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigPrecedence(t *testing.T) {
	path := writeConfig(t, `
endpoint = "root-endpoint"
bucket = "root-bucket"
region = "root-region"
access_key_id = "root-access"
secret_access_key = "root-secret"
public_base_url = "root-public"
key_prefix = "root-prefix"
template = "root-template"
no_source = false
indexable = true

[profiles.work]
endpoint = "profile-endpoint"
bucket = "profile-bucket"
region = "profile-region"
access_key_id = "profile-access"
secret_access_key = "profile-secret"
public_base_url = "profile-public"
template = "profile-template"
no_source = true
indexable = false
`, 0o600)

	cfg, err := LoadConfig(ConfigOptions{
		Path:    path,
		Profile: "work",
		Getenv: envMap(map[string]string{
			"AIRPLAN_BUCKET":            "env-bucket",
			"AIRPLAN_REGION":            "env-region",
			"AIRPLAN_SECRET_ACCESS_KEY": "env-secret",
			"AIRPLAN_PUBLIC_BASE_URL":   "env-public",
			"AIRPLAN_TEMPLATE":          "env-template",
		}),
	})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	assertEqual(t, cfg.Profile, "work")
	assertEqual(t, cfg.Endpoint, "profile-endpoint")
	assertEqual(t, cfg.Bucket, "env-bucket")
	assertEqual(t, cfg.Region, "env-region")
	assertEqual(t, cfg.AccessKeyID, "profile-access")
	assertEqual(t, cfg.SecretAccessKey, "env-secret")
	assertEqual(t, cfg.PublicBaseURL, "env-public")
	assertEqual(t, cfg.KeyPrefix, "root-prefix")
	assertEqual(t, cfg.Template, "env-template")
	assertEqual(t, cfg.NoSource, true)
	assertEqual(t, cfg.Indexable, false)
}

func TestLoadConfigDefaultRegion(t *testing.T) {
	cfg, err := LoadConfig(ConfigOptions{
		Path: missingPath(t),
		Getenv: envMap(map[string]string{
			"AIRPLAN_ENDPOINT": "env-endpoint",
			"AIRPLAN_BUCKET":   "env-bucket",
		}),
	})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	assertEqual(t, cfg.Profile, "")
	assertEqual(t, cfg.Endpoint, "env-endpoint")
	assertEqual(t, cfg.Bucket, "env-bucket")
	assertEqual(t, cfg.Region, "auto")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestLoadConfigPathResolution(t *testing.T) {
	explicit := writeConfig(t, `
endpoint = "explicit-endpoint"
bucket = "explicit-bucket"
`, 0o600)
	envPath := writeConfig(t, `
endpoint = "env-endpoint"
bucket = "env-bucket"
`, 0o600)

	t.Run("explicit path wins over AIRPLAN_CONFIG", func(t *testing.T) {
		cfg, err := LoadConfig(ConfigOptions{
			Path: explicit,
			Getenv: envMap(map[string]string{
				"AIRPLAN_CONFIG": envPath,
			}),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Endpoint, "explicit-endpoint")
		assertEqual(t, cfg.Bucket, "explicit-bucket")
	})

	t.Run("AIRPLAN_CONFIG overrides default path", func(t *testing.T) {
		cfg, err := LoadConfig(ConfigOptions{
			Getenv: envMap(map[string]string{
				"AIRPLAN_CONFIG": envPath,
			}),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Endpoint, "env-endpoint")
		assertEqual(t, cfg.Bucket, "env-bucket")
	})
}

func TestProfileResolution(t *testing.T) {
	t.Run("options profile wins over environment profile", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.cli]
endpoint = "cli-endpoint"
bucket = "cli-bucket"

[profiles.env]
endpoint = "env-endpoint"
bucket = "env-bucket"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path:    path,
			Profile: "cli",
			Getenv: envMap(map[string]string{
				"AIRPLAN_PROFILE": "env",
			}),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Profile, "cli")
		assertEqual(t, cfg.Endpoint, "cli-endpoint")
	})

	t.Run("environment profile is selected", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.env]
endpoint = "env-endpoint"
bucket = "env-bucket"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path: path,
			Getenv: envMap(map[string]string{
				"AIRPLAN_PROFILE": "env",
			}),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Profile, "env")
	})

	t.Run("forced missing profile errors", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.work]
bucket = "work"

[profiles.home]
bucket = "home"
`, 0o600)

		_, err := LoadConfig(ConfigOptions{
			Path:    path,
			Profile: "missing",
			Getenv:  envMap(nil),
		})
		assertErrorContains(
			t,
			err,
			"profile \"missing\" does not exist",
			"home, work",
		)
	})

	t.Run("default profile is selected", func(t *testing.T) {
		path := writeConfig(t, `
default_profile = "work"

[profiles.work]
endpoint = "work-endpoint"
bucket = "work-bucket"

[profiles.home]
endpoint = "home-endpoint"
bucket = "home-bucket"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Profile, "work")
		assertEqual(t, cfg.Endpoint, "work-endpoint")
	})

	t.Run("dangling default profile errors", func(t *testing.T) {
		path := writeConfig(t, `
default_profile = "missing"

[profiles.work]
bucket = "work"
`, 0o600)

		_, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		assertErrorContains(
			t,
			err,
			"default_profile \"missing\" does not exist",
			"work",
		)
	})

	t.Run("exactly one profile is selected", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.only]
endpoint = "only-endpoint"
bucket = "only-bucket"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Profile, "only")
	})

	t.Run("root complete after env overlay uses root values", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "root-endpoint"

[profiles.home]
bucket = "home"

[profiles.work]
bucket = "work"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path: path,
			Getenv: envMap(map[string]string{
				"AIRPLAN_BUCKET": "env-bucket",
			}),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Profile, "")
		assertEqual(t, cfg.Endpoint, "root-endpoint")
		assertEqual(t, cfg.Bucket, "env-bucket")
	})

	t.Run("incomplete root errors and lists profiles", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.home]
bucket = "home"

[profiles.work]
bucket = "work"
`, 0o600)

		_, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		assertErrorContains(
			t,
			err,
			"root-level values are incomplete",
			"home, work",
		)
	})

	t.Run("no profiles uses root values without profile errors", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "root-endpoint"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Profile, "")
		assertEqual(t, cfg.Endpoint, "root-endpoint")
	})

	t.Run("dangling default_profile errors with no profiles", func(t *testing.T) {
		path := writeConfig(t, `
default_profile = "missing"
endpoint = "root-endpoint"
`, 0o600)

		_, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		assertErrorContains(
			t,
			err,
			"default_profile \"missing\" does not exist",
			"none defined",
		)
	})
}

func TestLoadConfigNoFile(t *testing.T) {
	cfg, err := LoadConfig(ConfigOptions{
		Path: missingPath(t),
		Getenv: envMap(map[string]string{
			"AIRPLAN_ENDPOINT": "env-endpoint",
			"AIRPLAN_BUCKET":   "env-bucket",
		}),
	})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	assertEqual(t, cfg.Profile, "")
	assertEqual(t, cfg.Endpoint, "env-endpoint")
	assertEqual(t, cfg.Bucket, "env-bucket")
	assertEqual(t, cfg.Region, "auto")
}

func TestLoadConfigMalformedFile(t *testing.T) {
	path := writeConfig(t, `bucket = [`, 0o600)

	_, err := LoadConfig(ConfigOptions{
		Path:   path,
		Getenv: envMap(nil),
	})
	assertErrorContains(t, err, "parse config", path)
}

func TestLoadConfigBoolPointerMerging(t *testing.T) {
	path := writeConfig(t, `
endpoint = "root-endpoint"
bucket = "root-bucket"
no_source = true

[profiles.work]
no_source = false
`, 0o600)

	cfg, err := LoadConfig(ConfigOptions{
		Path:    path,
		Profile: "work",
		Getenv:  envMap(nil),
	})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	assertEqual(t, cfg.NoSource, false)
}

func TestLoadConfigCredentialsPermissionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not meaningful on Windows")
	}

	t.Run("warns when credentials file is broadly readable", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "root-endpoint"
bucket = "root-bucket"

[profiles.work]
access_key_id = "profile-access"
`, 0o644)

		cfg, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if len(cfg.Warnings) != 1 {
			t.Fatalf("Warnings length = %d, want 1", len(cfg.Warnings))
		}
		assertContains(t, cfg.Warnings[0], path)
		assertContains(t, cfg.Warnings[0], "credentials")
	})

	t.Run("does not warn when credentials file is private", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "root-endpoint"
bucket = "root-bucket"
access_key_id = "root-access"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, len(cfg.Warnings), 0)
	})

	t.Run("does not warn without credentials", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "root-endpoint"
bucket = "root-bucket"
`, 0o644)

		cfg, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, len(cfg.Warnings), 0)
	})
}

func TestValidateErrorContent(t *testing.T) {
	t.Run("resolved profile", func(t *testing.T) {
		err := (&Config{Profile: "work"}).Validate()
		assertErrorContains(
			t,
			err,
			"endpoint",
			"bucket",
			"profile \"work\" was resolved",
			"--endpoint",
			"--bucket",
			"AIRPLAN_ENDPOINT",
			"AIRPLAN_BUCKET",
			"config file key",
		)
	})

	t.Run("root values", func(t *testing.T) {
		err := (&Config{Endpoint: "set"}).Validate()
		assertErrorContains(
			t,
			err,
			"bucket",
			"root-level values were used",
			"--bucket",
			"AIRPLAN_BUCKET",
		)
	})
}

func TestDefaultConfigPath(t *testing.T) {
	t.Run("prefers XDG_CONFIG_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)

		path, err := DefaultConfigPath()
		if err != nil {
			t.Fatalf("DefaultConfigPath() error = %v", err)
		}
		assertEqual(t, path, filepath.Join(dir, "airplan", "config.toml"))
	})

	t.Run("falls back to os.UserConfigDir", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "")
		setUserConfigDirEnv(t, dir)

		configDir, err := os.UserConfigDir()
		if err != nil {
			t.Fatalf("os.UserConfigDir() error = %v", err)
		}

		path, err := DefaultConfigPath()
		if err != nil {
			t.Fatalf("DefaultConfigPath() error = %v", err)
		}
		assertEqual(
			t,
			path,
			filepath.Join(configDir, "airplan", "config.toml"),
		)
	})
}

func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	contents := []byte(strings.TrimSpace(body) + "\n")
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	return path
}

func missingPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "missing.toml")
}

func envMap(values map[string]string) func(string) string {
	return func(name string) string {
		return values[name]
	}
}

func setUserConfigDirEnv(t *testing.T, dir string) {
	t.Helper()

	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("HOME", dir)
	}
}

func assertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func assertErrorContains(t *testing.T, err error, substrings ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want one containing %q", substrings)
	}
	message := err.Error()
	for _, substring := range substrings {
		assertContains(t, message, substring)
	}
}

func assertContains(t *testing.T, text, substring string) {
	t.Helper()
	if !strings.Contains(text, substring) {
		t.Fatalf("%q does not contain %q", text, substring)
	}
}

func TestLoadConfigForcedProfileWithoutProfiles(t *testing.T) {
	path := writeConfig(t, `
endpoint = "https://example.com"
bucket   = "plans"
`, 0o600)

	_, err := LoadConfig(ConfigOptions{
		Path:    path,
		Profile: "work",
		Getenv:  func(string) string { return "" },
	})
	if err == nil {
		t.Fatal("expected error for profile with no profiles defined")
	}
	if !strings.Contains(err.Error(), `profile "work" does not exist`) {
		t.Errorf("error = %v", err)
	}
}

func TestLoadConfigEnvProfileWithoutConfigFile(t *testing.T) {
	env := map[string]string{"AIRPLAN_PROFILE": "work"}

	_, err := LoadConfig(ConfigOptions{
		Path:   filepath.Join(t.TempDir(), "missing.toml"),
		Getenv: func(k string) string { return env[k] },
	})
	if err == nil {
		t.Fatal("expected error for AIRPLAN_PROFILE with no config file")
	}
	if !strings.Contains(err.Error(), `profile "work" does not exist`) {
		t.Errorf("error = %v", err)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	t.Run("overrides beat env and profile", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "root-endpoint"
bucket   = "root-bucket"
`, 0o600)

		on := true
		cfg, err := LoadConfig(ConfigOptions{
			Path: path,
			Getenv: envMap(map[string]string{
				"AIRPLAN_BUCKET": "env-bucket",
			}),
			Overrides: Settings{
				Bucket:   "flag-bucket",
				NoSource: &on,
			},
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Bucket, "flag-bucket")
		assertEqual(t, cfg.Endpoint, "root-endpoint")
		if !cfg.NoSource {
			t.Error("NoSource override not applied")
		}
	})

	t.Run("explicit false overrides config true", func(t *testing.T) {
		path := writeConfig(t, `
endpoint  = "root-endpoint"
bucket    = "root-bucket"
no_source = true
`, 0o600)

		off := false
		cfg, err := LoadConfig(ConfigOptions{
			Path:      path,
			Getenv:    envMap(nil),
			Overrides: Settings{NoSource: &off},
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if cfg.NoSource {
			t.Error("explicit false override lost to config true")
		}
	})

	t.Run("overrides complete a multi-profile config", func(t *testing.T) {
		// SPEC §7 step 4: connection flags count toward completeness,
		// so a one-off --endpoint/--bucket run works against a config
		// file that defines several profiles and no default.
		path := writeConfig(t, `
[profiles.home]
endpoint = "home-endpoint"
bucket   = "home-bucket"

[profiles.work]
endpoint = "work-endpoint"
bucket   = "work-bucket"
`, 0o600)

		cfg, err := LoadConfig(ConfigOptions{
			Path:   path,
			Getenv: envMap(nil),
			Overrides: Settings{
				Endpoint: "flag-endpoint",
				Bucket:   "flag-bucket",
			},
		})
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		assertEqual(t, cfg.Profile, "")
		assertEqual(t, cfg.Endpoint, "flag-endpoint")
		assertEqual(t, cfg.Bucket, "flag-bucket")
	})
}

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"20s", 20 * time.Second, false},
		{"1m30s", 90 * time.Second, false},
		{"500ms", 500 * time.Millisecond, false},
		{"30", 30 * time.Second, false},
		{"0", 0, false},
		{" 5s ", 5 * time.Second, false},
		{"-1", 0, true},
		{"-5s", 0, true},
		{"bogus", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := parseTimeout(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseTimeout(%q) error = %v, wantErr %v",
				tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseTimeout(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestLoadConfigTimeout(t *testing.T) {
	base := `
endpoint = "e"
bucket   = "b"
`

	t.Run("default", func(t *testing.T) {
		cfg, err := LoadConfig(ConfigOptions{
			Path:   writeConfig(t, base, 0o600),
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Timeout != DefaultTimeout {
			t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
		}
	})

	t.Run("root value", func(t *testing.T) {
		cfg, err := LoadConfig(ConfigOptions{
			Path:   writeConfig(t, base+"timeout = \"45s\"\n", 0o600),
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Timeout != 45*time.Second {
			t.Errorf("Timeout = %v, want 45s", cfg.Timeout)
		}
	})

	t.Run("profile overrides root", func(t *testing.T) {
		cfg, err := LoadConfig(ConfigOptions{
			Path: writeConfig(t, base+`
timeout = "45s"

[profiles.work]
timeout = "90s"
`, 0o600),
			Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Timeout != 90*time.Second {
			t.Errorf("Timeout = %v, want 90s", cfg.Timeout)
		}
	})

	t.Run("env overrides file, flag overrides env", func(t *testing.T) {
		env := map[string]string{"AIRPLAN_TIMEOUT": "10s"}

		cfg, err := LoadConfig(ConfigOptions{
			Path:   writeConfig(t, base+"timeout = \"45s\"\n", 0o600),
			Getenv: envMap(env),
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Timeout != 10*time.Second {
			t.Errorf("env: Timeout = %v, want 10s", cfg.Timeout)
		}

		cfg, err = LoadConfig(ConfigOptions{
			Path:      writeConfig(t, base+"timeout = \"45s\"\n", 0o600),
			Getenv:    envMap(env),
			Overrides: Settings{Timeout: "3s"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Timeout != 3*time.Second {
			t.Errorf("flag: Timeout = %v, want 3s", cfg.Timeout)
		}
	})

	t.Run("zero disables", func(t *testing.T) {
		cfg, err := LoadConfig(ConfigOptions{
			Path:      writeConfig(t, base, 0o600),
			Getenv:    envMap(nil),
			Overrides: Settings{Timeout: "0"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Timeout != 0 {
			t.Errorf("Timeout = %v, want 0", cfg.Timeout)
		}
	})

	t.Run("invalid value errors", func(t *testing.T) {
		_, err := LoadConfig(ConfigOptions{
			Path:   writeConfig(t, base+"timeout = \"bogus\"\n", 0o600),
			Getenv: envMap(nil),
		})
		assertErrorContains(t, err, "invalid timeout \"bogus\"")
	})
}
