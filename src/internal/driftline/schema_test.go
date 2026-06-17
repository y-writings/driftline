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
	fileItemSchema := schemaDef(t, schema, stringValue(objectValue(filesSchema, "items"), "$ref"), "file")
	assertFalseValue(t, "file item additionalProperties", fileItemSchema, "additionalProperties")
	fileProperties := propertyNames(objectValue(fileItemSchema, "properties"))
	assertSameStringSet(t, "file properties", fileProperties, allowed["files"])
	assertSameStringSet(t, "file required", stringArrayValue(fileItemSchema, "required"), map[string]struct{}{"id": {}, "paths": {}})
	if _, ok := fileProperties["source_path"]; ok {
		t.Fatal("source manifest schema must not allow old file source_path key")
	}
	if _, ok := fileProperties["target"]; ok {
		t.Fatal("source manifest schema must not allow file target")
	}
	if _, ok := fileProperties["if_not_exists"]; ok {
		t.Fatal("source manifest schema must not allow file if_not_exists")
	}
	pathsSchema := objectValue(objectValue(fileItemSchema, "properties"), "paths")
	if got := numberValue(pathsSchema, "minProperties"); got != 1 {
		t.Fatalf("paths must require at least one item, got %v", got)
	}
	assertDynamicKeyPattern(t, "source path ids", pathsSchema)
	pathItemSchema := schemaDef(t, schema, stringValue(objectValue(pathsSchema, "additionalProperties"), "$ref"), "sourcePath")
	assertFalseValue(t, "source path additionalProperties", pathItemSchema, "additionalProperties")
	assertSameStringSet(t, "source path properties", propertyNames(objectValue(pathItemSchema, "properties")), map[string]struct{}{"name": {}, "path": {}})
	assertSameStringSet(t, "source path required", stringArrayValue(pathItemSchema, "required"), map[string]struct{}{"path": {}})
}

func TestTargetConfigSchemaMatchesParserAllowedKeys(t *testing.T) {
	schema := readTargetConfigSchema(t)
	allowed := allowedTargetConfigKeys()

	if got := stringValue(schema, "$schema"); got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected schema draft: %q", got)
	}
	assertFalseValue(t, "root additionalProperties", schema, "additionalProperties")
	rootProperties := objectValue(schema, "properties")
	assertSameStringSet(t, "root properties", propertyNames(rootProperties), allowed[""])
	assertSameStringSet(t, "root required", stringArrayValue(schema, "required"), map[string]struct{}{"version": {}, "source": {}, "files": {}})

	sourceSchema := schemaDef(t, schema, stringValue(objectValue(rootProperties, "source"), "$ref"), "source")
	assertFalseValue(t, "source additionalProperties", sourceSchema, "additionalProperties")
	assertSameStringSet(t, "source properties", propertyNames(objectValue(sourceSchema, "properties")), allowed["source"])
	assertSameStringSet(t, "source required", stringArrayValue(sourceSchema, "required"), map[string]struct{}{"repository": {}, "ref": {}})

	filesSchema := objectValue(rootProperties, "files")
	fileItemSchema := schemaDef(t, schema, stringValue(objectValue(filesSchema, "items"), "$ref"), "file")
	assertFalseValue(t, "file item additionalProperties", fileItemSchema, "additionalProperties")
	fileProperties := propertyNames(objectValue(fileItemSchema, "properties"))
	assertSameStringSet(t, "file properties", fileProperties, allowed["files"])
	assertSameStringSet(t, "file required", stringArrayValue(fileItemSchema, "required"), map[string]struct{}{"id": {}})
	if _, ok := fileProperties["target_path"]; ok {
		t.Fatal("target config schema must not allow old file target_path key")
	}

	pathOverridesSchema := objectValue(objectValue(fileItemSchema, "properties"), "path_overrides")
	if got := numberValue(pathOverridesSchema, "minProperties"); got != 1 {
		t.Fatalf("path_overrides must require at least one item, got %v", got)
	}
	assertDynamicKeyPattern(t, "path override ids", pathOverridesSchema)
	if stringValue(objectValue(pathOverridesSchema, "additionalProperties"), "$ref") != "#/$defs/relativePath" {
		t.Fatalf("path_overrides must allow file id keys with target path string values: %#v", pathOverridesSchema)
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
	return readSchema(t, "../../../schema.json")
}

func readTargetConfigSchema(t *testing.T) map[string]any {
	t.Helper()
	return readSchema(t, "../../../target-schema.json")
}

func readSchema(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return schema
}

func schemaDef(t *testing.T, schema map[string]any, ref string, name string) map[string]any {
	t.Helper()
	want := "#/$defs/" + name
	if ref != want {
		t.Fatalf("unexpected schema ref: got %q, want %q", ref, want)
	}
	return objectValue(objectValue(schema, "$defs"), name)
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

func numberValue(values map[string]any, key string) float64 {
	value, _ := values[key].(float64)
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

func assertDynamicKeyPattern(t *testing.T, label string, schema map[string]any) {
	t.Helper()
	propertyNames := objectValue(schema, "propertyNames")
	if stringValue(propertyNames, "type") != "string" || stringValue(propertyNames, "pattern") != "\\S" {
		t.Fatalf("%s must require non-blank keys, got %#v", label, propertyNames)
	}
}
