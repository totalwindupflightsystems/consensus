// Package jsonschema provides cross-backend JSON Schema validation for Conscience.
//
// It wraps github.com/santhosh-tekuri/jsonschema/v5 to provide:
//   - Compile: validate and cache JSON Schema documents
//   - Validate: check a JSON value against a compiled schema
//   - On Postgres, the pg_jsonschema extension's jsonb_matches_schema()
//     function is used for DB-level CHECK constraints
//   - On SQLite, this Go-level validator is used since modernc.org/sqlite
//     cannot load C extensions like sqlite-jsonschema
//
// The spec (SPEC-003 §4) mandates JSON Schema draft-07 for maximum parity
// between Postgres and SQLite. Both pg_jsonschema and this Go validator
// support draft-07.
//
// axiom:trace work_item=WI-003
//   spec=specs/003-database.md#4,specs/007-json-schema.md
//   plan=phase-2/task-1
//   impl=internal/db/jsonschema/
package jsonschema

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
	_ "github.com/santhosh-tekuri/jsonschema/v5/httploader"
)

// ============================================================================
// Constants
// ============================================================================

// Draft07Schema is the meta-schema URL for JSON Schema draft-07.
const Draft07Schema = "http://json-schema.org/draft-07/schema#"

// ============================================================================
// Validator — Thread-safe schema cache
// ============================================================================

// Validator compiles and validates JSON Schema documents.
// Compiled schemas are cached for performance.
type Validator struct {
	mu       sync.RWMutex
	schemas  map[string]*jsonschema.Schema // keyed by schema document text
}

// New creates a new Validator with an empty cache.
func New() *Validator {
	return &Validator{
		schemas: make(map[string]*jsonschema.Schema),
	}
}

// Compile compiles and caches a JSON Schema document.
// Returns the schema document itself on success (for use in SQL).
// The schema is cached by its exact text for reuse.
func (v *Validator) Compile(schemaJSON string) (*jsonschema.Schema, error) {
	// Check cache first (read lock)
	v.mu.RLock()
	cached, ok := v.schemas[schemaJSON]
	v.mu.RUnlock()
	if ok {
		return cached, nil
	}

	// Compile
	compiled, err := compileSchema(schemaJSON)
	if err != nil {
		return nil, fmt.Errorf("jsonschema: compile: %w", err)
	}

	// Store in cache (write lock)
	v.mu.Lock()
	v.schemas[schemaJSON] = compiled
	v.mu.Unlock()

	return compiled, nil
}

// Validate validates a JSON value against a compiled schema.
// The data parameter must be a Go value that can be JSON-marshaled (map, slice, etc.)
// or a JSON string.
//
// Returns nil if valid, or an error describing all validation failures.
func (v *Validator) Validate(schema *jsonschema.Schema, data any) error {
	var val any

	switch d := data.(type) {
	case string:
		// Parse the JSON string
		if err := json.Unmarshal([]byte(d), &val); err != nil {
			return fmt.Errorf("jsonschema: data is not valid JSON: %w", err)
		}
	default:
		val = d
	}

	if err := schema.Validate(val); err != nil {
		return fmt.Errorf("jsonschema: validation failed: %w", err)
	}

	return nil
}

// ValidateString is a convenience wrapper that validates a JSON string against
// a JSON Schema string. It compiles the schema if not already cached.
func (v *Validator) ValidateString(schemaJSON string, dataJSON string) error {
	schema, err := v.Compile(schemaJSON)
	if err != nil {
		return err
	}
	return v.Validate(schema, dataJSON)
}

// ============================================================================
// Helpers
// ============================================================================

// compileSchema parses and compiles a JSON Schema document.
func compileSchema(schemaJSON string) (*jsonschema.Schema, error) {
	// Parse the schema JSON
	var schemaDoc any
	if err := json.Unmarshal([]byte(schemaJSON), &schemaDoc); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}

	// Add $schema if not present (default to draft-07)
	if doc, ok := schemaDoc.(map[string]any); ok {
		if _, exists := doc["$schema"]; !exists {
			doc["$schema"] = Draft07Schema
		}
	}

	// Marshal back to JSON for the compiler
	canonicalJSON, err := json.Marshal(schemaDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Compile using jsonschema library
	compiled, err := jsonschema.CompileString("schema.json", string(canonicalJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
	}

	return compiled, nil
}

// ============================================================================
// Schema Utility Functions
// ============================================================================

// IsValidSchema checks whether a JSON string is a valid JSON Schema document.
func IsValidSchema(schemaJSON string) error {
	_, err := compileSchema(schemaJSON)
	return err
}

// FormatValidationError converts a jsonschema validation error into a
// human-readable string suitable for error messages and audit logs.
func FormatValidationError(err error) string {
	if err == nil {
		return ""
	}

	var msg string
	switch e := err.(type) {
	case *jsonschema.ValidationError:
		msg = formatValidationError(e)
	default:
		msg = err.Error()
	}

	return msg
}

func formatValidationError(err *jsonschema.ValidationError) string {
	var parts []string
	for _, cause := range err.Causes {
		parts = append(parts, cause.Error())
	}
	if len(parts) == 0 {
		return err.Error()
	}
	return strings.Join(parts, "; ")
}

// DefaultValidators is a package-level default validator instance.
// Use this for simple cases; create a custom Validator for production use.
var DefaultValidators = New()
