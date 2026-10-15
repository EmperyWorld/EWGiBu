// Package openapiv1 embeds the OpenAPI 3.1 transport contract.
package openapiv1

import _ "embed"

//go:embed openapi.yaml
var YAML []byte
