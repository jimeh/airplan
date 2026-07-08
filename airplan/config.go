package airplan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

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
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	path := opts.Path
	if path == "" {
		path = getenv("AIRPLAN_CONFIG")
	}
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	fileConfig, meta, loaded, err := loadFileConfig(path)
	if err != nil {
		return nil, err
	}

	profile, err := resolveProfile(opts.Profile, getenv, fileConfig, meta)
	if err != nil {
		return nil, err
	}

	cfg := &Config{Region: "auto", Profile: profile}
	applySettings(cfg, fileConfig.Settings, rootKeyDefined(meta, loaded))
	if profile != "" {
		applySettings(
			cfg,
			fileConfig.Profiles[profile],
			profileKeyDefined(meta, profile),
		)
	}
	applyEnv(cfg, getenv)

	if loaded {
		warnReadableCredentials(cfg, path, fileConfig)
	}

	return cfg, nil
}

// Validate checks that the configuration is complete enough to upload
// (endpoint and bucket present). The error names the missing field,
// which profile was resolved (or that root-level values were used),
// and the three ways to set it (SPEC.md §7).
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("airplan: config is nil")
	}

	var missing []string
	if c.Endpoint == "" {
		missing = append(missing, "endpoint")
	}
	if c.Bucket == "" {
		missing = append(missing, "bucket")
	}
	if len(missing) == 0 {
		return nil
	}

	where := "root-level values were used"
	if c.Profile != "" {
		where = fmt.Sprintf("profile %q was resolved", c.Profile)
	}

	return fmt.Errorf(
		"airplan: missing required config field(s) %s; %s; "+
			"set via flag(s) %s, env var(s) %s, or config file key(s) %s",
		strings.Join(missing, ", "),
		where,
		flagNames(missing),
		envNames(missing),
		strings.Join(missing, ", "),
	)
}

// DefaultConfigPath returns the platform default config file location
// (SPEC.md §7): $XDG_CONFIG_HOME/airplan/config.toml, defaulting to
// ~/.config on every platform except Windows, which uses the
// platform-appropriate config directory. Notably this includes macOS —
// ~/.config, not ~/Library/Application Support.
func DefaultConfigPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "airplan", "config.toml"), nil
	}

	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("airplan: default config path: %w", err)
		}
		return filepath.Join(dir, "airplan", "config.toml"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("airplan: default config path: %w", err)
	}

	return filepath.Join(home, ".config", "airplan", "config.toml"), nil
}

func loadFileConfig(path string) (FileConfig, toml.MetaData, bool, error) {
	var fileConfig FileConfig

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileConfig, toml.MetaData{}, false, nil
		}
		return fileConfig, toml.MetaData{}, false,
			fmt.Errorf("airplan: read config %s: %w", path, err)
	}

	meta, err := toml.DecodeFile(path, &fileConfig)
	if err != nil {
		return fileConfig, toml.MetaData{}, false,
			fmt.Errorf("airplan: parse config %s: %w", path, err)
	}
	if fileConfig.Profiles == nil {
		fileConfig.Profiles = map[string]Settings{}
	}

	return fileConfig, meta, true, nil
}

func resolveProfile(
	option string,
	getenv func(string) string,
	fileConfig FileConfig,
	meta toml.MetaData,
) (string, error) {
	names := profileNames(fileConfig.Profiles)

	// An explicitly requested profile must exist even when the config
	// file defines no profiles at all (SPEC.md §7, resolution step 1).
	selected := option
	if selected == "" {
		selected = getenv("AIRPLAN_PROFILE")
	}
	if selected != "" {
		if _, ok := fileConfig.Profiles[selected]; ok {
			return selected, nil
		}
		available := "none defined"
		if len(names) > 0 {
			available = strings.Join(names, ", ")
		}
		return "", fmt.Errorf(
			"airplan: profile %q does not exist; available profiles: %s",
			selected,
			available,
		)
	}

	// A dangling default_profile is likewise an error regardless of
	// how many profiles exist (SPEC.md §7, resolution step 2).
	if fileConfig.DefaultProfile != "" {
		if _, ok := fileConfig.Profiles[fileConfig.DefaultProfile]; ok {
			return fileConfig.DefaultProfile, nil
		}
		available := "none defined"
		if len(names) > 0 {
			available = strings.Join(names, ", ")
		}
		return "", fmt.Errorf(
			"airplan: default_profile %q does not exist; "+
				"available profiles: %s",
			fileConfig.DefaultProfile,
			available,
		)
	}

	if len(names) == 0 {
		return "", nil
	}

	if len(names) == 1 {
		return names[0], nil
	}

	rootCfg := &Config{Region: "auto"}
	applySettings(rootCfg, fileConfig.Settings, rootKeyDefined(meta, true))
	applyEnv(rootCfg, getenv)
	if rootCfg.Endpoint != "" && rootCfg.Bucket != "" {
		return "", nil
	}

	return "", fmt.Errorf(
		"airplan: no profile selected and root-level values are "+
			"incomplete; available profiles: %s",
		strings.Join(names, ", "),
	)
}

func profileNames(profiles map[string]Settings) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func rootKeyDefined(meta toml.MetaData, loaded bool) func(string) bool {
	return func(key string) bool {
		return loaded && meta.IsDefined(key)
	}
}

func profileKeyDefined(
	meta toml.MetaData,
	profile string,
) func(string) bool {
	return func(key string) bool {
		return meta.IsDefined("profiles", profile, key)
	}
}

func applySettings(
	cfg *Config,
	settings Settings,
	defined func(string) bool,
) {
	if defined("endpoint") {
		cfg.Endpoint = settings.Endpoint
	}
	if defined("bucket") {
		cfg.Bucket = settings.Bucket
	}
	if defined("region") {
		cfg.Region = settings.Region
	}
	if defined("access_key_id") {
		cfg.AccessKeyID = settings.AccessKeyID
	}
	if defined("secret_access_key") {
		cfg.SecretAccessKey = settings.SecretAccessKey
	}
	if defined("public_base_url") {
		cfg.PublicBaseURL = settings.PublicBaseURL
	}
	if defined("key_prefix") {
		cfg.KeyPrefix = settings.KeyPrefix
	}
	if defined("template") {
		cfg.Template = settings.Template
	}
	if settings.NoSource != nil {
		cfg.NoSource = *settings.NoSource
	}
	if settings.Indexable != nil {
		cfg.Indexable = *settings.Indexable
	}
}

func applyEnv(cfg *Config, getenv func(string) string) {
	applyEnvString(&cfg.Endpoint, getenv, "AIRPLAN_ENDPOINT")
	applyEnvString(&cfg.Bucket, getenv, "AIRPLAN_BUCKET")
	applyEnvString(&cfg.Region, getenv, "AIRPLAN_REGION")
	applyEnvString(&cfg.AccessKeyID, getenv, "AIRPLAN_ACCESS_KEY_ID")
	applyEnvString(&cfg.SecretAccessKey, getenv, "AIRPLAN_SECRET_ACCESS_KEY")
	applyEnvString(&cfg.PublicBaseURL, getenv, "AIRPLAN_PUBLIC_BASE_URL")
	applyEnvString(&cfg.KeyPrefix, getenv, "AIRPLAN_KEY_PREFIX")
	applyEnvString(&cfg.Template, getenv, "AIRPLAN_TEMPLATE")
}

func applyEnvString(field *string, getenv func(string) string, name string) {
	if value := getenv(name); value != "" {
		*field = value
	}
}

func warnReadableCredentials(
	cfg *Config,
	path string,
	fileConfig FileConfig,
) {
	if runtime.GOOS == "windows" || !containsCredentials(fileConfig) {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Mode().Perm()&0o077 == 0 {
		return
	}

	cfg.Warnings = append(
		cfg.Warnings,
		fmt.Sprintf(
			"config file %s contains credentials and is group- or "+
				"world-readable",
			path,
		),
	)
}

func containsCredentials(fileConfig FileConfig) bool {
	if fileConfig.AccessKeyID != "" || fileConfig.SecretAccessKey != "" {
		return true
	}
	for _, profile := range fileConfig.Profiles {
		if profile.AccessKeyID != "" || profile.SecretAccessKey != "" {
			return true
		}
	}
	return false
}

func flagNames(fields []string) string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, "--"+strings.ReplaceAll(field, "_", "-"))
	}
	return strings.Join(names, ", ")
}

func envNames(fields []string) string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, "AIRPLAN_"+strings.ToUpper(field))
	}
	return strings.Join(names, ", ")
}
