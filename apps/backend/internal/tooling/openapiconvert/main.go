// Command openapiconvert derives an OpenAPI 3.0 compatibility document from
// the authoritative OpenAPI 3.1 contract. oapi-codegen does not yet understand
// JSON Schema 2020-12 nullable type arrays, while the TypeScript generator and
// API validators consume the original document directly.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	input := flag.String("input", "", "authoritative OpenAPI 3.1 document")
	output := flag.String("output", "", "generated OpenAPI 3.0 document")
	flag.Parse()
	if *input == "" || *output == "" {
		exitf("both -input and -output are required")
	}

	source, err := os.ReadFile(*input)
	if err != nil {
		exitf("read input: %v", err)
	}
	converted, err := convert(string(source))
	if err != nil {
		exitf("convert input: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		exitf("create output directory: %v", err)
	}
	if err := os.WriteFile(*output, []byte(converted), 0o644); err != nil {
		exitf("write output: %v", err)
	}
}

func convert(source string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	output := make([]string, 0, len(lines)+16)

	for index := 0; index < len(lines); {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		indent := leadingWhitespace(line)

		if trimmed == "oneOf:" {
			end := index + 1
			for end < len(lines) {
				next := lines[end]
				if strings.TrimSpace(next) != "" &&
					len(leadingWhitespace(next)) <= len(indent) {
					break
				}
				end++
			}

			branches := lines[index+1 : end]
			hasNull := false
			branchCount := 0
			for _, branch := range branches {
				branchTrimmed := strings.TrimSpace(branch)
				if branchTrimmed == `- type: "null"` ||
					branchTrimmed == "- type: null" {
					hasNull = true
					continue
				}
				if strings.HasPrefix(branchTrimmed, "- ") {
					branchCount++
				}
			}
			if hasNull {
				if branchCount == 0 {
					return "", fmt.Errorf(
						"nullable oneOf at line %d has no value schema",
						index+1,
					)
				}
				if branchCount == 1 {
					output = append(output, indent+"allOf:")
				} else {
					output = append(output, line)
				}
				for _, branch := range branches {
					branchTrimmed := strings.TrimSpace(branch)
					if branchTrimmed == `- type: "null"` ||
						branchTrimmed == "- type: null" {
						continue
					}
					output = append(output, convertLine(branch)...)
				}
				output = append(output, indent+"nullable: true")
				index = end
				continue
			}
		}

		output = append(output, convertLine(line)...)
		index++
	}

	return strings.Join(output, "\n"), nil
}

func convertLine(line string) []string {
	indent := leadingWhitespace(line)
	trimmed := strings.TrimSpace(line)

	switch trimmed {
	case "openapi: 3.1.0":
		return []string{indent + "openapi: 3.0.3"}
	case `type: [string, "null"]`:
		return []string{indent + "type: string", indent + "nullable: true"}
	case `type: [integer, "null"]`:
		return []string{indent + "type: integer", indent + "nullable: true"}
	case "unevaluatedProperties: false":
		// OpenAPI 3.0 has no equivalent that composes safely through allOf.
		return nil
	}

	if strings.HasPrefix(trimmed, "const: ") {
		return []string{
			indent + "enum: [" + strings.TrimPrefix(trimmed, "const: ") + "]",
		}
	}
	if strings.HasPrefix(trimmed, "contentEncoding: ") {
		return nil
	}
	return []string{line}
}

func leadingWhitespace(value string) string {
	return value[:len(value)-len(strings.TrimLeft(value, " \t"))]
}

func exitf(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "openapiconvert: "+format+"\n", values...)
	os.Exit(1)
}
