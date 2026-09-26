package docs

import (
	_ "embed"
)

// OpenAPIYAML holds the embedded OpenAPI 3.0 specification.
//
//go:embed openapi.yaml
var OpenAPIYAML []byte
