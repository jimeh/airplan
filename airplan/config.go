package airplan

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// DefaultTimeout is the default operation timeout (SPEC.md §6).
const DefaultTimeout = 30 * time.Second

type configFieldDefinition struct {
	name           string
	builtin        bool
	envName        string
	overrideSource string
}

var configFieldDefinitions = []configFieldDefinition{
	{
		name: "endpoint", envName: "AIRPLAN_ENDPOINT",
		overrideSource: "--endpoint",
	},
	{
		name: "bucket", envName: "AIRPLAN_BUCKET",
		overrideSource: "--bucket",
	},
	{
		name: "region", builtin: true, envName: "AIRPLAN_REGION",
		overrideSource: "--region",
	},
	{
		name: "access_key_id", envName: "AIRPLAN_ACCESS_KEY_ID",
		overrideSource: "ConfigOptions.Overrides.access_key_id",
	},
	{
		name: "secret_access_key", envName: "AIRPLAN_SECRET_ACCESS_KEY",
		overrideSource: "ConfigOptions.Overrides.secret_access_key",
	},
	{
		name: "public_base_url", envName: "AIRPLAN_PUBLIC_BASE_URL",
		overrideSource: "--public-base-url",
	},
	{
		name: "key_prefix", envName: "AIRPLAN_KEY_PREFIX",
		overrideSource: "--key-prefix",
	},
	{
		name: "template", envName: "AIRPLAN_TEMPLATE",
		overrideSource: "--template",
	},
	{name: "no_source", builtin: true, overrideSource: "--no-source"},
	{name: "indexable", builtin: true, overrideSource: "--indexable"},
	{
		name: "no_external_assets", builtin: true,
		envName:        "AIRPLAN_NO_EXTERNAL_ASSETS",
		overrideSource: "--no-external-assets",
	},
	{
		name: "mermaid_url", builtin: true, envName: "AIRPLAN_MERMAID_URL",
		overrideSource: "--mermaid-url",
	},
	{
		name: "repo", builtin: true, envName: "AIRPLAN_REPO",
		overrideSource: "--repo",
	},
	{
		name: "timeout", builtin: true, envName: "AIRPLAN_TIMEOUT",
		overrideSource: "--timeout",
	},
}

// Settings holds every connection and behavior key that may appear at
// the root level of the config file or inside a [profiles.*] table
// (SPEC.md §7). Boolean fields are pointers so profile merging can
// distinguish "unset" from "false".
type Settings struct {
	Endpoint         string `toml:"endpoint" json:"endpoint,omitempty" jsonschema_description:"Absolute HTTP(S) S3-compatible API endpoint URL; path prefixes are allowed."`
	Bucket           string `toml:"bucket" json:"bucket,omitempty" jsonschema_description:"Bucket where rendered plans are uploaded."`
	Region           string `toml:"region" json:"region,omitempty" jsonschema_description:"S3 signing region; R2 commonly uses auto."`
	AccessKeyID      string `toml:"access_key_id" json:"access_key_id,omitempty" jsonschema_description:"Access key ID for explicit credentials; must be paired with secret_access_key."`
	SecretAccessKey  string `toml:"secret_access_key" json:"secret_access_key,omitempty" jsonschema_description:"Secret for explicit credentials; must be paired with access_key_id."`
	PublicBaseURL    string `toml:"public_base_url" json:"public_base_url,omitempty" jsonschema_description:"Absolute HTTP(S) public base URL used to assemble share links; path prefixes are allowed."`
	KeyPrefix        string `toml:"key_prefix" json:"key_prefix,omitempty" jsonschema_description:"UTF-8 path segments prepended to uploaded keys; empty, dot, and dot-dot segments are rejected."`
	Template         string `toml:"template" json:"template,omitempty" jsonschema_description:"Path to the HTML template used for rendered pages."`
	NoSource         *bool  `toml:"no_source" json:"no_source,omitempty" jsonschema_description:"Omit uploading the original source alongside rendered output."`
	Indexable        *bool  `toml:"indexable" json:"indexable,omitempty" jsonschema_description:"Allow search indexing by omitting the noindex robots meta tag."`
	NoExternalAssets *bool  `toml:"no_external_assets" json:"no_external_assets,omitempty" jsonschema_description:"Disable airplan-managed features that load external assets when a page is viewed."`
	// MermaidURL overrides the Mermaid module URL. Callers representing an
	// explicit empty override must use ResolveMermaidURLOverride; a bare empty
	// value is treated as unset when Settings are overlaid.
	MermaidURL string `toml:"mermaid_url" json:"mermaid_url,omitempty" jsonschema_description:"Absolute HTTPS URL of the Mermaid ECMAScript module."`
	Repository string `toml:"repo" json:"repo,omitempty" jsonschema_description:"Repository context: auto, none, or an explicit GitHub-compatible repository URL."`
	Timeout    string `toml:"timeout" json:"timeout,omitempty" jsonschema_description:"Operation timeout as a Go duration or seconds; 0 disables it."`
}

// FileConfig is the on-disk shape of the TOML config file: shared
// root-level Settings plus named profiles that override them
// (SPEC.md §7).
type FileConfig struct {
	Settings
	DefaultProfile string              `toml:"default_profile" json:"default_profile,omitempty" jsonschema_description:"Profile selected when no explicit profile is set."`
	Profiles       map[string]Settings `toml:"profiles" json:"profiles,omitempty" jsonschema_description:"Named profiles that inherit and override root-level settings."`
}

// Config is a fully resolved configuration: config file, environment
// variables, and built-in defaults merged per the precedence rules of
// SPEC.md §7. Flag overrides are overlaid by the caller afterwards,
// then completeness is checked with Validate.
type Config struct {
	Endpoint         string
	Bucket           string
	Region           string
	AccessKeyID      string
	SecretAccessKey  string
	PublicBaseURL    string
	KeyPrefix        string
	Template         string
	NoSource         bool
	Indexable        bool
	NoExternalAssets bool
	MermaidURL       string
	Repository       string

	// Timeout bounds one context-aware operation or phase (SPEC.md §6):
	// default 30 seconds, 0 means no timeout. The CLI applies it to its
	// contexts; library consumers manage their own contexts and may
	// ignore it.
	Timeout time.Duration

	// DisableManifest turns off local manifest recording (SPEC.md §9).
	// Code-only — deliberately not a config file key: the CLI always
	// records; the opt-out exists for library consumers that don't
	// want writes under the user's state directory.
	DisableManifest bool

	// ManifestPath overrides the manifest location ("" = platform
	// default). Code-only, mainly for tests and embedders.
	ManifestPath string

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
	// Path of the config file. An explicitly named missing file is an
	// error. "" means AIRPLAN_CONFIG, then the platform default
	// ($XDG_CONFIG_HOME/airplan/config.toml; the platform-appropriate
	// config directory on Windows). Only a missing default file is not
	// an error — env vars and flags may fully configure the tool.
	Path string

	// Profile forces a profile by name (--profile / AIRPLAN_PROFILE).
	// "" applies the resolution order of SPEC.md §7.
	Profile string

	// Getenv is the environment lookup, injectable for tests.
	// nil means os.Getenv.
	Getenv func(string) string

	// Overrides sit at the top of the precedence order (SPEC.md §7) —
	// typically CLI flag values. Empty strings and nil bools mean
	// "not set". They also count toward the profile-resolution
	// completeness check, so one-off --endpoint/--bucket invocations
	// work against a config file that defines multiple profiles.
	Overrides Settings
}

// ConfigProfilesOptions controls ListConfigProfiles.
type ConfigProfilesOptions struct {
	// Path of the config file. An explicitly named missing file is an
	// error. "" means AIRPLAN_CONFIG, then the platform default.
	Path string

	// Getenv is the environment lookup used for AIRPLAN_CONFIG,
	// injectable for tests. nil means os.Getenv. Other environment
	// variables do not affect profile inventory.
	Getenv func(string) string
}

// ConfigProfile describes one named profile in an airplan config file.
type ConfigProfile struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// ConfigProfilesResult is the profile inventory and non-fatal load warnings.
type ConfigProfilesResult struct {
	Profiles []ConfigProfile
	Warnings []string
}

// ConfigSourceKind identifies one layer in config resolution.
type ConfigSourceKind string

const (
	ConfigSourceBuiltin  ConfigSourceKind = "builtin"
	ConfigSourceRoot     ConfigSourceKind = "config_root"
	ConfigSourceProfile  ConfigSourceKind = "config_profile"
	ConfigSourceEnv      ConfigSourceKind = "environment"
	ConfigSourceOverride ConfigSourceKind = "override"
	ConfigSourceInferred ConfigSourceKind = "inferred"
)

// ConfigSource describes where a resolved config value came from.
type ConfigSource struct {
	Kind    ConfigSourceKind `json:"kind"`
	Name    string           `json:"name"`
	Path    string           `json:"path,omitempty"`
	Profile string           `json:"profile,omitempty"`
}

// FieldResolution records the winning source and the complete low-to-high
// precedence source chain for one config field. It deliberately carries no
// values, so shadowed credentials are never duplicated into diagnostics.
type FieldResolution struct {
	Source  *ConfigSource  `json:"source,omitempty"`
	Sources []ConfigSource `json:"sources,omitempty"`
}

// ConfigPathResolution describes the selected config path.
type ConfigPathResolution struct {
	Path   string       `json:"path"`
	Exists bool         `json:"exists"`
	Source ConfigSource `json:"source"`
}

// ProfileResolution describes the selected profile and why it was selected.
// Name is empty when root-level settings are active.
type ProfileResolution struct {
	Name   string       `json:"name,omitempty"`
	Source ConfigSource `json:"source"`
}

// ConfigResolution is a resolved Config plus field-level provenance.
type ConfigResolution struct {
	Config     *Config                    `json:"-"`
	ConfigPath ConfigPathResolution       `json:"config_file"`
	Profile    ProfileResolution          `json:"profile"`
	Fields     map[string]FieldResolution `json:"fields"`
}

// LoadConfig reads the config file (if present), applies AIRPLAN_* env
// vars, resolves the profile per SPEC.md §7, and returns the merged
// result. It does not check completeness — callers overlay flag values
// first, then Validate reports what is still missing.
func LoadConfig(opts ConfigOptions) (*Config, error) {
	resolution, err := ResolveConfig(opts)
	if err != nil {
		return nil, err
	}
	return resolution.Config, nil
}

// ListConfigProfiles reads the selected config file and returns its named
// profiles in lexical order. It does not resolve an active profile, merge
// environment settings, validate field values, or access credentials or
// storage.
func ListConfigProfiles(
	opts ConfigProfilesOptions,
) (*ConfigProfilesResult, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	configPath, explicitPath, err := resolveConfigPath(opts.Path, getenv)
	if err != nil {
		return nil, err
	}
	fileConfig, _, loaded, err := loadFileConfig(configPath.Path)
	if err != nil {
		return nil, err
	}
	if explicitPath && !loaded {
		return nil, fmt.Errorf(
			"airplan: config file %q does not exist", configPath.Path,
		)
	}
	if err := validateDefaultProfile(fileConfig); err != nil {
		return nil, err
	}

	profiles := make([]ConfigProfile, 0, len(fileConfig.Profiles))
	for _, name := range profileNames(fileConfig.Profiles) {
		profiles = append(profiles, ConfigProfile{
			Name: name, Default: name == fileConfig.DefaultProfile,
		})
	}
	warnings := make([]string, 0, 1)
	if loaded {
		warnings = append(
			warnings,
			readableCredentialWarnings(configPath.Path, fileConfig)...,
		)
	}

	return &ConfigProfilesResult{
		Profiles: profiles,
		Warnings: warnings,
	}, nil
}

// ResolveConfig performs the same merge as LoadConfig while retaining the
// winning source and complete source chain for every field.
func ResolveConfig(opts ConfigOptions) (*ConfigResolution, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}

	configPath, explicitPath, err := resolveConfigPath(opts.Path, getenv)
	if err != nil {
		return nil, err
	}
	path := configPath.Path

	fileConfig, meta, loaded, err := loadFileConfig(path)
	if err != nil {
		return nil, err
	}
	if explicitPath && !loaded {
		return nil, fmt.Errorf("airplan: config file %q does not exist", path)
	}

	profile, err := resolveProfile(opts, getenv, fileConfig, meta)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Region:     "auto",
		Profile:    profile,
		MermaidURL: DefaultMermaidURL,
		Repository: "auto",
	}
	applySettings(cfg, fileConfig.Settings, rootKeyDefined(meta, loaded))
	if profile != "" {
		applySettings(
			cfg,
			fileConfig.Profiles[profile],
			profileKeyDefined(meta, profile),
		)
	}
	if err := applyEnv(cfg, getenv); err != nil {
		return nil, err
	}
	applyOverrides(cfg, opts.Overrides)

	if err := resolveTimeout(cfg, opts, getenv, fileConfig,
		meta, loaded, profile); err != nil {
		return nil, err
	}

	if loaded {
		warnReadableCredentials(cfg, path, fileConfig)
	}

	return &ConfigResolution{
		Config: cfg,
		ConfigPath: ConfigPathResolution{
			Path: path, Exists: loaded, Source: configPath.Source,
		},
		Profile: profileResolution(opts, getenv, fileConfig, profile, path),
		Fields: configFieldResolutions(
			opts, getenv, meta, loaded, profile, path,
		),
	}, nil
}

// resolveTimeout applies the timeout precedence chain (SPEC.md §6,
// §7): flag override > AIRPLAN_TIMEOUT > profile > root > the 30s
// default. Timeout is kept as a string through merging so the winning
// value is parsed — and rejected — exactly once.
func resolveTimeout(
	cfg *Config,
	opts ConfigOptions,
	getenv func(string) string,
	fileConfig FileConfig,
	meta toml.MetaData,
	loaded bool,
	profile string,
) error {
	raw := ""
	if loaded && meta.IsDefined("timeout") {
		raw = fileConfig.Timeout
	}
	if profile != "" && meta.IsDefined("profiles", profile, "timeout") {
		raw = fileConfig.Profiles[profile].Timeout
	}
	if v := getenv("AIRPLAN_TIMEOUT"); v != "" {
		raw = v
	}
	if opts.Overrides.Timeout != "" {
		raw = opts.Overrides.Timeout
	}

	cfg.Timeout = DefaultTimeout
	if raw == "" {
		return nil
	}
	d, err := parseTimeout(raw)
	if err != nil {
		return err
	}
	cfg.Timeout = d
	return nil
}

func profileResolution(
	opts ConfigOptions,
	getenv func(string) string,
	fileConfig FileConfig,
	profile string,
	path string,
) ProfileResolution {
	source := ConfigSource{Kind: ConfigSourceInferred}
	switch {
	case opts.Profile != "":
		source = ConfigSource{Kind: ConfigSourceOverride, Name: "--profile"}
	case getenv("AIRPLAN_PROFILE") != "":
		source = ConfigSource{
			Kind: ConfigSourceEnv, Name: "AIRPLAN_PROFILE",
		}
	case fileConfig.DefaultProfile != "":
		source = ConfigSource{
			Kind: ConfigSourceRoot, Name: "default_profile", Path: path,
		}
	case len(fileConfig.Profiles) == 1:
		source.Name = "single named profile"
	case len(fileConfig.Profiles) == 0:
		source.Name = "no named profiles"
	default:
		source.Name = "complete root-level resolution"
	}
	return ProfileResolution{Name: profile, Source: source}
}

func configFieldResolutions(
	opts ConfigOptions,
	getenv func(string) string,
	meta toml.MetaData,
	loaded bool,
	profile string,
	path string,
) map[string]FieldResolution {
	chains := make(map[string][]ConfigSource, len(configFieldDefinitions))
	for _, field := range configFieldDefinitions {
		if field.builtin {
			chains[field.name] = append(chains[field.name], ConfigSource{
				Kind: ConfigSourceBuiltin, Name: "built-in default",
			})
		}
	}
	if loaded {
		for _, field := range configFieldDefinitions {
			if meta.IsDefined(field.name) {
				chains[field.name] = append(chains[field.name], ConfigSource{
					Kind: ConfigSourceRoot,
					Name: "root." + field.name,
					Path: path,
				})
			}
		}
	}
	if profile != "" {
		for _, field := range configFieldDefinitions {
			if meta.IsDefined("profiles", profile, field.name) {
				chains[field.name] = append(
					chains[field.name], ConfigSource{
						Kind:    ConfigSourceProfile,
						Name:    "profiles." + profile + "." + field.name,
						Path:    path,
						Profile: profile,
					})
			}
		}
	}

	for _, field := range configFieldDefinitions {
		if field.envName != "" && getenv(field.envName) != "" {
			chains[field.name] = append(chains[field.name], ConfigSource{
				Kind: ConfigSourceEnv, Name: field.envName,
			})
		}
	}

	for _, field := range overrideFields(opts.Overrides) {
		chains[field.name] = append(chains[field.name], ConfigSource{
			Kind: ConfigSourceOverride,
			Name: field.overrideSource,
		})
	}

	fields := make(map[string]FieldResolution, len(configFieldDefinitions))
	for _, definition := range configFieldDefinitions {
		sources := chains[definition.name]
		field := FieldResolution{Sources: sources}
		if len(sources) > 0 {
			winning := sources[len(sources)-1]
			field.Source = &winning
		}
		fields[definition.name] = field
	}
	return fields
}

func overrideFields(settings Settings) []configFieldDefinition {
	var fields []configFieldDefinition
	for _, field := range configFieldDefinitions {
		if configOverrideIsSet(settings, field.name) {
			fields = append(fields, field)
		}
	}
	return fields
}

func configOverrideIsSet(settings Settings, name string) bool {
	switch name {
	case "endpoint":
		return settings.Endpoint != ""
	case "bucket":
		return settings.Bucket != ""
	case "region":
		return settings.Region != ""
	case "access_key_id":
		return settings.AccessKeyID != ""
	case "secret_access_key":
		return settings.SecretAccessKey != ""
	case "public_base_url":
		return settings.PublicBaseURL != ""
	case "key_prefix":
		return settings.KeyPrefix != ""
	case "template":
		return settings.Template != ""
	case "no_source":
		return settings.NoSource != nil
	case "indexable":
		return settings.Indexable != nil
	case "no_external_assets":
		return settings.NoExternalAssets != nil
	case "mermaid_url":
		return settings.MermaidURL != ""
	case "repo":
		return settings.Repository != ""
	case "timeout":
		return settings.Timeout != ""
	default:
		return false
	}
}

// parseTimeout parses a timeout value (SPEC.md §6): a Go duration
// string ("30s", "1m30s") or a bare integer meaning seconds. 0
// disables the timeout.
func parseTimeout(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)

	var d time.Duration
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf(
				"airplan: invalid timeout %q (must not be negative)", s,
			)
		}
		if n > math.MaxInt64/int64(time.Second) {
			return 0, fmt.Errorf(
				"airplan: invalid timeout %q (out of range)", s,
			)
		}
		d = time.Duration(n) * time.Second
	} else if d, err = time.ParseDuration(s); err != nil {
		return 0, fmt.Errorf(
			"airplan: invalid timeout %q (examples: 30s, 1m30s, 0)", s,
		)
	}

	if d < 0 {
		return 0, fmt.Errorf(
			"airplan: invalid timeout %q (must not be negative)", s,
		)
	}
	return d, nil
}

// Validate checks configuration completeness and correctness for an
// upload (SPEC.md §7).
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
	if len(missing) > 0 {
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

	if (c.AccessKeyID == "") != (c.SecretAccessKey == "") {
		return errors.New(
			"airplan: access_key_id and secret_access_key must be " +
				"configured together, or both omitted to use the " +
				"standard AWS credential chain",
		)
	}
	if err := validateHTTPURL("endpoint", c.Endpoint); err != nil {
		return err
	}
	if c.PublicBaseURL != "" {
		if err := validateHTTPURL("public_base_url", c.PublicBaseURL); err != nil {
			return err
		}
	}
	if err := validateKeyPrefix(c.KeyPrefix); err != nil {
		return err
	}
	if c.MermaidURL != "" {
		if err := validateMermaidURL(c.MermaidURL); err != nil {
			return err
		}
	}
	if c.Repository != "" && c.Repository != "auto" &&
		c.Repository != "none" {
		if _, err := NormalizeRepositoryURL(c.Repository); err != nil {
			return err
		}
	}
	return nil
}

func validateMermaidURL(raw string) error {
	if !utf8.ValidString(raw) {
		return errors.New("airplan: mermaid_url must be valid UTF-8")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("airplan: invalid mermaid_url %q: %w", raw, err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf(
			"airplan: invalid mermaid_url %q: absolute HTTPS URL required",
			raw,
		)
	}
	if u.User != nil || u.Fragment != "" {
		return fmt.Errorf(
			"airplan: invalid mermaid_url %q: user info and fragment are not allowed",
			raw,
		)
	}
	return nil
}

func resolveMermaidURL(raw string) (string, error) {
	if raw == "" {
		raw = DefaultMermaidURL
	}
	if err := validateMermaidURL(raw); err != nil {
		return "", err
	}
	return raw, nil
}

// ResolveMermaidURLOverride normalizes an explicit empty override to the
// built-in Mermaid module URL. An unset or non-empty value is unchanged.
func ResolveMermaidURLOverride(value string, explicit bool) string {
	if explicit && value == "" {
		return DefaultMermaidURL
	}
	return value
}

func validateHTTPURL(name, raw string) error {
	if !utf8.ValidString(raw) {
		return fmt.Errorf("airplan: %s must be valid UTF-8", name)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("airplan: invalid %s %q: %w", name, raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf(
			"airplan: invalid %s %q: scheme must be http or https",
			name, raw,
		)
	}
	if u.Host == "" {
		return fmt.Errorf(
			"airplan: invalid %s %q: host is required", name, raw,
		)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf(
			"airplan: invalid %s %q: user info, query, and fragment "+
				"are not allowed",
			name, raw,
		)
	}
	return nil
}

func validateKeyPrefix(prefix string) error {
	if !utf8.ValidString(prefix) {
		return errors.New("airplan: key_prefix must be valid UTF-8")
	}
	trimmed := strings.Trim(prefix, "/")
	if trimmed == "" {
		return nil
	}
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf(
				"airplan: invalid key_prefix %q: empty, dot, and "+
					"dot-dot path segments are not allowed",
				prefix,
			)
		}
	}
	return nil
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

func resolveConfigPath(
	path string,
	getenv func(string) string,
) (ConfigPathResolution, bool, error) {
	explicit := path != ""
	source := ConfigSource{
		Kind: ConfigSourceOverride, Name: "--config",
	}
	if path == "" {
		path = getenv("AIRPLAN_CONFIG")
		explicit = path != ""
		source = ConfigSource{
			Kind: ConfigSourceEnv, Name: "AIRPLAN_CONFIG",
		}
	}
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return ConfigPathResolution{}, false, err
		}
		source = ConfigSource{
			Kind: ConfigSourceBuiltin, Name: "default config path",
		}
	}
	return ConfigPathResolution{Path: path, Source: source}, explicit, nil
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

	// Unknown keys are an error, not silently ignored: a typo'd key
	// (bucet = ...) otherwise surfaces as a baffling missing-field
	// error later. This also keeps the parser exactly in sync with
	// the published schema's additionalProperties: false (SPEC.md §7).
	if unknown := meta.Undecoded(); len(unknown) > 0 {
		names := make([]string, len(unknown))
		for i, k := range unknown {
			names[i] = k.String()
		}
		return fileConfig, toml.MetaData{}, false, fmt.Errorf(
			"airplan: unknown config key(s) in %s: %s",
			path, strings.Join(names, ", "),
		)
	}

	if fileConfig.Profiles == nil {
		fileConfig.Profiles = map[string]Settings{}
	}

	return fileConfig, meta, true, nil
}

func resolveProfile(
	opts ConfigOptions,
	getenv func(string) string,
	fileConfig FileConfig,
	meta toml.MetaData,
) (string, error) {
	names := profileNames(fileConfig.Profiles)

	// An explicitly requested profile must exist even when the config
	// file defines no profiles at all (SPEC.md §7, resolution step 1).
	selected := opts.Profile
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
		if err := validateDefaultProfile(fileConfig); err != nil {
			return "", err
		}
		return fileConfig.DefaultProfile, nil
	}

	if len(names) == 0 {
		return "", nil
	}

	if len(names) == 1 {
		return names[0], nil
	}

	// Completeness counts everything above profiles in the precedence
	// order: root values, env vars, and flag overrides (SPEC.md §7,
	// resolution step 4).
	rootCfg := &Config{Region: "auto"}
	applySettings(rootCfg, fileConfig.Settings, rootKeyDefined(meta, true))
	if err := applyEnv(rootCfg, getenv); err != nil {
		return "", err
	}
	applyOverrides(rootCfg, opts.Overrides)
	if rootCfg.Endpoint != "" && rootCfg.Bucket != "" {
		return "", nil
	}

	return "", fmt.Errorf(
		"airplan: no profile selected and root-level values are "+
			"incomplete; available profiles: %s",
		strings.Join(names, ", "),
	)
}

func validateDefaultProfile(fileConfig FileConfig) error {
	if fileConfig.DefaultProfile == "" {
		return nil
	}
	if _, ok := fileConfig.Profiles[fileConfig.DefaultProfile]; ok {
		return nil
	}
	names := profileNames(fileConfig.Profiles)
	available := "none defined"
	if len(names) > 0 {
		available = strings.Join(names, ", ")
	}
	return fmt.Errorf(
		"airplan: default_profile %q does not exist; available profiles: %s",
		fileConfig.DefaultProfile,
		available,
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
	if settings.NoExternalAssets != nil {
		cfg.NoExternalAssets = *settings.NoExternalAssets
	}
	if defined("mermaid_url") {
		cfg.MermaidURL = settings.MermaidURL
		if cfg.MermaidURL == "" {
			cfg.MermaidURL = DefaultMermaidURL
		}
	}
	if defined("repo") {
		cfg.Repository = settings.Repository
		if cfg.Repository == "" {
			cfg.Repository = "auto"
		}
	}
}

func applyEnv(cfg *Config, getenv func(string) string) error {
	for _, field := range configFieldDefinitions {
		if field.envName == "" || field.name == "timeout" {
			continue
		}
		value := getenv(field.envName)
		if value == "" {
			continue
		}
		if field.name == "no_external_assets" {
			if err := applyEnvBoolValue(
				&cfg.NoExternalAssets, value, field.envName,
			); err != nil {
				return err
			}
			continue
		}
		applyEnvStringValue(cfg, field.name, value)
	}
	return nil
}

func applyEnvBoolValue(field *bool, value, name string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf(
			"airplan: invalid %s %q: must be a boolean",
			name,
			value,
		)
	}
	*field = parsed
	return nil
}

func applyEnvStringValue(cfg *Config, name, value string) {
	switch name {
	case "endpoint":
		cfg.Endpoint = value
	case "bucket":
		cfg.Bucket = value
	case "region":
		cfg.Region = value
	case "access_key_id":
		cfg.AccessKeyID = value
	case "secret_access_key":
		cfg.SecretAccessKey = value
	case "public_base_url":
		cfg.PublicBaseURL = value
	case "key_prefix":
		cfg.KeyPrefix = value
	case "template":
		cfg.Template = value
	case "mermaid_url":
		cfg.MermaidURL = value
	case "repo":
		cfg.Repository = value
	}
}

// applyOverrides overlays top-of-precedence values (CLI flags): empty
// strings and nil bools are "not set" and leave cfg untouched.
func applyOverrides(cfg *Config, s Settings) {
	for field, value := range map[*string]string{
		&cfg.Endpoint:        s.Endpoint,
		&cfg.Bucket:          s.Bucket,
		&cfg.Region:          s.Region,
		&cfg.AccessKeyID:     s.AccessKeyID,
		&cfg.SecretAccessKey: s.SecretAccessKey,
		&cfg.PublicBaseURL:   s.PublicBaseURL,
		&cfg.KeyPrefix:       s.KeyPrefix,
		&cfg.Template:        s.Template,
		&cfg.MermaidURL:      s.MermaidURL,
		&cfg.Repository:      s.Repository,
	} {
		if value != "" {
			*field = value
		}
	}
	if s.NoSource != nil {
		cfg.NoSource = *s.NoSource
	}
	if s.Indexable != nil {
		cfg.Indexable = *s.Indexable
	}
	if s.NoExternalAssets != nil {
		cfg.NoExternalAssets = *s.NoExternalAssets
	}
	// Settings.Timeout is resolved separately (resolveTimeout) so the
	// winning raw value is parsed exactly once.
}

func warnReadableCredentials(
	cfg *Config,
	path string,
	fileConfig FileConfig,
) {
	cfg.Warnings = append(
		cfg.Warnings,
		readableCredentialWarnings(path, fileConfig)...,
	)
}

func readableCredentialWarnings(
	path string,
	fileConfig FileConfig,
) []string {
	if runtime.GOOS == "windows" || !containsCredentials(fileConfig) {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().Perm()&0o077 == 0 {
		return nil
	}

	return []string{
		fmt.Sprintf(
			"config file %s contains credentials and is group- or "+
				"world-readable",
			path,
		),
	}
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
