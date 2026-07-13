package airplan

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
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
mermaid_url = "https://root.example/mermaid.mjs"
repo = "https://github.com/root/project"
no_external_assets = false
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
mermaid_url = "https://profile.example/mermaid.mjs"
repo = "https://github.com/profile/project"
no_external_assets = true
`, 0o600)

	cfg, err := LoadConfig(ConfigOptions{
		Path:    path,
		Profile: "work",
		Getenv: envMap(map[string]string{
			"AIRPLAN_BUCKET":             "env-bucket",
			"AIRPLAN_REGION":             "env-region",
			"AIRPLAN_SECRET_ACCESS_KEY":  "env-secret",
			"AIRPLAN_PUBLIC_BASE_URL":    "env-public",
			"AIRPLAN_KEY_PREFIX":         "env-prefix",
			"AIRPLAN_TEMPLATE":           "env-template",
			"AIRPLAN_MERMAID_URL":        "https://env.example/mermaid.mjs",
			"AIRPLAN_REPO":               "https://github.com/env/project.git",
			"AIRPLAN_NO_EXTERNAL_ASSETS": "false",
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
	assertEqual(t, cfg.KeyPrefix, "env-prefix")
	assertEqual(t, cfg.Template, "env-template")
	assertEqual(t, cfg.NoSource, true)
	assertEqual(t, cfg.Indexable, false)
	assertEqual(t, cfg.MermaidURL, "https://env.example/mermaid.mjs")
	assertEqual(t, cfg.Repository, "https://github.com/env/project.git")
	assertEqual(t, cfg.NoExternalAssets, false)
}

func TestLoadConfigRepositoryPrecedenceAndDefault(t *testing.T) {
	cfg, err := LoadConfig(ConfigOptions{
		Path: missingPath(t), Getenv: envMap(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, cfg.Repository, "auto")

	path := writeConfig(t, `
repo = "none"

[profiles.work]
repo = "https://github.com/profile/project"
`, 0o600)
	cfg, err = LoadConfig(ConfigOptions{
		Path: path, Profile: "work",
		Getenv: envMap(map[string]string{
			"AIRPLAN_REPO": "https://github.com/env/project",
		}),
		Overrides: Settings{Repository: "https://github.com/flag/project"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, cfg.Repository, "https://github.com/flag/project")
}

func TestLoadConfigMermaidDefaultsAndValidation(t *testing.T) {
	cfg, err := LoadConfig(ConfigOptions{
		Path:   missingPath(t),
		Getenv: envMap(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertEqual(t, cfg.MermaidURL, DefaultMermaidURL)

	_, err = LoadConfig(ConfigOptions{
		Path: missingPath(t),
		Getenv: envMap(map[string]string{
			"AIRPLAN_NO_EXTERNAL_ASSETS": "sometimes",
		}),
	})
	if err == nil {
		t.Fatal("invalid external-assets environment boolean accepted")
	}

	for _, raw := range []string{
		"http://example.com/mermaid.mjs",
		"https://user@example.com/mermaid.mjs",
		"https://example.com/mermaid.mjs#fragment",
	} {
		cfg := &Config{
			Endpoint: "https://s3.example.com", Bucket: "b",
			MermaidURL: raw,
		}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid Mermaid URL %q accepted", raw)
		}
	}
}

func TestLoadConfigEmptyMermaidURLResetsInheritedValue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  string
		profile string
	}{
		{
			name: "empty root value",
			config: `
mermaid_url = ""
`,
		},
		{
			name: "empty profile value",
			config: `
mermaid_url = "https://assets.example.test/mermaid.mjs"

[profiles.work]
mermaid_url = ""
`,
			profile: "work",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := LoadConfig(ConfigOptions{
				Path: writeConfig(t, tc.config, 0o600), Profile: tc.profile,
				Getenv: envMap(nil),
			})
			if err != nil {
				t.Fatal(err)
			}
			assertEqual(t, cfg.MermaidURL, DefaultMermaidURL)
		})
	}
}

func TestResolveMermaidURLOverride(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    string
		explicit bool
		want     string
	}{
		{"unset empty", "", false, ""},
		{"explicit empty", "", true, DefaultMermaidURL},
		{
			"explicit custom",
			"https://assets.example.test/mermaid.mjs",
			true,
			"https://assets.example.test/mermaid.mjs",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveMermaidURLOverride(tc.value, tc.explicit)
			assertEqual(t, got, tc.want)
		})
	}
}

func TestLoadConfigDefaultRegion(t *testing.T) {
	cfg, err := LoadConfig(ConfigOptions{
		Path: missingPath(t),
		Getenv: envMap(map[string]string{
			"AIRPLAN_ENDPOINT": "https://s3.example.com",
			"AIRPLAN_BUCKET":   "env-bucket",
		}),
	})
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	assertEqual(t, cfg.Profile, "")
	assertEqual(t, cfg.Endpoint, "https://s3.example.com")
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

	t.Run("explicit missing option path errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.toml")
		_, err := LoadConfig(ConfigOptions{
			Path: path, Getenv: envMap(nil),
		})
		assertErrorContains(t, err, "config file", strconv.Quote(path),
			"does not exist")
	})

	t.Run("explicit missing environment path errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.toml")
		_, err := LoadConfig(ConfigOptions{Getenv: envMap(map[string]string{
			"AIRPLAN_CONFIG": path,
		})})
		assertErrorContains(t, err, "config file", strconv.Quote(path),
			"does not exist")
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

func TestValidateCredentials(t *testing.T) {
	base := Config{
		Endpoint: "https://s3.example.com",
		Bucket:   "plans",
	}

	t.Run("ambient chain", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("complete explicit pair", func(t *testing.T) {
		cfg := base
		cfg.AccessKeyID = "id"
		cfg.SecretAccessKey = "secret"
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	for _, tt := range []struct {
		name   string
		access string
		secret string
	}{
		{"access key only", "id", ""},
		{"secret key only", "", "secret"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.AccessKeyID = tt.access
			cfg.SecretAccessKey = tt.secret
			err := cfg.Validate()
			assertErrorContains(t, err,
				"access_key_id and secret_access_key",
				"configured together")
		})
	}

	t.Run("mixed precedence may complete pair", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "https://s3.example.com"
bucket = "plans"
access_key_id = "from-file"
`, 0o600)
		cfg, err := LoadConfig(ConfigOptions{
			Path: path,
			Getenv: envMap(map[string]string{
				"AIRPLAN_SECRET_ACCESS_KEY": "from-env",
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.Validate(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("profile partial remains invalid", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.work]
endpoint = "https://s3.example.com"
bucket = "plans"
secret_access_key = "secret-only"
`, 0o600)
		cfg, err := LoadConfig(ConfigOptions{
			Path: path, Profile: "work", Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertErrorContains(t, cfg.Validate(),
			"access_key_id and secret_access_key")
	})

	t.Run("environment partial remains invalid", func(t *testing.T) {
		cfg, err := LoadConfig(ConfigOptions{
			Path: missingPath(t),
			Getenv: envMap(map[string]string{
				"AIRPLAN_ENDPOINT":      "https://s3.example.com",
				"AIRPLAN_BUCKET":        "plans",
				"AIRPLAN_ACCESS_KEY_ID": "id-only",
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertErrorContains(t, cfg.Validate(),
			"access_key_id and secret_access_key")
	})

	t.Run("root file partial remains invalid", func(t *testing.T) {
		path := writeConfig(t, `
endpoint = "https://s3.example.com"
bucket = "plans"
access_key_id = "id-only"
`, 0o600)
		cfg, err := LoadConfig(ConfigOptions{
			Path: path, Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertErrorContains(t, cfg.Validate(),
			"access_key_id and secret_access_key")
	})
}

func TestValidateURLsAndKeyPrefix(t *testing.T) {
	valid := Config{
		Endpoint:      "https://s3.example.com/api/",
		Bucket:        "plans",
		PublicBaseURL: "https://cdn.example.com/shared/plans/",
		KeyPrefix:     "team/Jiméh plans",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	for _, tt := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			"endpoint scheme", func(c *Config) { c.Endpoint = "s3.example.com" },
			"scheme must be http or https",
		},
		{
			"endpoint host", func(c *Config) { c.Endpoint = "https:///api" },
			"host is required",
		},
		{
			"endpoint query", func(c *Config) { c.Endpoint += "?x=1" },
			"query",
		},
		{"public user info", func(c *Config) {
			c.PublicBaseURL = "https://user@cdn.example.com"
		}, "user info"},
		{"empty prefix segment", func(c *Config) {
			c.KeyPrefix = "team//plans"
		}, "path segments"},
		{"dot prefix segment", func(c *Config) {
			c.KeyPrefix = "team/../plans"
		}, "path segments"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			assertErrorContains(t, cfg.Validate(), tt.want)
		})
	}
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

	t.Run("falls back to the platform config directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "")
		setUserConfigDirEnv(t, dir)

		configDir := filepath.Join(dir, ".config")
		if runtime.GOOS == "windows" {
			var err error
			configDir, err = os.UserConfigDir()
			if err != nil {
				t.Fatalf("os.UserConfigDir() error = %v", err)
			}
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	return ""
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
		Path:   missingPath(t),
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

func TestResolveConfigTracksWinningSourcesAndChains(t *testing.T) {
	path := writeConfig(t, `
endpoint       = "https://root.example.com"
bucket         = "same-bucket"
default_profile = "work"

[profiles.work]
bucket = "same-bucket"
`, 0o600)
	env := map[string]string{
		"AIRPLAN_PROFILE": "work",
		"AIRPLAN_BUCKET":  "same-bucket",
	}
	resolution, err := ResolveConfig(ConfigOptions{
		Path:   path,
		Getenv: envMap(env),
		Overrides: Settings{
			Bucket:           "same-bucket",
			NoSource:         boolPointer(false),
			Indexable:        boolPointer(true),
			NoExternalAssets: boolPointer(true),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.ConfigPath.Path != path || !resolution.ConfigPath.Exists ||
		resolution.ConfigPath.Source.Name != "--config" {
		t.Fatalf("config path resolution = %+v", resolution.ConfigPath)
	}
	if resolution.Profile.Name != "work" ||
		resolution.Profile.Source.Name != "AIRPLAN_PROFILE" {
		t.Fatalf("profile resolution = %+v", resolution.Profile)
	}
	bucket := resolution.Fields["bucket"]
	if bucket.Source == nil || bucket.Source.Name != "--bucket" {
		t.Fatalf("bucket source = %+v", bucket.Source)
	}
	wantSources := []string{
		"root.bucket", "profiles.work.bucket", "AIRPLAN_BUCKET", "--bucket",
	}
	if len(bucket.Sources) != len(wantSources) {
		t.Fatalf("bucket sources = %+v", bucket.Sources)
	}
	for i, want := range wantSources {
		if bucket.Sources[i].Name != want {
			t.Fatalf("bucket source %d = %+v, want %q",
				i, bucket.Sources[i], want)
		}
	}
	noSource := resolution.Fields["no_source"]
	if noSource.Source == nil || noSource.Source.Name != "--no-source" ||
		resolution.Config.NoSource {
		t.Fatalf("no_source resolution = %+v, value %v",
			noSource, resolution.Config.NoSource)
	}
	for field, wantValue := range map[string]bool{
		"indexable": true, "no_external_assets": true,
	} {
		resolved := resolution.Fields[field]
		if resolved.Source == nil ||
			resolved.Source.Kind != ConfigSourceOverride {
			t.Fatalf("%s resolution = %+v", field, resolved)
		}
		var value bool
		switch field {
		case "indexable":
			value = resolution.Config.Indexable
		case "no_external_assets":
			value = resolution.Config.NoExternalAssets
		}
		if value != wantValue {
			t.Fatalf("%s value = %v, want %v", field, value, wantValue)
		}
	}
	region := resolution.Fields["region"]
	if region.Source == nil || region.Source.Kind != ConfigSourceBuiltin {
		t.Fatalf("region source = %+v", region.Source)
	}
}

func TestResolveConfigTracksDefaultAndRootProfileSelection(t *testing.T) {
	t.Run("default profile", func(t *testing.T) {
		path := writeConfig(t, `
default_profile = "work"
[profiles.work]
`, 0o600)
		resolution, err := ResolveConfig(ConfigOptions{
			Path: path, Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Profile.Name != "work" ||
			resolution.Profile.Source.Name != "default_profile" ||
			resolution.Profile.Source.Path != path {
			t.Fatalf("profile resolution = %+v", resolution.Profile)
		}
	})

	t.Run("explicit profile", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.home]
[profiles.work]
`, 0o600)
		resolution, err := ResolveConfig(ConfigOptions{
			Path: path, Profile: "home", Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Profile.Name != "home" ||
			resolution.Profile.Source.Name != "--profile" {
			t.Fatalf("profile resolution = %+v", resolution.Profile)
		}
	})

	t.Run("single profile", func(t *testing.T) {
		path := writeConfig(t, "[profiles.only]\n", 0o600)
		resolution, err := ResolveConfig(ConfigOptions{
			Path: path, Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Profile.Name != "only" ||
			resolution.Profile.Source.Name != "single named profile" {
			t.Fatalf("profile resolution = %+v", resolution.Profile)
		}
	})

	t.Run("no profiles", func(t *testing.T) {
		resolution, err := ResolveConfig(ConfigOptions{
			Path: missingPath(t), Getenv: envMap(nil),
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Profile.Name != "" ||
			resolution.Profile.Source.Name != "no named profiles" {
			t.Fatalf("profile resolution = %+v", resolution.Profile)
		}
		if resolution.ConfigPath.Exists ||
			resolution.ConfigPath.Source.Kind != ConfigSourceBuiltin {
			t.Fatalf("config path resolution = %+v", resolution.ConfigPath)
		}
	})

	t.Run("environment config path", func(t *testing.T) {
		path := writeConfig(t, "bucket = \"plans\"\n", 0o600)
		resolution, err := ResolveConfig(ConfigOptions{
			Getenv: envMap(map[string]string{"AIRPLAN_CONFIG": path}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.ConfigPath.Path != path ||
			resolution.ConfigPath.Source.Name != "AIRPLAN_CONFIG" {
			t.Fatalf("config path resolution = %+v", resolution.ConfigPath)
		}
	})

	t.Run("complete root-level resolution", func(t *testing.T) {
		path := writeConfig(t, `
[profiles.home]
[profiles.work]
`, 0o600)
		resolution, err := ResolveConfig(ConfigOptions{
			Path: path, Getenv: envMap(nil),
			Overrides: Settings{
				Endpoint: "https://flag.example.com", Bucket: "plans",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolution.Profile.Name != "" ||
			resolution.Profile.Source.Name != "complete root-level resolution" {
			t.Fatalf("profile resolution = %+v", resolution.Profile)
		}
	})
}

func TestConfigFieldDefinitionsCoverSettings(t *testing.T) {
	want := make(map[string]bool)
	typeOfSettings := reflect.TypeOf(Settings{})
	for i := range typeOfSettings.NumField() {
		name := strings.Split(typeOfSettings.Field(i).Tag.Get("toml"), ",")[0]
		want[name] = true
	}

	got := make(map[string]bool)
	envNames := make(map[string]bool)
	for _, field := range configFieldDefinitions {
		if got[field.name] {
			t.Fatalf("duplicate config field definition %q", field.name)
		}
		got[field.name] = true
		if field.overrideSource == "" {
			t.Fatalf("field %q has no override source", field.name)
		}
		if field.envName != "" {
			if envNames[field.envName] {
				t.Fatalf("duplicate config env name %q", field.envName)
			}
			envNames[field.envName] = true
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config field definitions = %v, want Settings fields %v",
			got, want)
	}
}

func TestResolveConfigLabelsLibraryCredentialOverridesAccurately(t *testing.T) {
	resolution, err := ResolveConfig(ConfigOptions{
		Getenv: envMap(nil),
		Overrides: Settings{
			AccessKeyID: "access", SecretAccessKey: "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{
		"access_key_id":     "ConfigOptions.Overrides.access_key_id",
		"secret_access_key": "ConfigOptions.Overrides.secret_access_key",
	} {
		source := resolution.Fields[field].Source
		if source == nil || source.Name != want {
			t.Fatalf("%s source = %+v, want %q", field, source, want)
		}
	}
}

func boolPointer(value bool) *bool { return &value }

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
		{"18446744074", 0, true},
		{"9223372036854775807", 0, true},
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
		if DefaultTimeout != 30*time.Second {
			t.Fatalf("DefaultTimeout = %v, want 30s", DefaultTimeout)
		}
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

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, `
endpoint = "e"
bucet    = "typo"
`, 0o600)

	_, err := LoadConfig(ConfigOptions{Path: path, Getenv: envMap(nil)})
	assertErrorContains(t, err, "unknown config key", "bucet")
}

func TestListConfigProfilesReturnsSortedInventory(t *testing.T) {
	path := writeConfig(t, `
endpoint = "not a URL"
default_profile = "alpha"

[profiles.zulu]
timeout = "not a duration"

[profiles.alpha]
access_key_id = "access"
`, 0o600)

	result, err := ListConfigProfiles(ConfigProfilesOptions{
		Path: path,
		Getenv: envMap(map[string]string{
			"AIRPLAN_PROFILE":            "missing",
			"AIRPLAN_NO_EXTERNAL_ASSETS": "not-a-boolean",
			"AIRPLAN_TIMEOUT":            "not-a-duration",
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []ConfigProfile{
		{Name: "alpha", Default: true},
		{Name: "zulu", Default: false},
	}
	if !reflect.DeepEqual(result.Profiles, want) {
		t.Fatalf("Profiles = %#v, want %#v", result.Profiles, want)
	}
	if result.Profiles == nil {
		t.Fatal("Profiles is nil")
	}
}

func TestListConfigProfilesDoesNotDefaultEmptyProfileName(t *testing.T) {
	path := writeConfig(t, `[profiles.""]`+"\n", 0o600)

	result, err := ListConfigProfiles(ConfigProfilesOptions{
		Path: path, Getenv: envMap(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []ConfigProfile{{Name: "", Default: false}}
	if !reflect.DeepEqual(result.Profiles, want) {
		t.Fatalf("Profiles = %#v, want %#v", result.Profiles, want)
	}
}

func TestListConfigProfilesPathPrecedenceAndMissingFiles(t *testing.T) {
	envPath := writeConfig(t, "[profiles.environment]\n", 0o600)
	explicitPath := writeConfig(t, "[profiles.explicit]\n", 0o600)

	result, err := ListConfigProfiles(ConfigProfilesOptions{
		Path: explicitPath,
		Getenv: envMap(map[string]string{
			"AIRPLAN_CONFIG": envPath,
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Profiles) != 1 ||
		result.Profiles[0].Name != "explicit" {
		t.Fatalf("Profiles = %#v", result.Profiles)
	}

	result, err = ListConfigProfiles(ConfigProfilesOptions{
		Getenv: envMap(map[string]string{
			"AIRPLAN_CONFIG": envPath,
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Profiles) != 1 ||
		result.Profiles[0].Name != "environment" {
		t.Fatalf("Profiles = %#v", result.Profiles)
	}

	missing := filepath.Join(t.TempDir(), "missing.toml")
	_, err = ListConfigProfiles(ConfigProfilesOptions{
		Path: missing, Getenv: envMap(nil),
	})
	assertErrorContains(t, err, "does not exist", strconv.Quote(missing))

	_, err = ListConfigProfiles(ConfigProfilesOptions{
		Getenv: envMap(map[string]string{"AIRPLAN_CONFIG": missing}),
	})
	assertErrorContains(t, err, "does not exist", strconv.Quote(missing))
}

func TestListConfigProfilesEmptyDefaultAndRootOnlyConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	result, err := ListConfigProfiles(ConfigProfilesOptions{
		Getenv: envMap(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Profiles == nil || len(result.Profiles) != 0 {
		t.Fatalf("Profiles = %#v, want non-nil empty slice", result.Profiles)
	}

	result, err = ListConfigProfiles(ConfigProfilesOptions{
		Path:   writeConfig(t, "bucket = \"plans\"\n", 0o600),
		Getenv: envMap(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Profiles == nil || len(result.Profiles) != 0 {
		t.Fatalf("Profiles = %#v, want non-nil empty slice", result.Profiles)
	}
}

func TestListConfigProfilesReturnsConfigFileErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  string
		wantErr string
	}{
		{
			name: "malformed TOML", config: "[profiles.work\n",
			wantErr: "parse config",
		},
		{
			name: "unknown key", config: "bucet = \"plans\"\n",
			wantErr: "unknown config key",
		},
		{
			name: "dangling default",
			config: `
default_profile = "missing"
[profiles.work]
`,
			wantErr: "default_profile \"missing\" does not exist",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ListConfigProfiles(ConfigProfilesOptions{
				Path:   writeConfig(t, tc.config, 0o600),
				Getenv: envMap(nil),
			})
			assertErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestListConfigProfilesWarnsForReadableCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission warning does not apply on Windows")
	}
	path := writeConfig(t, `
[profiles.work]
secret_access_key = "secret-sentinel"
`, 0o644)

	result, err := ListConfigProfiles(ConfigProfilesOptions{
		Path: path, Getenv: envMap(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 ||
		!strings.Contains(result.Warnings[0], "contains credentials") {
		t.Fatalf("Warnings = %#v", result.Warnings)
	}
	if strings.Contains(result.Warnings[0], "secret-sentinel") {
		t.Fatalf("warning leaked credential: %q", result.Warnings[0])
	}
}
