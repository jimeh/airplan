package airplan

import (
	"encoding/json"
	"reflect"
	"sort"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/invopop/jsonschema"
)

// configSchemaID deliberately points at the latest release rather
// than a versioned URL: the schema is additive-stable, the README's
// #:schema directive references the same URL, and a per-version $id
// would either desynchronize the committed golden copy from release
// assets or force regenerating it on every release (release-please
// owns versions; there is no version file to stamp at build time).
const configSchemaID = "https://github.com/jimeh/airplan/releases/latest/" +
	"download/airplan.schema.json"

// ConfigSchema returns the generated JSON Schema for airplan config files.
func ConfigSchema() ([]byte, error) {
	reflector := jsonschema.Reflector{
		ExpandedStruct:             true,
		RequiredFromJSONSchemaTags: true,
	}
	schema := reflector.ReflectFromType(reflect.TypeOf(FileConfig{}))
	schema.ID = jsonschema.ID(configSchemaID)
	schema.Title = "Airplan Config"
	schema.Description = "Airplan TOML config file."
	customizeThemeSchema(schema)

	out, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func customizeThemeSchema(schema *jsonschema.Schema) {
	themes, ok := schema.Properties.Get("themes")
	if ok {
		maximum := uint64(maxThemeIDLength)
		reserved := make([]any, 0, len(builtinThemes()))
		for _, theme := range builtinThemes() {
			reserved = append(reserved, theme.ID)
		}
		themes.PropertyNames = &jsonschema.Schema{
			Type:      "string",
			Pattern:   themeIDPattern.String(),
			MaxLength: &maximum,
			Not:       &jsonschema.Schema{Enum: reserved},
		}
	}

	themeConfig := schema.Definitions["ThemeConfig"]
	if themeConfig == nil || themeConfig.Properties == nil {
		return
	}
	syntax, ok := themeConfig.Properties.Get("syntax")
	if !ok {
		return
	}
	names := append([]string(nil), styles.Names()...)
	sort.Strings(names)
	syntax.Enum = make([]any, 0, len(names)+2)
	syntax.Enum = append(syntax.Enum, "", "derived")
	for _, name := range names {
		syntax.Enum = append(syntax.Enum, "chroma:"+name)
	}
}
