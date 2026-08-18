package airplan

import (
	"encoding/json"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestResolveThemeBundleBuiltinsAndCrossVariantDefaults(t *testing.T) {
	bundle, err := ResolveThemeBundle("one-dark", "tokyo-night-day", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Catalog) != 11 {
		t.Fatalf("catalog count = %d, want 11", len(bundle.Catalog))
	}
	if bundle.DefaultLight != "one-dark" || bundle.DefaultDark != "tokyo-night-day" {
		t.Fatalf("defaults = %s/%s", bundle.DefaultLight, bundle.DefaultDark)
	}
	if want := "6ef869946a1ccb9fc60de82d7643b0a5267d0a5e33c4ed0ba9b80ad98f18fb36"; bundle.CatalogSHA256 != want {
		t.Fatalf("built-in catalog digest = %q, want %q", bundle.CatalogSHA256, want)
	}
	for _, theme := range bundle.Catalog {
		id := theme.ID
		if !strings.Contains(string(bundle.CSS), `data-airplan-theme="`+id+`"`) {
			t.Errorf("CSS missing %s", id)
		}
		if !strings.Contains(string(bundle.SyntaxCSS), `data-airplan-theme="`+id+`"`) {
			t.Errorf("syntax CSS missing %s", id)
		}
		if name := strings.TrimPrefix(theme.Syntax, "chroma:"); !registeredChromaStyle(name) {
			t.Errorf("built-in %s Chroma style %q is not registered", id, name)
		}
	}
	if !strings.Contains(string(bundle.CSS), "@media print") || !strings.Contains(string(bundle.SyntaxCSS), "@media print") {
		t.Fatal("fixed print CSS is missing")
	}
}

func TestResolveThemeBundlePrintCSSIgnoresConfiguredDefaults(t *testing.T) {
	defaults, err := ResolveThemeBundle("", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	crossVariant, err := ResolveThemeBundle("one-dark", "tokyo-night-day", nil)
	if err != nil {
		t.Fatal(err)
	}
	printBlock := func(css template.CSS) string {
		t.Helper()
		value := string(css)
		start := strings.LastIndex(value, "@media print{")
		if start < 0 {
			t.Fatal("print CSS block is missing")
		}
		return value[start:]
	}
	if printBlock(defaults.CSS) != printBlock(crossVariant.CSS) {
		t.Fatal("semantic print CSS changed with configured defaults")
	}
	if printBlock(defaults.SyntaxCSS) != printBlock(crossVariant.SyntaxCSS) {
		t.Fatal("syntax print CSS changed with configured defaults")
	}
}

func TestResolveThemeBundleAcceptsExactRegisteredChromaStyle(t *testing.T) {
	theme := validCustomTheme()
	theme.Syntax = "chroma:github"
	bundle, err := ResolveThemeBundle("custom", DefaultDarkTheme, map[string]ThemeConfig{
		"custom": theme,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := bundle.Catalog[len(bundle.Catalog)-1].Syntax; got != "chroma:github" {
		t.Fatalf("syntax selector = %q", got)
	}
}

func TestResolveThemeBundleCustomNormalizationAndDeterminism(t *testing.T) {
	theme := validCustomTheme()
	theme.Background = "#AABBCCDD"
	first, err := ResolveThemeBundle("custom", DefaultDarkTheme, map[string]ThemeConfig{"custom": theme})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveThemeBundle("custom", DefaultDarkTheme, map[string]ThemeConfig{"custom": theme})
	if err != nil {
		t.Fatal(err)
	}
	if first.CatalogSHA256 != second.CatalogSHA256 || first.CSS != second.CSS || first.SyntaxCSS != second.SyntaxCSS || first.CatalogJSON != second.CatalogJSON {
		t.Fatal("equivalent theme bundles are not byte-deterministic")
	}
	if got := first.Catalog[len(first.Catalog)-1].Tokens.Background; got != "#aabbccdd" {
		t.Fatalf("normalized background = %q", got)
	}
	var browser browserThemeCatalog
	if err := json.Unmarshal([]byte(first.CatalogJSON), &browser); err != nil {
		t.Fatal(err)
	}
	if browser.DefaultLight != "custom" || len(browser.Themes) != 12 {
		t.Fatalf("browser catalog = %#v", browser)
	}
}

func TestResolveThemeBundleSortsCustomThemesBeforeSerialization(t *testing.T) {
	alpha := validCustomTheme()
	alpha.Name = "Alpha"
	zulu := validCustomTheme()
	zulu.Name = "Zulu"
	first, err := ResolveThemeBundle("alpha", "zulu", map[string]ThemeConfig{
		"zulu": zulu, "alpha": alpha,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveThemeBundle("alpha", "zulu", map[string]ThemeConfig{
		"alpha": alpha, "zulu": zulu,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.CatalogSHA256 != second.CatalogSHA256 || first.CatalogJSON != second.CatalogJSON ||
		first.CSS != second.CSS || first.SyntaxCSS != second.SyntaxCSS {
		t.Fatal("custom map insertion order changed resolved bundle bytes")
	}
	if first.Catalog[11].ID != "alpha" || first.Catalog[12].ID != "zulu" {
		t.Fatalf("custom catalog order = %s, %s", first.Catalog[11].ID, first.Catalog[12].ID)
	}
}

func TestThemeCatalogJSONEscapesAdversarialDisplayName(t *testing.T) {
	theme := validCustomTheme()
	theme.Name = `Custom </script>{{bad}} & name`
	bundle, err := ResolveThemeBundle("custom", DefaultDarkTheme, map[string]ThemeConfig{"custom": theme})
	if err != nil {
		t.Fatal(err)
	}
	out, err := RenderMarkdown([]byte("# Safe\n"), RenderOptions{Title: "Safe", Themes: bundle})
	if err != nil {
		t.Fatal(err)
	}
	page := string(out)
	if strings.Contains(page, theme.Name) || !strings.Contains(page, `Custom \u003c/script\u003e{{bad}} \u0026 name`) {
		t.Fatalf("catalog display name was not safely JSON-escaped")
	}
}

func TestBuiltinThemeContrastInvariants(t *testing.T) {
	for _, theme := range builtinThemes() {
		t.Run(theme.ID, func(t *testing.T) {
			checks := []struct {
				name, foreground, background string
				minimum                      float64
			}{
				{"foreground", theme.Tokens.Foreground, theme.Tokens.Background, 4.5},
				{"muted", theme.Tokens.Muted, theme.Tokens.Background, 2.9},
				{"accent", theme.Tokens.Accent, theme.Tokens.Background, 3.0},
				{"accent fill", theme.Tokens.AccentForeground, theme.Tokens.Accent, 3.0},
			}
			for _, check := range checks {
				if ratio := contrastRatio(check.foreground, check.background); ratio < check.minimum {
					t.Errorf("%s contrast = %.2f, want >= %.1f", check.name, ratio, check.minimum)
				}
			}
		})
	}
}

func TestDerivedChromaStyleMapsSemanticTokens(t *testing.T) {
	theme, err := normalizeTheme("custom", validCustomTheme())
	if err != nil {
		t.Fatal(err)
	}
	style, err := syntaxStyle(theme)
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		token chroma.TokenType
		want  string
	}{
		{chroma.Text, theme.Tokens.Foreground},
		{chroma.Comment, theme.Tokens.Muted},
		{chroma.Keyword, theme.Tokens.Accent},
		{chroma.LiteralString, theme.Tokens.Success},
		{chroma.LiteralNumber, theme.Tokens.Warning},
		{chroma.NameBuiltin, theme.Tokens.Important},
		{chroma.Error, theme.Tokens.Danger},
	}
	for _, check := range checks {
		if got := style.Get(check.token).Colour.String(); !strings.EqualFold(got, check.want) {
			t.Errorf("%s color = %s, want %s", check.token, got, check.want)
		}
	}
}

func TestResolveThemeBundleRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name, id string
		mutate   func(*ThemeConfig)
	}{
		{"reserved", "github-light", func(*ThemeConfig) {}},
		{"malformed ID", "Bad_ID", func(*ThemeConfig) {}},
		{"missing token", "custom", func(theme *ThemeConfig) { theme.Danger = "" }},
		{"invalid color", "custom", func(theme *ThemeConfig) { theme.Accent = "red" }},
		{"invalid variant", "custom", func(theme *ThemeConfig) { theme.Variant = "sepia" }},
		{"unknown chroma", "custom", func(theme *ThemeConfig) { theme.Syntax = "chroma:definitely-not-real" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			theme := validCustomTheme()
			test.mutate(&theme)
			if _, err := ResolveThemeBundle(DefaultLightTheme, DefaultDarkTheme, map[string]ThemeConfig{test.id: theme}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if _, err := ResolveThemeBundle("missing", DefaultDarkTheme, nil); err == nil {
		t.Fatal("unknown default accepted")
	}
}

func TestResolveConfigThemePrecedenceAndGlobalCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "airplan.toml")
	theme := `[themes.custom]
name = "Custom"
variant = "dark"
background = "#101010"
foreground = "#eeeeee"
muted = "#aaaaaa"
accent = "#6699ff"
accent_foreground = "#101010"
border = "#444444"
surface = "#202020"
surface_emphasis = "#303030"
info = "#55bbdd"
success = "#66bb77"
important = "#bb88ee"
warning = "#ddbb55"
danger = "#ee6677"
syntax = "derived"
`
	source := "endpoint = \"https://s3.example.test\"\nbucket = \"plans\"\nlight_theme = \"solarized-light\"\ndark_theme = \"one-dark\"\n\n[profiles.work]\nlight_theme = \"custom\"\ndark_theme = \"github-light\"\n\n" + theme
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	resolution, err := ResolveConfig(ConfigOptions{Path: path, Profile: "work", Getenv: func(name string) string {
		if name == "AIRPLAN_DARK_THEME" {
			return "tokyo-night"
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Config.LightTheme != "custom" || resolution.Config.DarkTheme != "tokyo-night" {
		t.Fatalf("resolved defaults = %s/%s", resolution.Config.LightTheme, resolution.Config.DarkTheme)
	}
	if len(resolution.Config.ThemeBundle.Catalog) != 12 {
		t.Fatalf("catalog count = %d", len(resolution.Config.ThemeBundle.Catalog))
	}
	if got := resolution.Fields["light_theme"].Source; got == nil || got.Kind != ConfigSourceProfile {
		t.Fatalf("light source = %#v", got)
	}
	if got := resolution.Fields["dark_theme"].Source; got == nil || got.Kind != ConfigSourceEnv {
		t.Fatalf("dark source = %#v", got)
	}
}

func TestResolveConfigRejectsUnknownCustomThemeKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "airplan.toml")
	source := `[themes.custom]
name = "Custom"
variant = "dark"
background = "#101010"
foreground = "#eeeeee"
muted = "#aaaaaa"
accent = "#6699ff"
accent_foreground = "#101010"
border = "#444444"
surface = "#202020"
surface_emphasis = "#303030"
info = "#55bbdd"
success = "#66bb77"
important = "#bb88ee"
warning = "#ddbb55"
danger = "#ee6677"
mystery = "not allowed"
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveConfig(ConfigOptions{Path: path}); err == nil ||
		!strings.Contains(err.Error(), "mystery") {
		t.Fatalf("unknown custom theme key error = %v", err)
	}
}

func validCustomTheme() ThemeConfig {
	return ThemeConfig{Name: "Custom", Variant: ThemeVariantDark, Syntax: "derived", ThemeTokens: ThemeTokens{
		Background: "#101010", Foreground: "#eeeeee", Muted: "#aaaaaa", Accent: "#6699ff", AccentForeground: "#101010", Border: "#444444", Surface: "#202020", SurfaceEmphasis: "#303030", Info: "#55bbdd", Success: "#66bb77", Important: "#bb88ee", Warning: "#ddbb55", Danger: "#ee6677",
	}}
}

func contrastRatio(foreground, background string) float64 {
	foregroundLuminance := relativeLuminance(foreground)
	backgroundLuminance := relativeLuminance(background)
	return (math.Max(foregroundLuminance, backgroundLuminance) + 0.05) /
		(math.Min(foregroundLuminance, backgroundLuminance) + 0.05)
}

func relativeLuminance(color string) float64 {
	channels := make([]float64, 3)
	for index := range channels {
		value, _ := strconv.ParseUint(color[1+index*2:3+index*2], 16, 8)
		channel := float64(value) / 255
		if channel <= 0.04045 {
			channels[index] = channel / 12.92
		} else {
			channels[index] = math.Pow((channel+0.055)/1.055, 2.4)
		}
	}
	return channels[0]*0.2126 + channels[1]*0.7152 + channels[2]*0.0722
}
