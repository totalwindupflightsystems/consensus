package jsonschema_test

import (
	"testing"

	"github.com/wojons/conscientiousness/internal/db/jsonschema"
)

// axiom:trace work_item=WI-003 spec=specs/003-database.md#4 plan=phase-2/task-1 test=internal/db/jsonschema/validator_test.go

func TestValidateValidJSON(t *testing.T) {
	v := jsonschema.New()

	schema := `{
		"type": "object",
		"required": ["name", "age"],
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer", "minimum": 0}
		}
	}`

	data := `{"name": "Alice", "age": 30}`

	if err := v.ValidateString(schema, data); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateInvalidJSON(t *testing.T) {
	v := jsonschema.New()

	schema := `{
		"type": "object",
		"required": ["name", "age"],
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer", "minimum": 0}
		}
	}`

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"missing required field", `{"name": "Alice"}`, true},
		{"wrong type", `{"name": "Alice", "age": "thirty"}`, true},
		{"negative age", `{"name": "Alice", "age": -1}`, true},
		{"extra property", `{"name": "Alice", "age": 30, "extra": true}`, false}, // additionalProperties not restricted
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateString(schema, tt.data)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestValidateAdditionalPropertiesFalse(t *testing.T) {
	v := jsonschema.New()

	schema := `{
		"type": "object",
		"required": ["name"],
		"properties": {
			"name": {"type": "string"}
		},
		"additionalProperties": false
	}`

	// Should fail: extra property not allowed
	err := v.ValidateString(schema, `{"name": "Bob", "extra": true}`)
	if err == nil {
		t.Error("expected error for additionalProperties: false")
	}

	// Should pass: no extra properties
	if err := v.ValidateString(schema, `{"name": "Bob"}`); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateNestedObject(t *testing.T) {
	v := jsonschema.New()

	schema := `{
		"type": "object",
		"required": ["order", "items"],
		"properties": {
			"order": {
				"type": "object",
				"required": ["id", "total"],
				"properties": {
					"id": {"type": "string"},
					"total": {"type": "number", "minimum": 0}
				}
			},
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"required": ["sku", "qty"],
					"properties": {
						"sku": {"type": "string"},
						"qty": {"type": "integer", "minimum": 1}
					}
				},
				"minItems": 1
			}
		}
	}`

	valid := `{
		"order": {"id": "ORD-001", "total": 49.99},
		"items": [{"sku": "ABC-123", "qty": 2}]
	}`

	if err := v.ValidateString(schema, valid); err != nil {
		t.Errorf("expected valid nested, got: %v", err)
	}

	invalid := `{
		"order": {"id": "ORD-001", "total": -5},
		"items": [{"sku": "ABC-123", "qty": 2}]
	}`

	if err := v.ValidateString(schema, invalid); err == nil {
		t.Error("expected error for negative total")
	}
}

func TestValidateArraySchema(t *testing.T) {
	v := jsonschema.New()

	schema := `{
		"type": "array",
		"items": {
			"type": "object",
			"required": ["id"],
			"properties": {
				"id": {"type": "integer"}
			}
		}
	}`

	if err := v.ValidateString(schema, `[{"id": 1}, {"id": 2}]`); err != nil {
		t.Errorf("expected valid array, got: %v", err)
	}

	if err := v.ValidateString(schema, `[{"id": "not-integer"}]`); err == nil {
		t.Error("expected error for wrong type in array")
	}
}

func TestIsValidSchema(t *testing.T) {
	if err := jsonschema.IsValidSchema(`{"type": "object"}`); err != nil {
		t.Errorf("expected valid schema, got: %v", err)
	}

	if err := jsonschema.IsValidSchema(`not json`); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSchemaCacheReuse(t *testing.T) {
	v := jsonschema.New()

	schema := `{"type": "object", "properties": {"x": {"type": "integer"}}}`

	// Compile twice — second should be cached
	_, err := v.Compile(schema)
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}

	_, err = v.Compile(schema)
	if err != nil {
		t.Fatalf("second compile (cached) failed: %v", err)
	}

	// Validate using cached schema
	if err := v.ValidateString(schema, `{"x": 42}`); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestValidateStringPattern(t *testing.T) {
	v := jsonschema.New()

	schema := `{
		"type": "object",
		"required": ["sku"],
		"properties": {
			"sku": {"type": "string", "pattern": "^[A-Z]{3}-\\d{4}$"}
		}
	}`

	if err := v.ValidateString(schema, `{"sku": "ABC-1234"}`); err != nil {
		t.Errorf("expected valid SKU, got: %v", err)
	}

	if err := v.ValidateString(schema, `{"sku": "invalid"}`); err == nil {
		t.Error("expected error for invalid SKU pattern")
	}
}

func TestDefaultValidator(t *testing.T) {
	schema := `{"type": "object", "properties": {"x": {"type": "number"}}}`

	if err := jsonschema.DefaultValidators.ValidateString(schema, `{"x": 3.14}`); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestFormatValidationError(t *testing.T) {
	v := jsonschema.New()

	schema := `{
		"type": "object",
		"required": ["name", "email"],
		"properties": {
			"name": {"type": "string"},
			"email": {"type": "string", "format": "email"}
		}
	}`

	err := v.ValidateString(schema, `{"name": 42}`)
	if err == nil {
		t.Fatal("expected validation error")
	}

	formatted := jsonschema.FormatValidationError(err)
	if formatted == "" {
		t.Error("expected non-empty formatted error")
	}
	t.Logf("Formatted error: %s", formatted)
}
