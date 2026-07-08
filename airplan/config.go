package airplan

import "errors"

// Settings holds every connection and behavior key that may appear at
// the root level of the config file or inside a [profiles.*] table
// (SPEC.md §7). Boolean fields are pointers so profile merging can
// distinguish "unset" from "false".
type Settings struct {
	Endpoint        string `toml:"endpoint"`
	Bucket          string `toml:"bucket"`
	Region          string `toml:"region"`
	AccessKeyID     string `toml:"access_key_id"`
	SecretAccessKey string `toml:"secret_access_key"`
	PublicBaseURL   string `toml:"public_base_url"`
	KeyPrefix       string `toml:"key_prefix"`
	Template        string `toml:"template"`
	NoSource        *bool  `toml:"no_source"`
	Indexable       *bool  `toml:"indexable"`
}

// FileConfig is the on-disk shape of the TOML config file: shared
// root-level Settings plus named profiles that override them
// (SPEC.md §7).
type FileConfig struct {
	Settings
	DefaultProfile string              `toml:"default_profile"`
	Profiles       map[string]Settings `toml:"profiles"`
}

// Config is a fully resolved configuration: config file, environment
// variables, and built-in defaults merged per the precedence rules of
// SPEC.md §7. Flag overrides are overlaid by the caller afterwards,
// then completeness is checked with Validate.
type Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	PublicBaseURL   string
	KeyPrefix       string
	Template        string
	NoSource        bool
	Indexable       bool

	// Profile is the resolved profile name, or "" when root-level
	// values were used.
	Profile string

	// Warnings collects non-fatal issues found during load (e.g. a
	// group/world-readable config file that contains credentials) for
	// the caller to print to stderr.
	Warnings []string
}

// ConfigOptions controls LoadConfig.
type ConfigOptions struct {
	// Path of the config file. "" means the platform default
	// ($XDG_CONFIG_HOME/airplan/config.toml; the platform-appropriate
	// config directory on Windows). A missing file is not an error —
	// env vars and flags may fully configure the tool.
	Path string

	// Profile forces a profile by name (--profile / AIRPLAN_PROFILE).
	// "" applies the resolution order of SPEC.md §7.
	Profile string

	// Getenv is the environment lookup, injectable for tests.
	// nil means os.Getenv.
	Getenv func(string) string
}

// LoadConfig reads the config file (if present), applies AIRPLAN_* env
// vars, resolves the profile per SPEC.md §7, and returns the merged
// result. It does not check completeness — callers overlay flag values
// first, then Validate reports what is still missing.
func LoadConfig(opts ConfigOptions) (*Config, error) {
	return nil, errors.New("airplan: LoadConfig not implemented")
}

// Validate checks that the configuration is complete enough to upload
// (endpoint and bucket present). The error names the missing field,
// which profile was resolved (or that root-level values were used),
// and the three ways to set it (SPEC.md §7).
func (c *Config) Validate() error {
	return errors.New("airplan: Validate not implemented")
}

// DefaultConfigPath returns the platform default config file location.
func DefaultConfigPath() (string, error) {
	return "", errors.New("airplan: DefaultConfigPath not implemented")
}
