package main

import (
	"strings"
	"testing"
)

func TestConvertRemovesOpenAPI31DocumentFields(t *testing.T) {
	t.Parallel()

	converted, err := convert(`openapi: 3.1.0
jsonSchemaDialect: https://json-schema.org/draft/2020-12/schema
info:
  title: Example
  version: 1.0.0
  summary: Example API
  license:
    name: Proprietary
    identifier: LicenseRef-Proprietary
paths: {}
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, unsupported := range []string{
		"openapi: 3.1.0",
		"jsonSchemaDialect:",
		"summary: Example API",
		"license:",
		"identifier: LicenseRef-Proprietary",
	} {
		if strings.Contains(converted, unsupported) {
			t.Fatalf("converted document retained %q:\n%s", unsupported, converted)
		}
	}
	if !strings.Contains(converted, "openapi: 3.0.3") {
		t.Fatalf("converted document lost compatible info fields:\n%s", converted)
	}
}

func TestConvertNullableReferenceAddsOpenAPI30Type(t *testing.T) {
	t.Parallel()

	converted, err := convert(`properties:
  terminal:
    oneOf:
      - $ref: "#/components/schemas/Terminal"
      - type: "null"
`)
	if err != nil {
		t.Fatal(err)
	}
	want := `  terminal:
    type: object
    allOf:
      - $ref: "#/components/schemas/Terminal"
    nullable: true`
	if !strings.Contains(converted, want) {
		t.Fatalf("nullable reference conversion mismatch:\n%s", converted)
	}
}

func TestConvertNullableObjectUnionAddsOpenAPI30Type(t *testing.T) {
	t.Parallel()

	converted, err := convert(`properties:
  result:
    oneOf:
      - $ref: "#/components/schemas/First"
      - $ref: "#/components/schemas/Second"
      - type: "null"
`)
	if err != nil {
		t.Fatal(err)
	}
	want := `  result:
    type: object
    oneOf:
      - $ref: "#/components/schemas/First"
      - $ref: "#/components/schemas/Second"
    nullable: true`
	if !strings.Contains(converted, want) {
		t.Fatalf("nullable union conversion mismatch:\n%s", converted)
	}
}
