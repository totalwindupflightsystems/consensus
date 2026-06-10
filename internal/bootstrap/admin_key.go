// Package bootstrap contains one-time instance bootstrap helpers.
//
// axiom:trace work_item=runtime-dev-bootstrap-auth-01 spec=specs/016-cli-interface.md,specs/015-api-and-mcp.md plan=.memory-bank/work-items/runtime-dev-bootstrap-auth-01/plan.md impl=internal/bootstrap/admin_key.go
// axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md plan=.memory-bank/work-items/bootstrap-output-stream-01/plan.md impl=internal/bootstrap/admin_key.go
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 spec=specs/023-adr-bootstrap-key-expiry.md impl=internal/bootstrap/admin_key.go
package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wojons/conscientiousness/internal/db"
)

// DefaultBootstrapKeyTTLHours is the default expiry for first-admin bootstrap keys (90 days).
// Override with CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS env var. Set to 0 to disable expiry.
//
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 impl=internal/bootstrap/admin_key.go
const DefaultBootstrapKeyTTLHours = 2160

var firstAdminKeyMu sync.Mutex

// AdminKeyResult describes the outcome of first-admin-key bootstrapping.
//
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 impl=internal/bootstrap/admin_key.go
type AdminKeyResult struct {
	Created   bool   `json:"created"`
	ID        string `json:"id"`
	APIKey    string `json:"api_key"`
	KeyPrefix string `json:"key_prefix"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"` // RFC 3339 or empty for no expiry
}

// GetBootstrapKeyTTL reads CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS from the environment
// and returns a time.Duration. Returns DefaultBootstrapKeyTTLHours * time.Hour if
// unset or invalid. Returns 0 for value 0 (no expiry, backward compatible).
//
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 impl=internal/bootstrap/admin_key.go
func GetBootstrapKeyTTL() time.Duration {
	v := os.Getenv("CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS")
	if v == "" {
		return DefaultBootstrapKeyTTLHours * time.Hour
	}
	h, err := strconv.Atoi(v)
	if err != nil || h < 0 {
		return DefaultBootstrapKeyTTLHours * time.Hour
	}
	return time.Duration(h) * time.Hour
}

// EnsureFirstAdminKey creates the first admin API key when none exists.
//
// The returned APIKey is only populated when a new key was created. Existing
// keys are never reprinted because the raw secret is not stored.
//
// ttl controls the expires_at value for newly created keys. When ttl > 0, the
// key expires after that duration. When ttl == 0 (zero value), expires_at is
// set to NULL (no expiry), preserving backward compatibility.
//
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-001 impl=internal/bootstrap/admin_key.go
func EnsureFirstAdminKey(ctx context.Context, database db.DB, ttl time.Duration) (AdminKeyResult, error) {
	firstAdminKeyMu.Lock()
	defer firstAdminKeyMu.Unlock()

	rows, err := database.Query(ctx, `SELECT id, key_prefix, created_at FROM api_keys WHERE scope = 'admin' LIMIT 1`)
	if err != nil {
		return AdminKeyResult{}, fmt.Errorf("bootstrap: check existing admin key: %w", err)
	}
	if len(rows) > 0 {
		return AdminKeyResult{
			Created:   false,
			ID:        toString(rows[0]["id"]),
			KeyPrefix: toString(rows[0]["key_prefix"]),
			CreatedAt: formatTime(rows[0]["created_at"]),
		}, nil
	}

	apiKey, err := generateScopedAPIKey("cs_ak_")
	if err != nil {
		return AdminKeyResult{}, err
	}
	keyID, err := newUUID()
	if err != nil {
		return AdminKeyResult{}, err
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	keyPrefix := apiKey[:8]
	keyHash := sha256Hex(apiKey)

	var expiresAtStr string
	if ttl > 0 {
		expiresAt := time.Now().UTC().Add(ttl)
		expiresAtStr = expiresAt.Format(time.RFC3339)
		if err := database.Exec(ctx,
			`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at, expires_at) VALUES ($1, $2, $3, 'admin', $4, $5)`,
			keyID, keyHash, keyPrefix, createdAt, expiresAtStr,
		); err != nil {
			return AdminKeyResult{}, fmt.Errorf("bootstrap: create first admin key: %w", err)
		}
	} else {
		if err := database.Exec(ctx,
			`INSERT INTO api_keys (id, key_hash, key_prefix, scope, created_at) VALUES ($1, $2, $3, 'admin', $4)`,
			keyID, keyHash, keyPrefix, createdAt,
		); err != nil {
			return AdminKeyResult{}, fmt.Errorf("bootstrap: create first admin key: %w", err)
		}
	}

	return AdminKeyResult{
		Created:   true,
		ID:        keyID,
		APIKey:    apiKey,
		KeyPrefix: keyPrefix,
		CreatedAt: createdAt,
		ExpiresAt: expiresAtStr,
	}, nil
}

// FormatResult returns structured key=value output lines for an AdminKeyResult.
//
// Output conforms to SPEC-016 §3 (scriptable, machine-parseable). For a
// newly created key the first line emits "created=true key=<secret>"; for an
// existing key "created=false prefix=<prefix>". Every line is prefixed with
// "conscience:" so operators and scripts can grep for it.
//
// When Created is true and an expiry is set, the machine-parseable first line
// includes expires_at=<RFC 3339> and a human-readable line describes the
// expiry. When TTL=0 (no expiry), a line notes the key does not expire.
//
// axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md impl=internal/bootstrap/admin_key.go
// axiom:trace work_item=bootstrap-admin-key-policy-01 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-002 impl=internal/bootstrap/admin_key.go
func FormatResult(r AdminKeyResult) []string {
	if r.Created {
		firstLine := fmt.Sprintf("conscience: first_admin_key created=true key=%s key_prefix=%s id=%s created_at=%s",
			r.APIKey, r.KeyPrefix, r.ID, r.CreatedAt)
		if r.ExpiresAt != "" {
			firstLine += fmt.Sprintf(" expires_at=%s", r.ExpiresAt)
			expiresTime, err := time.Parse(time.RFC3339, r.ExpiresAt)
			if err == nil {
				remaining := time.Until(expiresTime).Round(time.Minute)
				return []string{
					firstLine,
					fmt.Sprintf("conscience: this key expires at %s (%s from now)", r.ExpiresAt, remaining),
					"conscience: save this key now; it is stored hashed and will not be printed again",
				}
			}
			return []string{
				firstLine,
				fmt.Sprintf("conscience: this key expires at %s", r.ExpiresAt),
				"conscience: save this key now; it is stored hashed and will not be printed again",
			}
		}
		return []string{
			firstLine,
			"conscience: this key does not expire (TTL=0)",
			"conscience: save this key now; it is stored hashed and will not be printed again",
		}
	}
	return []string{
		fmt.Sprintf("conscience: first_admin_key created=false key_prefix=%s id=%s created_at=%s",
			r.KeyPrefix, r.ID, r.CreatedAt),
	}
}

// FormatResultJSON returns the AdminKeyResult as indented JSON bytes.
//
// The secret is redacted when Created is false.
//
// axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md impl=internal/bootstrap/admin_key.go
func FormatResultJSON(r AdminKeyResult) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func generateScopedAPIKey(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("bootstrap: generate api key: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("bootstrap: generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func formatTime(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case time.Time:
		return value.UTC().Format(time.RFC3339)
	default:
		return ""
	}
}
