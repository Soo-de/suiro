package api

import _ "embed" // for Spec

// Spec is openapi.yaml, the OpenAPI description of the HTTP/JSON API.
//
//go:embed openapi.yaml
var Spec []byte
