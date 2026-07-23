// Package api exposes Airplan's authoritative REST wire contract.
package api

import _ "embed"

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config oapi-codegen.yaml openapi.yaml

// openAPISource is kept byte-for-byte so /openapi.yaml can serve the exact
// checked-in contract.
//
//go:embed openapi.yaml
var openAPISource []byte

// OpenAPI returns a copy of the embedded OpenAPI source.
func OpenAPI() []byte {
	return append([]byte(nil), openAPISource...)
}
