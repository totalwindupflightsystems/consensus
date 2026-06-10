// Package secrets implements secret injection and scrubbing for the Conscience
// harness (SPEC-008 §Secrets Injection & Scrubbing, SPEC-005).
//
// Secrets are sensitive values (API keys, tokens, credentials) that the agent
// can reference via aliases like {{SECRET.LLM_API_KEY}} but never sees directly.
// The injection phase replaces aliases with real values before SQL execution.
// The scrubbing phase removes real values from LLM responses before storage.
//
// Security properties:
//   - Aliases are case-sensitive and namespaced ({{SECRET.X}})
//   - Injection uses exact string replacement
//   - Scrubbing uses exact match against known secret values
//   - Scrubbing runs on every response before storage or display
//   - Secrets are never logged, even at debug level
//
// axiom:trace work_item=runtime-harness-01 spec=specs/008-harness.md,specs/012-system-prompt-and-discovery.md plan=phase-1/task-1-1/step-1-1-4 impl=internal/secrets/secrets.go
package secrets

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// Types
// ============================================================================

// Store holds named secret values for injection and scrubbing.
// The key is the secret alias (e.g., "LLM_API_KEY"), the value is the actual secret.
// Store does not log or persist secrets.
type Store struct {
	values map[string]string
}

// New creates a new empty secret store.
func New() *Store {
	return &Store{
		values: make(map[string]string),
	}
}

// NewFromMap creates a store initialized from an existing map.
func NewFromMap(initial map[string]string) *Store {
	s := New()
	for k, v := range initial {
		s.Set(k, v)
	}
	return s
}

// Set stores a secret under the given alias.
func (s *Store) Set(alias, value string) {
	s.values[alias] = value
}

// Get retrieves a secret by alias. Returns empty string if not found.
func (s *Store) Get(alias string) string {
	return s.values[alias]
}

// Aliases returns the list of all registered secret aliases.
func (s *Store) Aliases() []string {
	result := make([]string, 0, len(s.values))
	for k := range s.values {
		result = append(result, k)
	}
	return result
}

// Count returns the number of stored secrets.
func (s *Store) Count() int {
	return len(s.values)
}

// ============================================================================
// Injection — Replace aliases with real values
// ============================================================================

var aliasPattern = regexp.MustCompile(`\{\{SECRET\.([A-Za-z0-9_]+)\}\}`)

// Inject replaces all {{SECRET.X}} aliases in the text with their real values.
//
// Example: Inject("{{SECRET.API_KEY}}", store) → "sk-abc123"
//
// If a referenced alias doesn't exist in the store, the alias is left unchanged
// and an error is returned along with the partially-injected text.
func (s *Store) Inject(text string) (string, error) {
	var missing []string

	result := aliasPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract alias name from {{SECRET.NAME}}
		alias := match[9 : len(match)-2] // strip {{SECRET. and }}
		value, ok := s.values[alias]
		if !ok {
			missing = append(missing, alias)
			return match // leave alias unchanged
		}
		return value
	})

	if len(missing) > 0 {
		return result, fmt.Errorf("unknown secrets: %s", strings.Join(missing, ", "))
	}

	return result, nil
}

// InjectSQL replaces secret aliases in a SQL statement.
// Convenience method; identical to Inject().
func (s *Store) InjectSQL(sql string) (string, error) {
	return s.Inject(sql)
}

// ============================================================================
// Scrubbing — Remove real values from text
// ============================================================================

// Scrub removes all known secret values from the text, replacing them with
// [REDACTED:alias] markers.
//
// Example: Scrub("Using key sk-abc123", store) → "Using key [REDACTED:API_KEY]"
//
// Scrubbing uses exact string matching. If a secret value happens to appear
// in benign content, that content will also be redacted.
func (s *Store) Scrub(text string) string {
	result := text
	for alias, value := range s.values {
		if value == "" {
			continue
		}
		result = strings.ReplaceAll(result, value, fmt.Sprintf("[REDACTED:%s]", alias))
	}
	return result
}

// ScrubResponse scrubs a complete LLM response (including internal_monologue).
// Convenience method; identical to Scrub().
func (s *Store) ScrubResponse(response string) string {
	return s.Scrub(response)
}

// ============================================================================
// Alias Validation
// ============================================================================

var validAliasRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// IsValidAlias returns true if the alias name is valid for use with Inject/Scrub.
func IsValidAlias(alias string) bool {
	return validAliasRe.MatchString(alias)
}
