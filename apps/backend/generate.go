package backend

// Generation tools are pinned in go.mod's tool block so contributors do not
// need global sqlc or oapi-codegen installations.
//
//go:generate go run ./internal/tooling/openapiconvert -input ../../api/openapi.yaml -output internal/adapter/httpapi/generated/openapi.codegen.yaml
//go:generate go tool sqlc generate
//go:generate go tool oapi-codegen --config internal/adapter/httpapi/oapi-codegen.yaml internal/adapter/httpapi/generated/openapi.codegen.yaml
