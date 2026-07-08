package airplan

import (
	"encoding/json"
	"reflect"

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

	out, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
