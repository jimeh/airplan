# Configure Airplan

Airplan accepts configuration from command-line flags, environment variables,
and a TOML file. Use the [README quick start](../README.md#quick-start) for a
single-bucket setup. This guide covers profiles, backends, themes, diagnostics,
and shell completion.

## Precedence

Airplan resolves settings from highest to lowest priority:

1. Command-line flags
2. `AIRPLAN_*` environment variables
3. The selected named profile
4. Root-level config file values
5. Built-in defaults

The default config path is `$XDG_CONFIG_HOME/airplan/config.toml`, normally
`~/.config/airplan/config.toml` on Linux and macOS. Windows uses its normal
per-user config directory. Select another file with `--config PATH` or
`AIRPLAN_CONFIG`.

An explicitly selected file must exist. Only the platform-default file is
optional, which allows a complete environment-only setup.

## Config files and profiles

Root-level values are shared defaults. Named profiles inherit them and override
only what differs:

```toml
# Root-level keys must appear before the first [profiles.*] table.
backend = "s3"
endpoint = "https://<account-id>.r2.cloudflarestorage.com"
region = "auto"
key_prefix = "jimeh"
default_profile = "work"
light_theme = "github-light"
dark_theme = "github-dark"

[profiles.work]
bucket = "work-plans"
public_base_url = "https://plans.work.example.com"
light_theme = "solarized-light"
dark_theme = "tokyo-night"

[profiles.personal]
bucket = "personal-plans"
public_base_url = "https://plans.example.com"

[profiles.shared]
backend = "airplan"
api_url = "https://airplan.example.com"
api_token = "..."
```

Select a profile for one command with `--profile NAME` or `-p NAME`. Set
`AIRPLAN_PROFILE` for a project shell, task runner, or `.envrc`:

```sh
export AIRPLAN_PROFILE="work"
```

Airplan does not load `.env` files. Your shell or environment manager must
export the variable. An explicitly selected profile must exist in the config
file.

Without an explicit selection, Airplan uses `default_profile`, the only named
profile, or complete root-level values, in that order. For a shared bucket, give
each person a distinct `key_prefix`; the prefix also scopes remote list and
purge operations.

## Backend-specific settings

`backend` defaults to `s3`. An `s3` profile needs an endpoint, bucket, region,
public base URL, and credentials from either explicit keys or the AWS credential
chain.

An `airplan` profile needs only `api_url` and `api_token`. It does not load the
AWS credential chain. S3 values inherited from root configuration are inactive
for that profile. API settings are inactive for an `s3` profile.

Hosted rendering uses the server's theme configuration. Client-side
`light_theme`, `dark_theme`, `theme`, `available_themes`, and custom `themes`
definitions are not sent to the server. Airplan still validates custom
definitions when it loads the client config and warns about valid but inactive
theme selectors.

See [Run a single-user Airplan server](server.md) for server setup and trust
boundaries.

## Environment-only configuration

A config file is not required. For direct S3 access:

```sh
export AIRPLAN_ENDPOINT="https://<account-id>.r2.cloudflarestorage.com"
export AIRPLAN_BUCKET="plans"
export AIRPLAN_REGION="auto"
export AIRPLAN_PUBLIC_BASE_URL="https://plans.example.com"
export AIRPLAN_ACCESS_KEY_ID="..."
export AIRPLAN_SECRET_ACCESS_KEY="..."

airplan plan.md
```

For a server-backed client:

```sh
export AIRPLAN_BACKEND="airplan"
export AIRPLAN_API_URL="https://airplan.example.com"
export AIRPLAN_API_TOKEN="..."

airplan plan.md
```

Avoid committing credentials in project environment files. Explicit access and
secret keys must be set as a pair. If neither is set through `AIRPLAN_*`
variables or the config file, Airplan uses the standard AWS credential chain,
including `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and the shared
credentials file.

## Environment variable reference

| Variable                            | Purpose                                       |
| ----------------------------------- | --------------------------------------------- |
| `AIRPLAN_CONFIG`                    | Select an alternate config file               |
| `AIRPLAN_PROFILE`                   | Select a named `[profiles.*]` profile         |
| `AIRPLAN_BACKEND`                   | Select `s3` or `airplan`                      |
| `AIRPLAN_API_URL`                   | Set an Airplan server base URL                |
| `AIRPLAN_API_TOKEN`                 | Set its bearer token                          |
| `AIRPLAN_SERVER_HOST`               | Set the `serve` bind host                     |
| `AIRPLAN_SERVER_PORT`               | Set the decimal `serve` port                  |
| `AIRPLAN_SERVER_ALLOW_NON_LOOPBACK` | Acknowledge non-loopback serving              |
| `AIRPLAN_SERVER_TOKEN`              | Provide the static server token               |
| `AIRPLAN_SERVER_TOKEN_FILE`         | Read the static token from a file             |
| `AIRPLAN_SERVER_ALLOWED_ORIGINS`    | Set comma-separated hosted MCP origins        |
| `AIRPLAN_SERVER_TEMP_DIR`           | Set the upload spool directory                |
| `AIRPLAN_SERVER_LOG_LEVEL`          | Set `serve` logging from `error` to `trace`   |
| `AIRPLAN_MANIFEST`                  | Select a local S3 or service manifest         |
| `AIRPLAN_ENDPOINT`                  | Set the S3-compatible API endpoint            |
| `AIRPLAN_BUCKET`                    | Set the destination bucket                    |
| `AIRPLAN_REGION`                    | Set the S3 signing region                     |
| `AIRPLAN_ACCESS_KEY_ID`             | Set the explicit access key ID                |
| `AIRPLAN_SECRET_ACCESS_KEY`         | Set the matching secret access key            |
| `AIRPLAN_PUBLIC_BASE_URL`           | Set the base URL used for public links        |
| `AIRPLAN_KEY_PREFIX`                | Prefix and scope uploaded object keys         |
| `AIRPLAN_TEMPLATE`                  | Select a custom HTML page template            |
| `AIRPLAN_COLLECTION_TEMPLATE`       | Select a collection overview template         |
| `AIRPLAN_LIGHT_THEME`               | Select the default light-mode slot theme      |
| `AIRPLAN_DARK_THEME`                | Select the default dark-mode slot theme       |
| `AIRPLAN_THEME`                     | Force one theme and omit theme controls       |
| `AIRPLAN_NO_EXTERNAL_ASSETS`        | Disable Airplan-managed external loads        |
| `AIRPLAN_MERMAID_URL`               | Set an alternate HTTPS Mermaid module URL     |
| `AIRPLAN_REPO`                      | Set `auto`, `none`, or a repository URL       |
| `AIRPLAN_TIMEOUT`                   | Set a duration such as `30s`; `0` disables it |

`no_source` and `indexable` do not have environment variables. Configure them
in TOML or override them with `--no-source` and `--indexable`.

## Page themes

The Appearance button keeps System, Light, and Dark mode separate from the
theme assigned to each mode. Both theme selectors show the full catalog in
Light themes and Dark themes groups, so either variant can be assigned to
either slot. Preferences persist per origin. Pages without JavaScript use the
uploader defaults and the reader's system mode. Printing always uses GitHub
Light for the page, syntax highlighting, and Mermaid diagrams.

Limit the selectable catalog and preserve the listed order within each variant
group with a root-level or profile-level list:

```toml
available_themes = ["github-light", "tokyo-night-day", "tokyo-night"]
light_theme = "tokyo-night-day"
dark_theme = "tokyo-night"
```

Airplan adds a configured slot default that is missing from the list and warns
on stderr. An explicit profile list replaces the root list.

To publish a fixed theme without theme controls, use
`theme = "tokyo-night"` or `AIRPLAN_THEME`. The Appearance panel retains the
Fixed navbar switch. A fixed theme takes priority over the catalog and slot
defaults, with a warning when those settings are also explicit.

Built-in themes are GitHub Light and Dark, Catppuccin Latte and Mocha, Rose Pine
Dawn and Main, Solarized Light and Dark, Tokyo Night Day and Night, and One
Dark.

### Define a custom theme

Custom themes are global config entries available to every profile:

```toml
[themes.docs]
name = "Docs"
variant = "dark"
background = "#10131a"
foreground = "#e7eaf0"
muted = "#9aa3b2"
accent = "#7aa2f7"
accent_foreground = "#10131a"
border = "#3b4261"
surface = "#181c27"
surface_emphasis = "#242a3a"
info = "#7dcfff"
success = "#9ece6a"
important = "#bb9af7"
warning = "#e0af68"
danger = "#f7768e"
syntax = "derived" # or chroma:<registered-style-name>
```

Every color token is required and accepts `#rrggbb` or `#rrggbbaa`. Built-in
IDs cannot be replaced. Unknown keys and selected IDs fail before upload unless
an active forced `theme` deliberately ignores that selector.

Omitting `syntax` or setting it to an empty string is equivalent to `derived`.
See [built-in theme sources](../THIRD_PARTY_THEMES.md) for pinned upstream
palettes and licenses.

Custom page templates receive safe theme CSS and catalog fields, conditional
Mermaid palette data, and `.AppearanceEnabled`. A custom template must add its
own theme styling, controls, and runtime behavior.

## Inspect configuration

```sh
airplan config profiles        # list named profiles and the explicit default
airplan config show            # show resolved values and their winning source
airplan config show --json     # return the same diagnostics for scripts
airplan config schema          # print the complete config file JSON Schema
```

`config show` always redacts access keys, secret keys, and API tokens. It
resolves the active configuration without contacting storage or resolving the
standard AWS credential chain. `config profiles --json` provides a scriptable
profile inventory without requiring every profile to be complete.

## Shell completion

Airplan generates completion scripts at runtime for Bash, Zsh, Fish, and
PowerShell:

```sh
# Bash
source <(airplan completion bash)

# Zsh
source <(airplan completion zsh)

# Fish
airplan completion fish | source
```

```powershell
# PowerShell
airplan completion powershell | Out-String | Invoke-Expression
```

These commands enable completion for the current session. Run
`airplan completion <shell> --help` for persistent installation instructions
for that shell and platform.

## Full contract

Run `airplan config schema` to inspect every file setting. [SPEC.md section
7](../SPEC.md#7-configuration) defines the configuration contract, including
validation, inactive-field warnings, and selector precedence.
