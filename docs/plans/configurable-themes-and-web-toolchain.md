# Configurable Themes and TypeScript Web Toolchain Implementation Plan

Status: proposed

Scope: Bun-based web tooling, authored TypeScript, configurable built-in page
themes, theme selection UI, derived syntax highlighting, Mermaid integration,
and fixed print styling

Repository baseline: Airplan v0.9.0, spec 0.38.0

## 1. Goal

Replace the built-in template's fixed light/dark palette with a small,
extensible theme system while preserving Airplan's standalone-page model.

An uploader can select the default themes used by light and dark modes, define
additional themes in Airplan's TOML configuration, and keep using the built-in
document and collection templates. A reader can independently choose:

- whether the page follows the system, light, or dark mode slot;
- which theme is assigned to the light-mode slot; and
- which theme is assigned to the dark-mode slot.

Theme classification is descriptive rather than restrictive. A reader may put
a dark-classified theme in the light-mode slot or a light-classified theme in
the dark-mode slot. The selected theme's classification controls native color
behavior and integrations, while the selected mode controls only which slot is
active.

The work also replaces the repository's small npm/JavaScript island with a
Bun-managed TypeScript toolchain. All maintained browser and browser-test code
becomes TypeScript, static browser assets are bundled and minified by Bun, and
CSS gains explicit formatting, linting, and minification.

Deliver the work as two pull requests:

1. a behavior-preserving web-toolchain and source conversion; then
2. the observable configurable-theme feature built on that foundation.

## 2. Settled decisions

1. Use Bun for JavaScript package management, script execution, and production
   asset bundling. Commit `bun.lock`, remove `package-lock.json`, and configure
   Bun to reject releases newer than seven days for every dependency by
   default.
2. Add Bun to `mise.toml` and `mise.lock`. Retain Node in Mise initially so
   Playwright continues to run under its supported Node runtime even though Bun
   installs its dependencies.
3. Use TypeScript 7 in strict mode for all maintained browser code, Playwright
   configuration, browser tests, and asset-build scripts.
4. Use Oxfmt for TypeScript, JavaScript output fixtures where applicable, CSS,
   JSON, Markdown, and other supported repository text. Use Oxlint plus
   `oxlint-tsgolint` for ordinary and type-aware TypeScript linting.
5. Use Stylelint with `stylelint-config-standard` for CSS correctness. Use Bun's
   CSS bundler and minifier for production output; do not add Prettier or a
   second CSS minifier.
6. Do not add Preact in either pull request. Keep server-rendered markup and a
   small TypeScript controller. Reconsider Preact only if later theme editing,
   previews, search, or other component-heavy UI makes split DOM ownership
   worthwhile.
7. Keep mode and theme selection separate. Mode is `system`, `light`, or
   `dark`; light and dark theme assignments are independent theme IDs.
8. A theme declares `variant = "light"` or `variant = "dark"`. The variant
   determines catalog grouping, CSS `color-scheme`, Mermaid `darkMode`, and
   other palette-sensitive behavior. It never limits which mode slot may use
   the theme.
9. Both theme selectors show the complete catalog, grouped into Light themes
   and Dark themes. Custom themes appear in the group selected by their
   declared variant.
10. The initial built-in catalog contains GitHub Light, GitHub Dark,
    Catppuccin Latte, Catppuccin Mocha, Rose Pine Dawn, Rose Pine, Solarized
    Light, Solarized Dark, Tokyo Night Day, Tokyo Night, and One Dark.
11. The current Airplan light and dark palettes become GitHub Light and GitHub
    Dark because their colors substantially match GitHub Primer rather than an
    original Airplan color system.
12. Built-in themes use curated corresponding Chroma styles. Custom themes use
    a Chroma style derived from their semantic tokens by default and may
    instead select any Chroma style bundled in the pinned Chroma release.
13. Mermaid uses `theme: "base"` plus variables derived from the resolved
    Airplan theme. Theme changes re-render from preserved source and cache the
    result by theme ID.
14. Printing ignores interactive theme assignments and always uses a dedicated,
    non-configurable GitHub Light palette for page CSS, Chroma syntax, and
    Mermaid.
15. Custom templates remain responsible for their own presentation and do not
    receive automatically injected controls or assets. The template data
    contract exposes resolved theme data so an author can opt in, and the
    template printed by `airplan template` continues to work unchanged when
    saved and reused.
16. Theme configuration is part of the rendering recipe and provenance. Theme
    changes must not silently pass upgrade or reconciliation checks as the
    same rendered configuration.

## 3. Non-goals

- A browser-based custom-theme editor.
- Downloading themes, JavaScript, CSS, fonts, or syntax definitions at page
  view time. Mermaid remains the only Airplan-managed conditional external
  load.
- Arbitrary user-authored CSS in theme configuration.
- Per-document theme controls on the upload CLI in the first release.
- Per-token custom syntax-highlight definitions in the first release.
- Replacing Chroma, Goldmark, or server-side code highlighting.
- Making theme definitions portable to arbitrary custom templates
  automatically.
- Synchronizing preferences across origins, browsers, or devices.
- Making a reader's local theme preferences mutate uploaded content or remote
  marker metadata.
- A selectable or configurable print theme in the first release.
- Adding a component framework solely for the theme panel.

## 4. Terminology and state model

Keep these concepts independent in Go, TypeScript, configuration, markup, and
the specification:

| Term            | Meaning                                                            |
| --------------- | ------------------------------------------------------------------ |
| Mode preference | Reader selection: `system`, `light`, or `dark`                     |
| Resolved mode   | `light` or `dark` after applying the system preference             |
| Mode slot       | The light-theme or dark-theme assignment selected by resolved mode |
| Theme ID        | Stable lowercase identifier for one built-in or custom theme       |
| Theme variant   | Theme metadata: `light` or `dark`                                  |
| Resolved theme  | Theme selected from the active mode slot after fallback            |
| Print theme     | Fixed internal GitHub Light palette, independent of both slots     |
| Theme recipe    | Upload-time defaults and exact resolved catalog digest             |

The resolution sequence is:

```text
mode preference
  -> system media query when needed
  -> resolved light/dark mode
  -> reader's stored assignment for that slot when available
  -> uploader-configured assignment for that slot
  -> built-in GitHub Light/GitHub Dark fallback
  -> resolved theme and its declared variant
```

The root element should expose separate state attributes, for example:

```html
<html
  data-airplan-mode="system"
  data-airplan-resolved-mode="light"
  data-airplan-theme="tokyo-night"
  data-airplan-theme-variant="dark"
></html>
```

Do not overload one attribute with both slot selection and palette identity.

## 5. Delivery strategy

### 5.1 Pull request 1: Bun and TypeScript web-toolchain foundation

This pull request changes how browser assets are maintained and produced but
does not intentionally change page behavior or appearance. It lands first and
becomes the baseline for the theme feature.

Its review should be dominated by reproducibility, generated-output
equivalence, package-manager migration, and test coverage rather than theme
design.

### 5.2 Pull request 2: Configurable themes

This pull request introduces the public configuration and page behavior. It
depends on the generated TypeScript/CSS asset pipeline from pull request 1 and
contains the specification, schema, provenance, UI, Mermaid, syntax, and
browser changes as one coherent feature.

Do not partially expose theme configuration in pull request 1. The first pull
request may establish internal TypeScript interfaces needed by the second, but
the shipped page and config contract stay fixed until pull request 2.

## 6. Pull request 1: Bun and TypeScript toolchain

### 6.1 Package-manager migration

Add a major-version Bun selector to Mise and refresh the exact multi-platform
resolution in `mise.lock`.

Migrate the existing npm lockfile using Bun's lockfile importer, verify the
resolved Playwright dependency remains equivalent, commit text `bun.lock`, and
delete `package-lock.json`. Keep exact development dependency versions in
`package.json`, following the repository's current deterministic dependency
style and minimum-release-age policy.

Commit a repository `bunfig.toml` with Bun's install-time release-age gate:

```toml
[install]
minimumReleaseAge = 604800
minimumReleaseAgeExcludes = []
```

The seven-day minimum applies to all direct and transitive packages, including
TypeScript, type declarations, test tooling, linters, formatters, and their
dependencies. Do not weaken it with standing package exclusions. A confirmed
security update may use a narrowly scoped, temporary exception in the update
that needs it, consistent with the repository's existing security-update
policy; remove that exception as soon as the release reaches the normal age.
Document this exceptional procedure rather than silently bypassing the gate.

Replace project-development commands as follows:

| Current                        | Replacement                       |
| ------------------------------ | --------------------------------- |
| `npm ci`                       | `bun ci`                          |
| `npm run <script>`             | `bun run <script>`                |
| `npx <tool>`                   | `bunx <tool>` or a package script |
| `npm audit --audit-level=high` | `bun audit --audit-level=high`    |

The user-facing `npx skills add ...` example is not part of Airplan's
development package management and should remain unchanged unless that
external tool officially changes its recommended invocation.

Update Mise setup, browser tests, dependency audit, CI, README development
instructions, AGENTS task descriptions, and any workflow cache keys that
refer to npm or `package-lock.json`.

Retain Node 24 in Mise for Playwright. Run Playwright's CLI through its installed
package under Node if Bun's bin execution would change the runtime. Bun still
owns dependency resolution and installation.

### 6.2 Development dependencies

Add exact compatible releases of:

- `typescript` 7.x;
- `@types/node` matching the retained Node major;
- `oxlint`;
- `oxlint-tsgolint` matching TypeScript 7;
- `oxfmt`;
- `stylelint`;
- `stylelint-config-standard`; and
- the existing `@playwright/test` dependency.

Prefer package-local Oxc tools so their schemas, editor integration, and CI
versions are coupled to `bun.lock`. Remove the standalone Mise Oxfmt tool after
package scripts own all Oxfmt invocations; do not keep two independently
resolved formatter versions.

Add:

- `tsconfig.json` for browser code with strict TypeScript 7, DOM libraries,
  modern browser target, and bundler resolution;
- a separate Playwright/tooling tsconfig with Node types;
- `.oxlintrc.json` enabling correctness plus material type-aware promise and
  exception rules;
- `.oxfmtrc.json` with generated outputs, lockfiles, artifacts, coverage, and
  dependency directories excluded; and
- `stylelint.config.mjs` extending the standard config with only narrowly
  justified exceptions for the existing CSS architecture.

Keep `tsc --noEmit` as an explicit typecheck even though Oxlint can also report
type diagnostics. Oxlint's type-aware mode provides semantic lint rules; it is
not the sole compiler gate.

### 6.3 Maintained source layout

Move maintained browser sources out of the embedded generated-asset directory
into a clear source tree:

```text
web/
├── build.ts
├── src/
│   ├── theme-init.ts
│   ├── theme.ts
│   ├── page.ts
│   ├── collection.ts
│   └── mermaid.ts
└── styles/
    ├── shared.css
    ├── page.css
    └── collection.css
```

Convert:

- `airplan/assets/theme-init.js`;
- `airplan/assets/theme.js`;
- `airplan/assets/page.js`;
- `airplan/assets/collection.js`;
- `airplan/assets/mermaid.js.tmpl`;
- `playwright.config.js`; and
- every JavaScript file under `tests/browser/`

to TypeScript-maintained equivalents.

Browser-delivered output remains JavaScript. “All JavaScript becomes
TypeScript” means no project-authored `.js`, `.mjs`, or `.cjs` source remains;
committed generated `.js` assets are explicitly allowed and identified as
generated.

Preserve the early, synchronous theme bootstrap as its own tiny bundle loaded
before page paint. Do not merge it into a deferred page bundle and reintroduce
a light-theme flash.

### 6.4 Deterministic asset generation

Have `web/build.ts` invoke Bun's build API for explicit browser entry points.
Generate two deterministic representations under `airplan/assets/generated/`:

```text
airplan/assets/generated/
├── readable/
│   ├── theme-init.js
│   ├── page.js
│   ├── collection.js
│   ├── mermaid.js.tmpl
│   ├── page.css
│   └── collection.css
└── minified/
    └── ...matching production assets...
```

The exact split may retain a separate shared runtime when that produces less
duplication without complicating inline embedding. Measure final embedded page
bytes before choosing shared versus per-page bundles; do not add runtime
network requests for bundled code.

Use readable generated assets for `airplan template` output and minified assets
for executable built-in pages. This keeps the custom-template starting point
reviewable while reducing uploaded page size.

The Mermaid module URL currently appears inside a Go-template JavaScript
expression. Keep the authored TypeScript valid by compiling a unique inert
sentinel and replacing exactly that sentinel with the existing escaped Go
template expression during generation. The generator must fail unless the
sentinel appears exactly once. Do not make the module URL an unescaped string
concatenation or weaken the existing HTTPS validation.

Keep that sentinel out of a literal `import("...")` expression, which Bun would
try to resolve. Store it in a variable and use a non-literal dynamic import, or
mark it external explicitly. Build the Mermaid entry as ESM because its inline
`type="module"` script uses top-level await. Build `theme-init`, page, and
collection as classic-script IIFEs with no exported global API. Pin the format
and browser target for every entry rather than relying on Bun defaults.

Add `mise run generate:web` and `mise run generate:web:check`. The check builds
into a temporary directory and byte-compares every expected output, rejects
missing or extra generated files, and leaves the worktree unchanged.

Wire the web generator into `mise run generate`, `mise run generate:check`, and
the normal `mise run check` dependency graph. Keep `go generate ./...` focused
on Go-owned generators if that boundary remains clearer; Mise is the canonical
cross-language orchestration surface.

Generated assets must carry a concise generated-file header in readable mode.
Minified output may omit comments except required license notices.

Because baking happens before Go parses the result as `html/template`, reject
generated JavaScript or CSS containing `{{` or `}}`; either sequence could be
interpreted as a template action. Add a round-trip test that parses the baked
template and proves each minified bundle is emitted byte-for-byte, catching both
template delimiters and `html/template` JavaScript-context corruption.

### 6.5 CSS tooling

Oxfmt owns source CSS formatting. Stylelint owns CSS correctness and convention
checks. Bun owns production parsing, browser-target lowering, bundling, and
minification.

Initially extend `stylelint-config-standard` and disable only rules proven
incompatible with intentional existing patterns such as custom property names,
specificity layering, or required vendor properties. Do not copy a broad
exception list without reproducing each need in Airplan's styles.

Enable cross-file unknown-custom-property checking if Stylelint's
`referenceFiles` can model the shared/page/collection token definitions without
false positives. Otherwise defer that one rule and cover the canonical token
surface through the theme generator tests in pull request 2.

Lint formatted source, never generated minified output. Parse the generated CSS
with Bun during every build so invalid source still fails generation.

### 6.6 Tasks, hooks, CI, and documentation

Add discoverable Mise tasks:

- `typecheck:web`;
- `lint:web` and focused `lint:ts` / `lint:css` tasks;
- `format:web` and `format:web:check` as appropriate;
- `generate:web` and `generate:web:check`; and
- a Bun-backed `test:web` for future pure TypeScript tests.

Fold them into the existing `lint`, `format`, `check`, `generate`, and
`audit:deps` surfaces without weakening Go or workflow checks.

Make `mise run setup` install locked JavaScript dependencies with `bun ci`
before installing hooks. Route package-local Oxfmt, Oxlint, Stylelint,
TypeScript, and Playwright through `bun run` or `bunx` in tasks and hooks in the
same commit that removes Mise's standalone Oxfmt. A fresh clone or treeboot
setup must therefore leave Markdown hooks working without a separately
installed formatter.

Update Lefthook so staged TypeScript and CSS receive fast formatting and
non-type-aware lint checks, while dependency/config/source changes trigger the
complete TypeScript typecheck and type-aware lint. Hooks remain read-only and
must not restage files.

Give CI explicit web-tooling feedback rather than hiding every failure inside
one broad job. Reuse Mise's Bun installation instead of adding an independently
pinned Bun setup action unless CI evidence shows Mise cannot provide the
required cache or Windows behavior.

Keep the native Windows matrix Go-only and disable Bun installation for that
job; Windows still receives Go unit-test coverage without paying for unused web
tooling. Web format, lint, generation, and browser jobs run on Linux with Bun.
Retain Bun's multi-platform Mise lock entries for supported local development.

Update `IMPLEMENTATION.md` to describe Bun as build-time tooling rather than a
runtime dependency. The released Airplan executable remains a static Go binary
with browser assets embedded through `go:embed`.

Because embedded assets and dumped template bytes are contract-sensitive even
when behavior is preserved, update the source-friendly template description
and advance the spec minor version under the pre-1.0 policy for this observable
template-output contract change. Pull request 2 then advances the next minor
version for the observable theme feature.

### 6.7 Pull request 1 verification

During conversion, compare representative rendered output before and after:

- page structure and accessible names;
- light, system, and dark state behavior;
- source/rendered/changes views;
- copy controls and revision discovery;
- collection controls;
- Mermaid rendering, switching, viewer, and print behavior; and
- no-JavaScript output.

Required automated evidence:

1. New TypeScript typechecks run and name both browser and Playwright projects.
2. Oxlint ordinary and type-aware passes run against maintained TypeScript.
3. Oxfmt and Stylelint pass against their intended source sets.
4. `generate:web:check` detects a representative stale generated asset before
   passing with restored output.
5. Go template marker, round-trip, golden, and rendering sentinels are updated
   for generated formatting and pass against generated assets.
6. Existing Playwright smoke tests run from `.ts` files and cover all current
   projects.
7. `mise run check` passes.
8. `mise run test:browser` passes.
9. `mise run audit:deps` uses Bun and passes.
10. A focused configuration test parses `bunfig.toml` and requires exactly a
    seven-day minimum with an empty exclusion list. A scratch install without a
    lockfile proves Bun accepts and applies the repository configuration during
    fresh resolution; `bun ci` separately proves the committed lockfile is
    reproducible. Do not claim that a locked install age-checks versions already
    present in `bun.lock`.
11. `mise run verify` passes on the intended final head, subject to documented
    Docker availability.

Capture readable and minified byte counts for each entry point in the pull
request description. Treat them as evidence, not a hard size budget until the
new baseline is established.

### 6.8 Pull request 1 acceptance criteria

- Bun is the only project JavaScript package manager.
- `bun.lock` is committed and `package-lock.json` is absent.
- Bun enforces a seven-day minimum release age for every dependency by default,
  with no standing exclusions.
- Every maintained JavaScript source has become TypeScript.
- Node remains only for supported tooling runtime needs, not package
  management.
- Production browser JavaScript and CSS are Bun-bundled and minified.
- `airplan template` remains readable and reusable.
- Generated output is deterministic and checked locally and in CI.
- Existing visible page behavior has no intentional regression.
- The repository's normal handoff and broad verification gates pass.

## 7. Pull request 2: Configurable themes

### 7.1 Theme domain model

Add an internal/publicly consumable resolved theme model with:

```go
type ThemeVariant string

const (
	ThemeVariantLight ThemeVariant = "light"
	ThemeVariantDark  ThemeVariant = "dark"
)

type Theme struct {
	ID      string
	Name    string
	Variant ThemeVariant
	Tokens  ThemeTokens
	Syntax  string
}
```

Use a deliberately small semantic token surface. Require these custom-theme
tokens:

| Token               | Purpose                                       |
| ------------------- | --------------------------------------------- |
| `background`        | Page and modal background                     |
| `foreground`        | Primary text and headings                     |
| `muted`             | Secondary text and subdued controls           |
| `accent`            | Links, focus rings, and active accents        |
| `accent_foreground` | Text/icons placed on an accent fill           |
| `border`            | Ordinary dividers and boundaries              |
| `surface`           | Code blocks, cards, tracks, and subtle panels |
| `surface_emphasis`  | Raised/active controls and stronger panels    |
| `info`              | Note and informational foreground             |
| `success`           | Tip, inserted, and success foreground         |
| `important`         | Important-callout foreground                  |
| `warning`           | Warning foreground                            |
| `danger`            | Caution, deleted, and failure foreground      |

Derive inline-code background, code border, control hover/active states, mark
background, alert backgrounds, dialog backdrop, and similar secondary values
with deterministic color mixing against `background`, `surface`, and
`foreground`. This avoids requiring dozens of near-duplicate configuration
values while retaining enough semantic colors for syntax and Mermaid.

Restrict configured colors to canonical hexadecimal forms accepted by the
shared Go and TypeScript color parser. Normalize to lowercase six- or
eight-digit output during resolution. Composite alpha colors against the theme
background before passing them to Chroma or Mermaid. This prevents CSS or
template injection and makes recipe hashing deterministic.

Validate built-in foreground/background, muted/background, accent/background,
and accent-fill contrast with focused tests. Custom themes receive syntax and
structural validation, but low contrast is not a hard configuration error in
the first release; custom presentation remains the author's responsibility.

### 7.2 Built-in catalog

Reserve these IDs and classifications:

| ID                 | Display name     | Variant | Chroma syntax      |
| ------------------ | ---------------- | ------- | ------------------ |
| `github-light`     | GitHub Light     | light   | `github`           |
| `catppuccin-latte` | Catppuccin Latte | light   | `catppuccin-latte` |
| `rose-pine-dawn`   | Rose Pine Dawn   | light   | `rose-pine-dawn`   |
| `solarized-light`  | Solarized Light  | light   | `solarized-light`  |
| `tokyo-night-day`  | Tokyo Night Day  | light   | `tokyonight-day`   |
| `github-dark`      | GitHub Dark      | dark    | `github-dark`      |
| `catppuccin-mocha` | Catppuccin Mocha | dark    | `catppuccin-mocha` |
| `rose-pine`        | Rose Pine        | dark    | `rose-pine`        |
| `solarized-dark`   | Solarized Dark   | dark    | `solarized-dark`   |
| `tokyo-night`      | Tokyo Night      | dark    | `tokyonight-night` |
| `one-dark`         | One Dark         | dark    | `onedark`          |

Source the semantic page tokens from each project's official open-source
palette rather than sampling screenshots or third-party ports. Add a concise
third-party theme attribution document containing upstream project, exact
source revision or release, license, and the mapping method used. Keep palette
updates explicit and reviewable rather than following upstream automatically.

Built-in IDs cannot be overridden by custom theme definitions. Unknown built-in
or custom IDs fail upload-time config validation when selected as a default.

### 7.3 Configuration contract

Add profile-aware default selections:

```toml
light_theme = "github-light"
dark_theme = "github-dark"

[profiles.work]
light_theme = "solarized-light"
dark_theme = "tokyo-night"
```

Add optional environment overrides:

```text
AIRPLAN_LIGHT_THEME
AIRPLAN_DARK_THEME
```

Do not add upload CLI flags initially. Config, profile, and environment
selection cover durable defaults without expanding the already broad upload
flag surface.

Define custom themes globally in the config file:

```toml
[themes.dracula-docs]
name = "Dracula Docs"
variant = "dark"
background = "#282a36"
foreground = "#f8f8f2"
muted = "#a8a8b2"
accent = "#bd93f9"
accent_foreground = "#282a36"
border = "#44475a"
surface = "#30323e"
surface_emphasis = "#44475a"
info = "#8be9fd"
success = "#50fa7b"
important = "#bd93f9"
warning = "#f1fa8c"
danger = "#ff5555"
syntax = "derived"
```

Theme definitions are global catalog entries rather than profile-merged maps.
Profiles select from the resolved global catalog. This avoids ambiguous deep
merge and deletion semantics while allowing every profile to choose different
defaults.

Theme IDs must be lowercase ASCII slugs with a conservative length limit.
Names must be non-empty valid UTF-8 with a bounded length. Reject reserved IDs,
unknown keys, invalid variants, invalid colors, incomplete token sets,
malformed syntax selectors, and defaults that do not exist.

Supported `syntax` values are:

- omitted or `derived`; and
- `chroma:<registered-style-name>`.

Validate Chroma names against `styles.Names()` rather than accepting the
fallback behavior of `styles.Get`, which would silently turn typos into another
palette.

Update config structs, layering, provenance reporting, config inspection,
generated JSON Schema, README, `SPEC.md`, and `IMPLEMENTATION.md` together.

### 7.4 Resolved theme bundle and template data

Resolve one immutable `ThemeBundle` before rendering:

```text
catalog
default light theme ID
default dark theme ID
canonical catalog SHA-256
generated screen CSS
generated fixed print CSS
browser-safe catalog metadata
```

Sort custom IDs and every serialized map key before hashing or output. The same
effective configuration must produce the same bytes across processes and
platforms.

Expose safe resolved fields to document and collection template data, likely:

- `ThemeCSS` as `template.CSS`;
- `ThemeCatalogJSON` as safely encoded `template.JS` or JSON in an inert script
  element;
- `DefaultLightTheme`;
- `DefaultDarkTheme`; and
- fixed print-theme metadata where a custom template explicitly wants it.

Do not expose raw unvalidated TOML strings as trusted CSS or JavaScript.

The built-in templates consume this data. Custom templates receive it but gain
no injected UI, runtime, CSS, or behavior. Existing custom templates that do
not reference the new fields continue working.

The template printed by `airplan template document` or `collection` uses the
new fields and remains reusable unchanged. Extend template round-trip and bake
marker tests for every new dynamic insertion point.

### 7.5 CSS application and no-JavaScript fallback

Generate one CSS custom-property block per catalog theme, selected by exact
root theme ID. Keep layout and component CSS semantic: it references only the
canonical variables and never contains theme-specific selectors outside the
generated theme block.

For JavaScript-disabled readers, the root CSS uses the uploader-configured
light/dark defaults through `light-dark()` or equivalent media-scoped blocks.
The page follows `prefers-color-scheme` and does not show the JavaScript-only
theme panel.

For JavaScript-enabled readers, the early bootstrap resolves persisted state
before first paint and sets the explicit theme attributes. Unknown persisted
theme IDs fall back for that page without deleting the stored preference; the
same ID may be valid on another Airplan page sharing the origin.

The actual theme variant sets `color-scheme`. Therefore a dark theme assigned
to the light-mode slot receives dark native controls and scrollbars while the
mode still remains light for slot-selection purposes.

### 7.6 Reader preference persistence and migration

Use separate origin-scoped storage keys:

```text
airplan-color-mode
airplan-light-theme
airplan-dark-theme
```

Valid mode values are `system`, `light`, and `dark`. Theme values are IDs and
are validated against the page's embedded catalog before use.

When `airplan-color-mode` is absent, seed it from the current `airplan-theme`
key at bootstrap:

- `light` becomes `airplan-color-mode=light`;
- `dark` becomes `airplan-color-mode=dark`; and
- an absent old key remains the system default.

Do not delete the legacy key merely because migration succeeded: already
published pages remain immutable and continue to depend on it. Whenever a new
page writes the mode, mirror compatibility state as well: `light` or `dark`
writes the same value to `airplan-theme`, while `system` removes both stored mode
keys. New keys are authoritative when present, so a later visit to an old page
cannot clobber the reader's new theme-slot choices. Storage failures remain
non-fatal and retain in-memory behavior for the current page.

Persisted preferences remain shared by document and collection pages on the
same origin. Upload-time defaults apply only when a reader has no valid stored
assignment for that slot.

Dispatch one typed theme-change event containing at least:

```ts
interface AirplanThemeChangeDetail {
  mode: "system" | "light" | "dark";
  resolvedMode: "light" | "dark";
  theme: string;
  variant: "light" | "dark";
}
```

Mermaid and any future integration listen to this resolved event rather than
reimplementing preference logic.

### 7.7 Theme settings panel

Replace the current three-button theme toggle with one icon-only appearance
button. Activating it opens a compact anchored panel containing:

1. a System / Light / Dark mode control; then
2. a Light mode theme select; and
3. a Dark mode theme select.

Each select contains the same options in two `optgroup` sections: Light themes
and Dark themes. Do not filter either select by slot.

Use native buttons and selects. The panel must provide:

- an accessible name for the trigger and panel;
- clear labels for all three settings;
- correct selected/pressed state;
- keyboard navigation using native control behavior;
- Escape and outside-click dismissal;
- focus restoration to the trigger;
- usable positioning at desktop and narrow widths;
- no clipped content at viewport edges;
- reduced-motion compliance; and
- immediate visual updates without page reload.

Prefer the smallest accessible popover implementation supported by the page's
browser floor. If using the Popover API, provide a functional non-modal
fallback rather than making appearance controls disappear in an otherwise
supported browser.

Changing the inactive slot updates storage and its select immediately. It need
not change the visible page until that slot becomes active, but it should
proactively ask Mermaid to prepare that selection.

### 7.8 Fixed GitHub Light printing

Printing is a separate rendering target, not another mode slot.

Under `@media print`, force:

- the GitHub Light semantic token set;
- `color-scheme: light`;
- a white page background and dark foreground;
- GitHub Light code and alert surfaces;
- the Chroma `github` syntax stylesheet; and
- the GitHub Light-derived Mermaid SVG.

Do not consult the light-mode assignment, dark-mode assignment, active theme,
or custom theme catalog for print colors. A Solarized, dark, or custom screen
theme therefore cannot color the physical page background.

Prepare and cache the fixed Mermaid print result before printing can occur.
The synchronous `beforeprint` path swaps to that existing result; it must not
start an asynchronous Mermaid render that the print dialog can overtake.
`afterprint` restores the current resolved screen theme and an open Mermaid
viewer, preserving its transform as today.

Continue expanding and restoring authored disclosures through the existing
print lifecycle.

### 7.9 Syntax highlighting

Generate class-based Chroma CSS per catalog theme. Scope each stylesheet to its
exact theme ID so token classes present in one style cannot leak into another.
Do not emit one global default stylesheet plus partial overrides.

For built-ins, use the curated style names from the catalog table.

For custom `derived` syntax, construct a Chroma style with
`chroma.NewStyleBuilder`. Map at least:

- background and ordinary text;
- muted comments and line numbers;
- accent keywords, functions, tags, and decorators;
- success strings and inserted text;
- warning numbers, constants, and types;
- important declarations and special names;
- danger errors and deleted text; and
- surface emphasis for highlighted lines.

Choose mappings for legibility and semantic consistency rather than pretending
to reproduce a sophisticated editor theme from thirteen inputs. Derived syntax
is a coherent default, not an exact upstream palette.

For `chroma:<name>`, render the selected registered style unchanged except for
selector scoping. A theme may intentionally combine a light page with a dark
syntax style or vice versa; do not enforce variant matching.

Always emit the fixed print `github` syntax stylesheet independently from the
screen catalog.

### 7.10 Mermaid theme integration

Replace Mermaid's current `default`/`dark` initialization with `theme: "base"`
and explicit theme variables derived from the resolved Airplan theme.

At minimum set:

- `background`;
- `darkMode` from the theme variant;
- primary, secondary, and tertiary fills;
- their text and border colors;
- general text and line colors;
- edge-label background; and
- the page font stack.

Derive node fills by mixing background or surface toward accent, foreground,
or semantic colors with documented fixed percentages. Use the theme's explicit
border when it gives better contrast. Normalize alpha colors before Mermaid
receives them.

Preserve every authored diagram source independently from its rendered DOM.
Key cached render results by resolved theme ID, with a reserved key for the
fixed print palette. The catalog is immutable within one uploaded page, so an
ID is sufficient after page load.

At startup, render only the themes currently assigned to the two mode slots
plus the fixed print theme, deduplicating identical IDs. Do not render every
catalog theme. When a reader changes either assignment, render that theme in a
serialized queue and cache it. Ignore or stage stale asynchronous results so a
fast sequence of selections cannot replace a newer theme with an older render.

On failure:

- keep readable authored source when no theme rendered successfully;
- retain the last valid SVG when a newly selected variant fails;
- mark failure per theme rather than poisoning every variant; and
- warn to the console without breaking page, source, theme panel, viewer, or
  print behavior.

Keep Mermaid's strict security level, secure configuration keys, unique render
IDs, viewer ID rewriting, bindings, focus behavior, and conditional external
load contract.

### 7.11 Rendering recipes, markers, and upgrades

Extend the document and collection render recipe with:

```json
{
  "themes": {
    "default_light": "github-light",
    "default_dark": "github-dark",
    "catalog_sha256": "<lowercase sha256>"
  }
}
```

The digest covers the canonical resolved catalog actually embedded in the page,
including built-in definitions, custom definitions, variants, normalized
tokens, display names, and syntax choices. The fixed print palette is part of
the renderer generation and need not be repeated as user configuration.

Do not store local config paths. Full custom theme definitions need not be
duplicated in the marker, just as custom template source is not stored there;
the hash records identity and the current configuration supplies source for a
future forced upgrade.

Thread theme identity through:

- fresh document and collection markers;
- linked-document render recipes;
- update and upgrade eligibility;
- marker validation;
- provenance/config inspection where rendering settings are reported;
- sync and remote reconciliation; and
- tests and golden marker fixtures.

An exact theme-recipe match is required where the existing operation requires
render-recipe equality. A changed custom definition or selected default is a
real rendering change and follows the existing explicit-force rules rather
than silently rewriting an existing page.

Pull request 1 intentionally leaves `RendererGeneration` at 2 and the marker
schema at v4 because its output is behavior-preserving. Pull request 2 increments
`RendererGeneration` to 3 so source-backed pages are eligible to be re-rendered
with the new built-in page capability.

Pull request 2 also advances the marker schema from v4 to v5. Although JSON can
physically represent an optional `themes` recipe field, an older v4 writer can
decode a new recipe into its smaller known struct and later rewrite it without
theme provenance. A v5 marker makes those older readers reject the marker as
unsupported and fail closed instead of stripping data.

Preserve full v4 read and upgrade compatibility while adding v5. Audit every
comparison with `MarkerVersion` (`==`, `!=`, `<`, `<=`, `>`, and `>=`), every
hardcoded marker-version literal, and all user-facing version text. Distinguish
fields introduced by v4 from fields introduced by v5 rather than treating
"older than current" as synonymous with v3.

The audit must explicitly cover these compatibility traps:

- encoding and decoding v4 producer, render recipe, revision, object digest,
  and role fields without stripping them;
- allowing the valid v4 revision/diff combination instead of applying the
  pre-v4 prohibition after the constant becomes 5;
- keeping historical v4 `upgrade` and `link` manifest events valid during
  manifest reads, while requiring newly appended events to use the current
  version where appropriate;
- basing upgrade template-mismatch refusal and stored-recipe carry-over on the
  presence of a v4-or-newer render recipe, not equality with the latest marker
  version;
- advertising all supported HTTP marker versions, including 4, preferably by
  deriving the capability list from the supported-version predicate;
- enriching totals for v5 records and continuing revision-projection
  reinspection for both v4 and v5 records; and
- updating the complete-v5 prerequisite-upgrade error text without weakening
  its fail-closed behavior.

Update marker encoding, validation, declared totals, local manifest reduction,
sync/reconciliation, HTTP capabilities, fixtures, and SPEC.md together. Test
both directions explicitly: current code decodes and safely upgrades a valid v4
revision marker containing a diff; a pre-bump manifest fixture with v4 upload,
upgrade, and link records remains visible and projects revisions correctly; and
a v4-compatible decoder rejects v5 before it can rewrite it.

### 7.12 Specification, documentation, and attribution

Advance `SPEC.md` from the pull request 1 baseline with a minor version because
the feature adds observable configuration and page behavior.

Specify:

- theme config keys and custom-theme schema;
- validation and precedence;
- built-in IDs and default selections;
- mode/slot/variant semantics;
- persistence keys and legacy migration;
- no-JavaScript fallback;
- panel behavior and grouping;
- fixed GitHub Light printing;
- syntax derivation and explicit Chroma selection;
- Mermaid rendering and failure behavior;
- custom-template responsibility and new data fields; and
- render-recipe provenance.

Update README configuration examples, feature overview, browser behavior,
development commands, and template guidance. Update `IMPLEMENTATION.md` with
the Go/TypeScript boundary, generation pipeline, theme resolution, Chroma
builder, and Mermaid cache design.

Add a theme attribution document or README section naming upstream source and
license for Primer/GitHub, Catppuccin, Rose Pine, Solarized, Tokyo Night, and
One Dark. The generated page does not need an intrusive attribution panel; the
repository and distributed source retain the required notices.

### 7.13 Pull request 2 automated testing

#### Go configuration and domain tests

Cover:

- built-in defaults with no config file;
- root and profile default-theme selection;
- environment precedence;
- global custom catalog visibility across profiles;
- arbitrary light/dark variant assignment to either slot;
- reserved and malformed IDs;
- missing tokens, unknown keys, invalid variants, invalid colors, and invalid
  syntax selectors;
- unknown selected theme IDs;
- exact Chroma-name validation without fallback;
- deterministic normalization, ordering, CSS, JSON, and catalog digest;
- built-in contrast invariants;
- derived Chroma style token mappings;
- fixed print syntax independent of configured defaults;
- generated schema refresh and golden comparison; and
- config inspection/provenance without leaking unrelated values.

#### TypeScript unit tests

Use Bun's test runner for pure state logic without adding a browser-DOM
simulator. Cover:

- system/light/dark resolution;
- slot selection independent of theme variant;
- stored value validation and page-local fallback;
- legacy `airplan-theme` seeding only when the new mode key is absent;
- compatibility mirroring for explicit modes and removal for system mode;
- a legacy-key change never overwriting already-established new state;
- unavailable stored custom IDs remaining stored but inactive;
- storage exception handling;
- typed theme-change event detail; and
- stale Mermaid render request rejection where the logic can be isolated.

#### Go rendering and marker tests

Cover:

- generated theme and print CSS scoping;
- syntax styles scoped for every built-in and custom theme;
- template data escaping and safe JSON/CSS insertion;
- document and collection template bake markers;
- dumped-template round trips;
- custom templates remaining uninjected;
- marker recipe theme defaults and digest;
- update/upgrade recipe mismatch behavior;
- `RendererGeneration` 2-to-3 upgrade eligibility;
- v4 marker decode, validation, stored-recipe carry-over, custom-template
  refusal, and upgrade compatibility, including a revision marker with a diff;
- v5 theme-recipe validation and fail-closed rejection by a v4-compatible
  decoder;
- historical v4 manifest upload/upgrade/link visibility and revision projection
  after the current marker version becomes 5;
- HTTP capabilities advertising marker versions 1 through 5; and
- refreshed document, text, collection, and upload-mode goldens.

When adding a material regression test, first observe failure at its intended
assertion against the unimplemented or deliberately perturbed behavior, then
restore and confirm it passes.

#### Browser tests

Extend the real Chromium fixture with a theme gallery containing all built-ins,
a custom theme, representative Chroma tokens, all alert types, controls,
tables, inline code, disclosures, and multiple Mermaid diagram types.

Cover:

- system, explicit light, and explicit dark mode;
- media-query changes while system mode is active;
- both selectors containing identical light/dark optgroups;
- selecting a dark theme for the light slot and a light theme for the dark
  slot;
- actual variant controlling computed `color-scheme`;
- persistence across document and collection reloads;
- unknown stored custom IDs falling back without breaking the page;
- legacy preference seeding and compatibility mirroring with an already
  published page fixture;
- panel keyboard behavior, dismissal, focus restoration, labels, and narrow
  positioning;
- no-JavaScript uploader-default rendering;
- custom-theme computed tokens;
- built-in and derived syntax colors;
- Mermaid startup variants, live re-rendering, cache reuse, failure isolation,
  viewer synchronization, and rapid-selection races;
- print media always computing GitHub Light CSS and GitHub syntax regardless of
  screen selection;
- print Mermaid using the prepared GitHub Light SVG;
- disclosures remaining expanded in print and restored afterward; and
- reduced-motion behavior.

Keep selectors behavioral and accessible. Screenshots and traces remain failure
evidence rather than committed pixel goldens.

### 7.14 Pull request 2 visual verification

Inspect the generated gallery in a real browser at:

- desktop and narrow widths;
- every built-in theme at least once;
- representative cross-variant slot assignments;
- the custom-theme fixture;
- ordinary and high-density prose/code content;
- Mermaid flowchart, sequence, and state/class-style diagrams;
- the open Mermaid viewer before and after theme changes; and
- print emulation from Solarized Light, One Dark, and the custom theme.

Record exact fixture URLs or screenshots in the pull request when the delivery
workflow calls for linkable visual evidence. Do not treat computed-style
assertions alone as proof that the complete page is visually coherent.

### 7.15 Pull request 2 validation gates

Run focused checks while implementing, then:

1. `mise run check`;
2. `mise run test:browser`;
3. `mise run audit:deps`;
4. `mise run test-integration` because marker recipes change;
5. `mise run generate:check` after every schema, asset, or Mermaid-related
   generation change;
6. `mise run release:snapshot` to verify embedded generated assets package
   correctly; and
7. `mise run verify` on the intended final head where Docker is available.

The browser test must confirm by project and test count that the new theme cases
actually ran. Existing green light/dark projects are regression evidence, not
proof of configurable-theme coverage.

### 7.16 Pull request 2 acceptance criteria

- Uploaders can select valid default light and dark theme IDs through root,
  profile, or environment configuration.
- Uploaders can define validated custom themes in TOML.
- Readers can choose mode and independently assign either variant to either
  mode slot.
- Both selectors contain the same catalog grouped by declared variant.
- GitHub Light/Dark replace the misleading Airplan theme names.
- The complete eleven-theme built-in catalog is available.
- Theme preferences persist, seed from the current toggle, and keep permanent
  legacy pages compatible without letting them overwrite established new state.
- No-JavaScript pages retain correct uploader-default system behavior.
- Syntax highlighting follows every screen theme; custom themes derive a
  coherent style or select an explicit Chroma style.
- Mermaid fully derives from, re-renders for, and caches the selected theme.
- Print always uses GitHub Light page, syntax, and Mermaid colors.
- Collection and document pages share the same preference contract.
- Custom templates remain isolated but receive safe opt-in theme data.
- Theme identity participates in marker/render-recipe validation.
- Renderer generation 3 makes prior source-backed pages upgradeable, while
  marker v5 prevents older writers from stripping theme provenance and current
  code retains v4 upgrade compatibility.
- SPEC, implementation notes, schema, README, attributions, goldens, and
  generated assets agree.
- Focused tests, browser tests, integration tests, and repository gates pass.

## 8. Risk management

### 8.1 First-paint flash

Risk: moving theme logic into a bundle delays selection and briefly displays
the wrong palette.

Mitigation: retain a separately generated synchronous head bootstrap containing
only validated storage reads, system resolution, and root-attribute assignment.

### 8.2 CSS size growth

Risk: eleven Chroma styles and semantic token blocks significantly increase
every uploaded HTML page.

Mitigation: Bun-minify static CSS, scope compact generated variables, measure
the exact before/after standalone page size, and avoid embedding unused
third-party source. If Chroma dominates, generate normalized token rules and
deduplicate identical declarations before considering lazy external assets.

### 8.3 Mermaid startup cost

Risk: rendering every catalog theme multiplies Mermaid work.

Mitigation: render only two assigned slot themes plus fixed print, deduplicate
IDs, and lazily cache later selections.

### 8.4 Asynchronous theme races

Risk: a slow Mermaid render from an older selection overwrites a newer one.

Mitigation: serialize render work, stage results off-DOM, track request
revisions, and commit only the currently selected request.

### 8.5 Config-driven injection

Risk: custom names or colors break out of CSS, JSON, or template contexts.

Mitigation: conservative IDs, bounded escaped names, canonical hex-only colors,
typed trusted template fields created only after validation, and adversarial
tests for closing tags, quotes, braces, and control characters.

### 8.6 Provenance ambiguity

Risk: changing a custom theme under the same ID appears identical to existing
render recipes.

Mitigation: hash the complete canonical resolved catalog, not only selected
IDs.

### 8.7 Package-manager/runtime conflation

Risk: replacing npm accidentally forces Playwright or another Node-only tool to
execute under Bun.

Mitigation: distinguish package management from runtime. Bun owns installs and
scripts; retain explicit Node execution where upstream support or observed
behavior requires it.

## 9. Alternatives rejected

- **Keep npm and use Bun only as a bundler:** leaves two overlapping JavaScript
  toolchains and lock responsibilities for no benefit.
- **Use Preact for the initial panel:** introduces a framework boundary without
  enough component complexity to repay it.
- **Render every Mermaid theme at startup:** work scales with catalog size
  instead of the reader's two assignments.
- **Use Mermaid's built-in `default` and `dark` themes:** cannot accurately
  follow community or custom Airplan palettes.
- **Require light themes in the light slot:** conflicts with the explicit reader
  customization model and makes variant metadata a hidden restriction.
- **Print the light slot:** lets Solarized, custom, or even dark themes consume
  ink and reduce print readability.
- **Expose raw CSS in custom theme configuration:** creates an unnecessary
  injection surface and makes semantic integrations impossible to derive.
- **Require a complete custom Chroma token map:** makes simple page themes
  needlessly difficult; derived syntax is a better initial default.
- **Store only selected theme IDs in marker provenance:** misses changes to a
  custom definition under the same ID.

## 10. Implementation order

### Pull request 1

1. Add and lock Bun; add the all-dependency seven-day release-age gate; migrate
   npm lock and commands.
2. Add TypeScript 7, Oxc, and Stylelint configuration and tasks.
3. Establish authored web source and generated-asset directories.
4. Convert browser assets to TypeScript one behavior cluster at a time.
5. Convert Playwright configuration, helpers, and tests to TypeScript.
6. Add readable/minified deterministic Bun generation.
7. Integrate formatting, linting, typechecking, hooks, CI, and audits.
8. Refresh generated assets and rendering goldens.
9. Update implementation/development documentation and spec clarification.
10. Complete focused, browser, check, audit, and broad verification.

### Pull request 2

1. Add theme domain types, built-in catalog, color normalization, and tests.
2. Add config fields, custom-theme schema, resolution, and generated JSON
   Schema.
3. Add deterministic theme bundles, CSS, Chroma generation, and recipe digest.
4. Extend template data; advance marker v4 to v5 and renderer generation 2 to 3;
   preserve v4 upgrade handling; refresh marker and rendering goldens.
5. Implement early state resolution, persistence migration, and typed events.
6. Replace the toggle with the accessible settings panel.
7. Integrate derived and explicit Chroma syntax styles.
8. Refactor Mermaid to base themes, bounded pre-rendering, and lazy caching.
9. Add fixed GitHub Light print CSS, syntax, and Mermaid behavior.
10. Update SPEC, implementation notes, README, and theme attributions.
11. Complete Go, Bun, rendering, browser, visual, integration, and broad
    verification.

## 11. Open questions

None. The two-pull-request boundary, Bun package management, TypeScript/Oxc/CSS
tooling, built-in catalog, arbitrary cross-slot assignment, Mermaid strategy,
syntax strategy, lack of Preact, and fixed GitHub Light print behavior are all
settled for implementation.
