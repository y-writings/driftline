package driftline

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSourceManifestSchemaMatchesParserAllowedKeys(t *testing.T) {
	schema := readSourceManifestSchema(t)
	allowed := allowedSourceManifestKeys()

	if got := stringValue(schema, "$schema"); got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected schema draft: %q", got)
	}
	assertFalseValue(t, "root additionalProperties", schema, "additionalProperties")
	assertSameStringSet(t, "root properties", propertyNames(objectValue(schema, "properties")), allowed[""])
	assertSameStringSet(t, "root required", stringArrayValue(schema, "required"), map[string]struct{}{"version": {}, "files": {}})

	filesSchema := objectValue(objectValue(schema, "properties"), "files")
	fileItemSchema := schemaRef(t, schema, stringValue(objectValue(filesSchema, "items"), "$ref"))
	assertFalseValue(t, "file item additionalProperties", fileItemSchema, "additionalProperties")
	fileProperties := propertyNames(objectValue(fileItemSchema, "properties"))
	assertSameStringSet(t, "file properties", fileProperties, allowed["files"])
	assertSameStringSet(t, "file required", stringArrayValue(fileItemSchema, "required"), map[string]struct{}{"id": {}, "source": {}})
	if _, ok := fileProperties["target"]; ok {
		t.Fatal("source manifest schema must not allow file target")
	}
}

func TestSourceManifestSchemaRejectsTrailingSlashSourcePaths(t *testing.T) {
	schema := readSourceManifestSchema(t)
	relativePath := objectValue(objectValue(schema, "$defs"), "relativePath")
	pattern := stringValue(relativePath, "pattern")

	if !strings.Contains(pattern, "(?!.*/$)") {
		t.Fatalf("relativePath pattern must reject trailing slash paths, got %q", pattern)
	}
}

func readSourceManifestSchema(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../../schema.json")
	if err != nil {
		t.Fatalf("read schema.json: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse schema.json: %v", err)
	}
	return schema
}

func schemaRef(t *testing.T, schema map[string]any, ref string) map[string]any {
	t.Helper()
	if ref != "#/$defs/file" {
		t.Fatalf("unexpected file item ref: %q", ref)
	}
	return objectValue(objectValue(schema, "$defs"), "file")
}

func objectValue(values map[string]any, key string) map[string]any {
	value, ok := values[key].(map[string]any)
	if !ok {
		return nil
	}
	return value
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func stringArrayValue(values map[string]any, key string) map[string]struct{} {
	items, _ := values[key].([]any)
	out := map[string]struct{}{}
	for _, item := range items {
		value, ok := item.(string)
		if ok {
			out[value] = struct{}{}
		}
	}
	return out
}

func propertyNames(properties map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	for name := range properties {
		out[name] = struct{}{}
	}
	return out
}

func assertSameStringSet(t *testing.T, label string, got map[string]struct{}, want map[string]struct{}) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s mismatch\ngot:  %#v\nwant: %#v", label, got, want)
	}
}

func assertFalseValue(t *testing.T, label string, values map[string]any, key string) {
	t.Helper()
	value, ok := values[key].(bool)
	if !ok || value {
		t.Fatalf("%s must be false, got %#v", label, values[key])
	}
}
