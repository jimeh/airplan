package airplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
)

const (
	DefaultLightTheme  = "github-light"
	DefaultDarkTheme   = "github-dark"
	maxThemeIDLength   = 48
	maxThemeNameLength = 80
)

// ThemeVariant describes the palette itself. It does not constrain the mode
// slot to which a reader may assign the theme.
type ThemeVariant string

const (
	ThemeVariantLight ThemeVariant = "light"
	ThemeVariantDark  ThemeVariant = "dark"
)

// ThemeTokens is the deliberately small semantic palette accepted for custom
// themes. Values are normalized to lowercase six- or eight-digit hex.
type ThemeTokens struct {
	Background       string `toml:"background" json:"background" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Foreground       string `toml:"foreground" json:"foreground" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Muted            string `toml:"muted" json:"muted" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Accent           string `toml:"accent" json:"accent" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	AccentForeground string `toml:"accent_foreground" json:"accent_foreground" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Border           string `toml:"border" json:"border" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Surface          string `toml:"surface" json:"surface" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	SurfaceEmphasis  string `toml:"surface_emphasis" json:"surface_emphasis" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Info             string `toml:"info" json:"info" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Success          string `toml:"success" json:"success" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Important        string `toml:"important" json:"important" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Warning          string `toml:"warning" json:"warning" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
	Danger           string `toml:"danger" json:"danger" jsonschema:"required,pattern=^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$"`
}

// ThemeConfig is one user-authored global theme definition.
type ThemeConfig struct {
	Name    string       `toml:"name" json:"name" jsonschema:"required,minLength=1,maxLength=80"`
	Variant ThemeVariant `toml:"variant" json:"variant" jsonschema:"required,enum=light,enum=dark"`
	ThemeTokens
	Syntax string `toml:"syntax" json:"syntax,omitempty" jsonschema_description:"Empty or derived, or chroma:<registered-style-name>. Empty derives highlighting from semantic tokens."`
}

// Theme is one normalized catalog entry.
type Theme struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Variant ThemeVariant `json:"variant"`
	Tokens  ThemeTokens  `json:"tokens"`
	Syntax  string       `json:"syntax"`
}

type browserTheme struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Variant ThemeVariant   `json:"variant"`
	Mermaid map[string]any `json:"mermaid"`
}

type browserThemeCatalog struct {
	DefaultLight string         `json:"defaultLight"`
	DefaultDark  string         `json:"defaultDark"`
	Themes       []browserTheme `json:"themes"`
}

// ThemeBundle is the immutable, deterministic rendering input shared by the
// document and collection templates.
type ThemeBundle struct {
	Catalog       []Theme
	DefaultLight  string
	DefaultDark   string
	CatalogSHA256 string
	CSS           template.CSS
	SyntaxCSS     template.CSS
	CatalogJSON   template.JS
}

var themeIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

func builtinThemes() []Theme {
	return []Theme{
		builtinTheme("github-light", "GitHub Light", ThemeVariantLight, "github", ThemeTokens{"#ffffff", "#1f2328", "#59636e", "#0969da", "#ffffff", "#d1d9e0", "#f6f8fa", "#eff1f3", "#0969da", "#1a7f37", "#8250df", "#9a6700", "#cf222e"}),
		builtinTheme("catppuccin-latte", "Catppuccin Latte", ThemeVariantLight, "catppuccin-latte", ThemeTokens{"#eff1f5", "#4c4f69", "#6c6f85", "#1e66f5", "#eff1f5", "#bcc0cc", "#e6e9ef", "#dce0e8", "#209fb5", "#40a02b", "#8839ef", "#df8e1d", "#d20f39"}),
		builtinTheme("rose-pine-dawn", "Rose Pine Dawn", ThemeVariantLight, "rose-pine-dawn", ThemeTokens{"#faf4ed", "#575279", "#797593", "#907aa9", "#faf4ed", "#dfdad9", "#fffaf3", "#f2e9e1", "#286983", "#56949f", "#907aa9", "#ea9d34", "#b4637a"}),
		builtinTheme("solarized-light", "Solarized Light", ThemeVariantLight, "solarized-light", ThemeTokens{"#fdf6e3", "#586e75", "#839496", "#268bd2", "#fdf6e3", "#eee8d5", "#eee8d5", "#e5dfcc", "#2aa198", "#859900", "#6c71c4", "#b58900", "#dc322f"}),
		builtinTheme("tokyo-night-day", "Tokyo Night Day", ThemeVariantLight, "tokyonight-day", ThemeTokens{"#e1e2e7", "#3760bf", "#6172b0", "#2e7de9", "#e1e2e7", "#a8aecb", "#d4d6e4", "#c4c8da", "#007197", "#587539", "#7847bd", "#8c6c3e", "#f52a65"}),
		builtinTheme("github-dark", "GitHub Dark", ThemeVariantDark, "github-dark", ThemeTokens{"#0d1117", "#e6edf3", "#9198a1", "#4493f8", "#0d1117", "#3d444d", "#151b23", "#212830", "#58a6ff", "#3fb950", "#a371f7", "#d29922", "#f85149"}),
		builtinTheme("catppuccin-mocha", "Catppuccin Mocha", ThemeVariantDark, "catppuccin-mocha", ThemeTokens{"#1e1e2e", "#cdd6f4", "#a6adc8", "#89b4fa", "#1e1e2e", "#45475a", "#181825", "#313244", "#89dceb", "#a6e3a1", "#cba6f7", "#f9e2af", "#f38ba8"}),
		builtinTheme("rose-pine", "Rose Pine", ThemeVariantDark, "rose-pine", ThemeTokens{"#191724", "#e0def4", "#908caa", "#c4a7e7", "#191724", "#403d52", "#1f1d2e", "#26233a", "#9ccfd8", "#31748f", "#c4a7e7", "#f6c177", "#eb6f92"}),
		builtinTheme("solarized-dark", "Solarized Dark", ThemeVariantDark, "solarized-dark", ThemeTokens{"#002b36", "#93a1a1", "#839496", "#268bd2", "#002b36", "#586e75", "#073642", "#0b4652", "#2aa198", "#859900", "#6c71c4", "#b58900", "#dc322f"}),
		builtinTheme("tokyo-night", "Tokyo Night", ThemeVariantDark, "tokyonight-night", ThemeTokens{"#1a1b26", "#c0caf5", "#9aa5ce", "#7aa2f7", "#1a1b26", "#3b4261", "#24283b", "#292e42", "#7dcfff", "#9ece6a", "#bb9af7", "#e0af68", "#f7768e"}),
		builtinTheme("one-dark", "One Dark", ThemeVariantDark, "onedark", ThemeTokens{"#282c34", "#abb2bf", "#7f848e", "#61afef", "#282c34", "#4b5263", "#21252b", "#2c313c", "#56b6c2", "#98c379", "#c678dd", "#e5c07b", "#e06c75"}),
	}
}

func builtinTheme(id, name string, variant ThemeVariant, syntax string, tokens ThemeTokens) Theme {
	return Theme{ID: id, Name: name, Variant: variant, Syntax: "chroma:" + syntax, Tokens: tokens}
}

// ResolveThemeBundle validates and resolves the complete page-local catalog.
func ResolveThemeBundle(lightID, darkID string, custom map[string]ThemeConfig) (*ThemeBundle, error) {
	if lightID == "" {
		lightID = DefaultLightTheme
	}
	if darkID == "" {
		darkID = DefaultDarkTheme
	}
	catalog := builtinThemes()
	reserved := make(map[string]bool, len(catalog))
	for _, theme := range catalog {
		reserved[theme.ID] = true
	}
	ids := make([]string, 0, len(custom))
	for id := range custom {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if reserved[id] {
			return nil, fmt.Errorf("airplan: custom theme ID %q is reserved", id)
		}
		cfg := custom[id]
		theme, err := normalizeTheme(id, cfg)
		if err != nil {
			return nil, err
		}
		catalog = append(catalog, theme)
	}
	known := make(map[string]bool, len(catalog))
	for _, theme := range catalog {
		known[theme.ID] = true
	}
	if !known[lightID] {
		return nil, fmt.Errorf("airplan: light_theme %q does not exist", lightID)
	}
	if !known[darkID] {
		return nil, fmt.Errorf("airplan: dark_theme %q does not exist", darkID)
	}

	canonical, err := json.Marshal(catalog)
	if err != nil {
		return nil, fmt.Errorf("airplan: encode theme catalog: %w", err)
	}
	sum := sha256.Sum256(canonical)
	browser := browserThemeCatalog{DefaultLight: lightID, DefaultDark: darkID}
	for _, theme := range catalog {
		browser.Themes = append(browser.Themes, browserTheme{ID: theme.ID, Name: theme.Name, Variant: theme.Variant, Mermaid: mermaidVariables(theme)})
	}
	browserJSON, err := json.Marshal(browser)
	if err != nil {
		return nil, fmt.Errorf("airplan: encode browser theme catalog: %w", err)
	}
	css, err := renderThemeCSS(catalog, lightID, darkID)
	if err != nil {
		return nil, err
	}
	syntax, err := renderThemeSyntaxCSS(catalog, lightID, darkID)
	if err != nil {
		return nil, err
	}
	return &ThemeBundle{Catalog: catalog, DefaultLight: lightID, DefaultDark: darkID, CatalogSHA256: hex.EncodeToString(sum[:]), CSS: template.CSS(css), SyntaxCSS: template.CSS(syntax), CatalogJSON: template.JS(browserJSON)}, nil
}

func normalizeTheme(id string, cfg ThemeConfig) (Theme, error) {
	if len(id) > maxThemeIDLength || !themeIDPattern.MatchString(id) {
		return Theme{}, fmt.Errorf("airplan: invalid custom theme ID %q: use a lowercase ASCII slug up to %d characters", id, maxThemeIDLength)
	}
	if !utf8.ValidString(cfg.Name) || strings.TrimSpace(cfg.Name) == "" || utf8.RuneCountInString(cfg.Name) > maxThemeNameLength {
		return Theme{}, fmt.Errorf("airplan: theme %q name must be non-empty valid UTF-8 up to %d characters", id, maxThemeNameLength)
	}
	if cfg.Variant != ThemeVariantLight && cfg.Variant != ThemeVariantDark {
		return Theme{}, fmt.Errorf("airplan: theme %q variant must be light or dark", id)
	}
	tokens := cfg.ThemeTokens
	fields := []struct {
		name  string
		value *string
	}{
		{"background", &tokens.Background}, {"foreground", &tokens.Foreground}, {"muted", &tokens.Muted}, {"accent", &tokens.Accent}, {"accent_foreground", &tokens.AccentForeground}, {"border", &tokens.Border}, {"surface", &tokens.Surface}, {"surface_emphasis", &tokens.SurfaceEmphasis}, {"info", &tokens.Info}, {"success", &tokens.Success}, {"important", &tokens.Important}, {"warning", &tokens.Warning}, {"danger", &tokens.Danger},
	}
	for _, field := range fields {
		normalized, err := normalizeHex(*field.value)
		if err != nil {
			return Theme{}, fmt.Errorf("airplan: theme %q %s: %w", id, field.name, err)
		}
		*field.value = normalized
	}
	syntax := cfg.Syntax
	if syntax == "" {
		syntax = "derived"
	}
	if syntax != "derived" {
		if !strings.HasPrefix(syntax, "chroma:") || !registeredChromaStyle(strings.TrimPrefix(syntax, "chroma:")) {
			return Theme{}, fmt.Errorf("airplan: theme %q syntax %q is not derived or a registered chroma:<name> style", id, syntax)
		}
	}
	return Theme{ID: id, Name: strings.TrimSpace(cfg.Name), Variant: cfg.Variant, Tokens: tokens, Syntax: syntax}, nil
}

func normalizeHex(value string) (string, error) {
	if len(value) != 7 && len(value) != 9 {
		return "", errors.New("must be #rrggbb or #rrggbbaa")
	}
	if value[0] != '#' {
		return "", errors.New("must begin with #")
	}
	if _, err := hex.DecodeString(value[1:]); err != nil {
		return "", errors.New("must contain only hexadecimal digits")
	}
	return strings.ToLower(value), nil
}

func registeredChromaStyle(name string) bool {
	for _, candidate := range styles.Names() {
		if candidate == name {
			return true
		}
	}
	return false
}

func renderThemeCSS(catalog []Theme, lightID, darkID string) (string, error) {
	byID := map[string]Theme{}
	for _, theme := range catalog {
		byID[theme.ID] = theme
	}
	var out strings.Builder
	writeProperties := func(selector string, theme Theme) {
		t := theme.Tokens
		fmt.Fprintf(&out, "%s{color-scheme:%s;--bg:%s;--fg:%s;--heading:%s;--subheading:%s;--muted:%s;--link:%s;--border:%s;--surface-subtle:%s;--code-inline-bg:%s;--code-bg:%s;--code-border:%s;--control-track:%s;--control-active:%s;--control-hover:%s;--control-divider:%s;--on-link:%s;--info:%s;--success:%s;--important:%s;--warning:%s;--danger:%s;--alert-info-bg:%s;--alert-success-bg:%s;--alert-important-bg:%s;--alert-warning-bg:%s;--alert-danger-bg:%s;--dialog-backdrop:%s}\n",
			selector, theme.Variant, t.Background, t.Foreground, t.Foreground, mixHex(t.Foreground, t.Background, 82), t.Muted, t.Accent, t.Border, t.Surface, mixHex(t.Surface, t.Background, 55), t.Surface, mixHex(t.Border, t.Surface, 65), t.Surface, t.SurfaceEmphasis, mixHex(t.SurfaceEmphasis, t.Background, 55), mixHex(t.Border, t.Background, 75), t.AccentForeground, t.Info, t.Success, t.Important, t.Warning, t.Danger, mixHex(t.Info, t.Background, 12), mixHex(t.Success, t.Background, 12), mixHex(t.Important, t.Background, 12), mixHex(t.Warning, t.Background, 12), mixHex(t.Danger, t.Background, 12), "#00000080")
	}
	writeProperties(`:root`, byID[lightID])
	// No-JS dark system default follows the uploader's configured dark slot.
	out.WriteString("@media screen and (prefers-color-scheme:dark){")
	writeProperties(`:root:not([data-airplan-theme])`, byID[darkID])
	out.WriteString("}\n")
	for _, theme := range catalog {
		writeProperties(`:root[data-airplan-theme="`+theme.ID+`"]`, theme)
	}
	// Print is a fixed rendering target and deliberately ignores every slot.
	out.WriteString("@media print{")
	writeProperties(`:root,:root[data-airplan-theme]`, byID[DefaultLightTheme])
	out.WriteString("body{background:#fff;color:#1f2328}}\n")
	return out.String(), nil
}

func mixHex(foreground, background string, percent int) string {
	fg := compositeHex(foreground, background)
	bg := compositeHex(background, "#ffffff")
	decode := func(value string, offset int) int {
		n, _ := strconv.ParseInt(value[offset:offset+2], 16, 0)
		return int(n)
	}
	var b strings.Builder
	b.WriteByte('#')
	for _, offset := range []int{1, 3, 5} {
		value := (decode(fg, offset)*percent + decode(bg, offset)*(100-percent) + 50) / 100
		fmt.Fprintf(&b, "%02x", value)
	}
	return b.String()
}

func compositeHex(value, background string) string {
	if len(value) == 7 {
		return value
	}
	if len(value) != 9 {
		return value
	}
	alpha, _ := strconv.ParseInt(value[7:9], 16, 0)
	percent := int((alpha*100 + 127) / 255)
	return mixHex(value[:7], background, percent)
}

func renderThemeSyntaxCSS(catalog []Theme, lightID, darkID string) (string, error) {
	formatter := chromahtml.New(chromahtml.WithClasses(true), chromahtml.WithCSSComments(false))
	var out strings.Builder
	cssByID := make(map[string]string, len(catalog))
	type styleGroup struct {
		css    string
		scopes []string
	}
	groups := make([]styleGroup, 0, len(catalog))
	groupByCSS := make(map[string]int, len(catalog))
	for _, theme := range catalog {
		style, err := syntaxStyle(theme)
		if err != nil {
			return "", err
		}
		var css strings.Builder
		if err := formatter.WriteCSS(&css, style); err != nil {
			return "", fmt.Errorf("airplan: render syntax style for %q: %w", theme.ID, err)
		}
		cssByID[theme.ID] = css.String()
		scope := `:root[data-airplan-theme="` + theme.ID + `"]`
		if index, ok := groupByCSS[css.String()]; ok {
			groups[index].scopes = append(groups[index].scopes, scope)
		} else {
			groupByCSS[css.String()] = len(groups)
			groups = append(groups, styleGroup{css: css.String(), scopes: []string{scope}})
		}
	}
	out.WriteString("@media screen{")
	for _, group := range groups {
		css, err := compactScopedSyntaxCSS(group.css, group.scopes...)
		if err != nil {
			return "", err
		}
		out.WriteString(css)
	}
	out.WriteString("}@media screen and (prefers-color-scheme:light){")
	lightCSS, err := compactScopedSyntaxCSS(cssByID[lightID], `:root:not([data-airplan-theme])`)
	if err != nil {
		return "", err
	}
	out.WriteString(lightCSS)
	out.WriteString("}@media screen and (prefers-color-scheme:dark){")
	darkCSS, err := compactScopedSyntaxCSS(cssByID[darkID], `:root:not([data-airplan-theme])`)
	if err != nil {
		return "", err
	}
	out.WriteString(darkCSS)
	out.WriteByte('}')
	var printCSS strings.Builder
	if err := formatter.WriteCSS(&printCSS, styles.Get("github")); err != nil {
		return "", err
	}
	compactPrintCSS, err := compactScopedSyntaxCSS(printCSS.String())
	if err != nil {
		return "", err
	}
	out.WriteString("@media print{" + compactPrintCSS + "}")
	return out.String(), nil
}

func syntaxStyle(theme Theme) (*chroma.Style, error) {
	if strings.HasPrefix(theme.Syntax, "chroma:") {
		return styles.Get(strings.TrimPrefix(theme.Syntax, "chroma:")), nil
	}
	t := theme.Tokens
	builder := chroma.NewStyleBuilder("airplan-" + theme.ID)
	builder.Add(chroma.Background, "bg:"+compositeHex(t.Background, t.Background))
	builder.Add(chroma.Text, compositeHex(t.Foreground, t.Background))
	for _, token := range []chroma.TokenType{chroma.Comment, chroma.CommentHashbang, chroma.CommentMultiline, chroma.CommentSingle, chroma.LineNumbers, chroma.LineNumbersTable} {
		builder.Add(token, compositeHex(t.Muted, t.Background))
	}
	for _, token := range []chroma.TokenType{chroma.Keyword, chroma.KeywordDeclaration, chroma.KeywordNamespace, chroma.NameFunction, chroma.NameTag, chroma.NameDecorator} {
		builder.Add(token, compositeHex(t.Accent, t.Background))
	}
	for _, token := range []chroma.TokenType{chroma.LiteralString, chroma.LiteralStringDouble, chroma.LiteralStringSingle, chroma.GenericInserted} {
		builder.Add(token, compositeHex(t.Success, t.Background))
	}
	for _, token := range []chroma.TokenType{chroma.LiteralNumber, chroma.KeywordType, chroma.NameClass, chroma.NameConstant} {
		builder.Add(token, compositeHex(t.Warning, t.Background))
	}
	for _, token := range []chroma.TokenType{chroma.NameBuiltin, chroma.NameVariableMagic, chroma.GenericHeading} {
		builder.Add(token, compositeHex(t.Important, t.Background))
	}
	for _, token := range []chroma.TokenType{chroma.Error, chroma.GenericDeleted} {
		builder.Add(token, compositeHex(t.Danger, t.Background))
	}
	builder.Add(chroma.GenericEmph, "italic")
	builder.Add(chroma.GenericStrong, "bold")
	builder.Add(chroma.GenericTraceback, "bg:"+compositeHex(t.SurfaceEmphasis, t.Background))
	return builder.Build()
}

func mermaidVariables(theme Theme) map[string]any {
	t := theme.Tokens
	background := compositeHex(t.Background, t.Background)
	return map[string]any{
		"airplanTheme": theme.ID,
		"background":   background, "darkMode": theme.Variant == ThemeVariantDark,
		"primaryColor": mixHex(t.Accent, background, 18), "primaryTextColor": compositeHex(t.Foreground, background), "primaryBorderColor": compositeHex(t.Border, background),
		"secondaryColor": mixHex(t.Success, background, 14), "secondaryTextColor": compositeHex(t.Foreground, background), "secondaryBorderColor": compositeHex(t.Border, background),
		"tertiaryColor": mixHex(t.Important, background, 12), "tertiaryTextColor": compositeHex(t.Foreground, background), "tertiaryBorderColor": compositeHex(t.Border, background),
		"textColor": compositeHex(t.Foreground, background), "lineColor": compositeHex(t.Muted, background), "edgeLabelBackground": compositeHex(t.Surface, background),
		"fontFamily": `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
	}
}

// ThemeRecipe is the render-provenance identity introduced by marker v5.
type ThemeRecipe struct {
	DefaultLight  string `json:"default_light"`
	DefaultDark   string `json:"default_dark"`
	CatalogSHA256 string `json:"catalog_sha256"`
}

func themeRecipe(bundle *ThemeBundle) ThemeRecipe {
	if bundle == nil {
		bundle = defaultThemeBundle()
	}
	return ThemeRecipe{DefaultLight: bundle.DefaultLight, DefaultDark: bundle.DefaultDark, CatalogSHA256: bundle.CatalogSHA256}
}

func themeRecipePtr(bundle *ThemeBundle) *ThemeRecipe {
	value := themeRecipe(bundle)
	return &value
}

var defaultThemeBundle = sync.OnceValue(func() *ThemeBundle {
	bundle, err := ResolveThemeBundle("", "", nil)
	if err != nil {
		panic(err)
	}
	return bundle
})
