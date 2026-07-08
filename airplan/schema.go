package airplan

import (
	"encoding/json"
	"reflect"

	"github.com/invopop/jsonschema"
)

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
