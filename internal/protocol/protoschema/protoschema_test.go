package protoschema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/get-vix/vix/internal/protocol"
)

// committedSchemaPath is the golden file relative to this package directory.
const committedSchemaPath = "../schema/vix-protocol.schema.json"

// TestSchemaNotStale fails when the committed schema drifts from the generator
// output — the drift gate that keeps downstream (Swift) clients honest. Run
// `make proto-schema` and commit the result to fix.
func TestSchemaNotStale(t *testing.T) {
	got, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want, err := os.ReadFile(committedSchemaPath)
	if err != nil {
		t.Fatalf("read committed schema (%s): %v — run `make proto-schema`", committedSchemaPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("committed schema is stale (%s differs from generator output).\n"+
			"Run `make proto-schema` and commit the regenerated file.", filepath.Clean(committedSchemaPath))
	}
}

// committedSwiftPath is the generated Swift models file, relative to this
// package directory (internal/protocol/protoschema → repo root → apps/…).
const committedSwiftPath = "../../../apps/vix-mac/Sources/VixProtocol/Generated.swift"

// TestSwiftNotStale keeps the committed Swift models in lockstep with the Go
// structs: it fails when apps/vix-mac/.../Generated.swift drifts from the Swift
// emitter output. This runs in the normal `go test` loop, so a protocol change
// that forgets `make mac-models` is caught without a separate Swift CI job. Run
// `make mac-models` and commit to fix.
func TestSwiftNotStale(t *testing.T) {
	got, err := GenerateSwift()
	if err != nil {
		t.Fatalf("GenerateSwift: %v", err)
	}
	want, err := os.ReadFile(committedSwiftPath)
	if err != nil {
		t.Fatalf("read committed Swift models (%s): %v — run `make mac-models`", committedSwiftPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("committed Swift models are stale (%s differs from generator output).\n"+
			"Run `make mac-models` and commit the regenerated file.", filepath.Clean(committedSwiftPath))
	}
}

// TestRoundTrip marshals a zero value of every registered payload and validates
// it against that type's generated $def, proving the schema actually matches the
// JSON the Go structs emit (no missing property, no type mismatch).
func TestRoundTrip(t *testing.T) {
	b, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal generated schema: %v", err)
	}
	defs, _ := doc["$defs"].(map[string]any)
	if defs == nil {
		t.Fatal("generated schema has no $defs")
	}

	check := func(kind string, reg map[string]any) {
		for disc, zero := range reg {
			if zero == nil {
				continue
			}
			t.Run(kind+"/"+disc, func(t *testing.T) {
				raw, err := json.Marshal(zero)
				if err != nil {
					t.Fatalf("marshal zero value: %v", err)
				}
				var inst any
				if err := json.Unmarshal(raw, &inst); err != nil {
					t.Fatalf("unmarshal instance: %v", err)
				}
				name := reflect.TypeOf(zero).Name()
				sch, ok := defs[name].(map[string]any)
				if !ok {
					t.Fatalf("no $def for payload type %q", name)
				}
				if err := validate(inst, sch, defs, name); err != nil {
					t.Fatalf("%s does not validate against its schema: %v", name, err)
				}
			})
		}
	}
	check("event", protocol.EventTypes)
	check("command", protocol.CommandTypes)
}

// validate is a minimal JSON-Schema checker covering exactly the constructs this
// package emits: $ref, object (properties/required/additionalProperties), array
// (items), and the scalar types. null satisfies any schema (fields are treated
// as nullable), which is sufficient for the round-trip smoke check — strictness
// comes from TestSchemaNotStale.
func validate(inst any, sch, defs map[string]any, path string) error {
	if ref, ok := sch["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		d, ok := defs[name].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: dangling $ref %q", path, ref)
		}
		return validate(inst, d, defs, path)
	}
	if inst == nil {
		return nil
	}
	switch typ, _ := sch["type"].(string); typ {
	case "object":
		m, ok := inst.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object, got %T", path, inst)
		}
		if req, ok := sch["required"].([]any); ok {
			for _, r := range req {
				key, _ := r.(string)
				if _, present := m[key]; !present {
					return fmt.Errorf("%s: missing required property %q", path, key)
				}
			}
		}
		props, _ := sch["properties"].(map[string]any)
		ap, _ := sch["additionalProperties"].(map[string]any)
		for k, v := range m {
			if ps, ok := props[k].(map[string]any); ok {
				if err := validate(v, ps, defs, path+"."+k); err != nil {
					return err
				}
			} else if ap != nil {
				if err := validate(v, ap, defs, path+"."+k); err != nil {
					return err
				}
			}
		}
	case "array":
		arr, ok := inst.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array, got %T", path, inst)
		}
		if items, ok := sch["items"].(map[string]any); ok {
			for i, el := range arr {
				if err := validate(el, items, defs, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case "string":
		if _, ok := inst.(string); !ok {
			return fmt.Errorf("%s: expected string, got %T", path, inst)
		}
	case "integer", "number":
		if _, ok := inst.(float64); !ok {
			return fmt.Errorf("%s: expected number, got %T", path, inst)
		}
	case "boolean":
		if _, ok := inst.(bool); !ok {
			return fmt.Errorf("%s: expected boolean, got %T", path, inst)
		}
	case "":
		// No type constraint (any / json.RawMessage / free-form object values).
	}
	return nil
}
