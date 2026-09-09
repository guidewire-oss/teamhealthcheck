// It reads docs/swagger/examples.json (definition name -> field name ->
// example value) and applies it to docs/swagger/swagger.json,
// docs/swagger/swagger.yaml, and the embedded spec in docs/swagger/docs.go.
//
// Run via `make swagger-gen`, after `swag init` has produced fresh output.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const swaggerDir = "docs/swagger"

func main() {
	examples, err := loadExamples(swaggerDir + "/examples.json")
	if err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}

	specBytes, err := os.ReadFile(swaggerDir + "/swagger.json")
	if err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(specBytes, &doc); err != nil {
		log.Fatalf("swagger-examples: parsing swagger.json: %v", err)
	}

	definitions, _ := doc["definitions"].(map[string]any)
	if definitions == nil {
		log.Fatalf("swagger-examples: swagger.json has no \"definitions\" object")
	}

	if err := applyExamples(definitions, examples); err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}

	jsonOut, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}
	if err := os.WriteFile(swaggerDir+"/swagger.json", append(jsonOut, '\n'), 0o644); err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}

	yamlOut, err := yaml.Marshal(doc)
	if err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}
	if err := os.WriteFile(swaggerDir+"/swagger.yaml", yamlOut, 0o644); err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}

	definitionsJSON, err := json.MarshalIndent(definitions, "    ", "    ")
	if err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}
	if err := patchDocsGo(swaggerDir+"/docs.go", definitionsJSON); err != nil {
		log.Fatalf("swagger-examples: %v", err)
	}

	fmt.Println("swagger-examples: injected examples into swagger.json, swagger.yaml, docs.go")
}

// loadExamples reads the definition -> field -> example map, ignoring the
// "_comment" key used for documentation inside the JSON file itself.
func loadExamples(path string) (map[string]map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rawTop map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawTop); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	delete(rawTop, "_comment")

	parsed := make(map[string]map[string]string, len(rawTop))
	for defName, fieldsRaw := range rawTop {
		var fields map[string]string
		if err := json.Unmarshal(fieldsRaw, &fields); err != nil {
			return nil, fmt.Errorf("parsing %s: definition %q: %w", path, defName, err)
		}
		parsed[defName] = fields
	}
	return parsed, nil
}

// applyExamples sets schema["example"] for every definition/field pair found
// in examples, coercing the configured string value to match the field's
// declared Swagger type. It fails loudly if examples.json references a
// definition or field that no longer exists, so the mapping can't silently
// drift out of sync with the DTOs.
func applyExamples(definitions map[string]any, examples map[string]map[string]string) error {
	for defName, fields := range examples {
		defSchema, ok := definitions[defName].(map[string]any)
		if !ok {
			return fmt.Errorf("examples.json references unknown definition %q (renamed or removed DTO/@name?)", defName)
		}
		props, ok := defSchema["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("definition %q has no properties to attach examples to", defName)
		}
		for field, raw := range fields {
			propSchema, ok := props[field].(map[string]any)
			if !ok {
				return fmt.Errorf("examples.json references unknown field %q on definition %q", field, defName)
			}
			coerced, err := coerceExample(propSchema, raw)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", defName, field, err)
			}
			propSchema["example"] = coerced
		}
	}
	return nil
}

func coerceExample(schema map[string]any, raw string) (any, error) {
	schemaType, _ := schema["type"].(string)
	if schemaType == "array" {
		itemsSchema, _ := schema["items"].(map[string]any)
		itemType, _ := itemsSchema["type"].(string)
		parts := strings.Split(raw, ",")
		out := make([]any, len(parts))
		for i, p := range parts {
			v, err := coerceScalar(itemType, strings.TrimSpace(p))
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	}
	return coerceScalar(schemaType, raw)
}

func coerceScalar(schemaType, raw string) (any, error) {
	switch schemaType {
	case "boolean":
		return strconv.ParseBool(raw)
	case "integer":
		return strconv.ParseInt(raw, 10, 64)
	case "number":
		return strconv.ParseFloat(raw, 64)
	default:
		return raw, nil
	}
}

// patchDocsGo replaces the "definitions": {...} block inside docs.go's
// docTemplate string with newDefinitionsJSON, leaving the surrounding Go
// template placeholders (schemes/info/host/basePath) untouched.
func patchDocsGo(path string, newDefinitionsJSON []byte) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	const key = `"definitions": {`
	start := bytes.Index(content, []byte(key))
	if start == -1 {
		return fmt.Errorf("%s: could not find %q", path, key)
	}
	braceStart := start + len(key) - 1 // index of the opening '{'

	end, err := matchingBrace(content, braceStart)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	replacement := append([]byte(`"definitions": `), newDefinitionsJSON...)
	patched := make([]byte, 0, len(content)-((end+1)-start)+len(replacement))
	patched = append(patched, content[:start]...)
	patched = append(patched, replacement...)
	patched = append(patched, content[end+1:]...)

	return os.WriteFile(path, patched, 0o644)
}

// matchingBrace returns the index of the '}' that closes the '{' at
// content[openIdx], scanning JSON-aware so braces inside string literals
// (including escaped quotes) are ignored.
func matchingBrace(content []byte, openIdx int) (int, error) {
	depth := 0
	inString := false
	escaped := false
	for i := openIdx; i < len(content); i++ {
		c := content[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unbalanced braces starting at offset %d", openIdx)
}
