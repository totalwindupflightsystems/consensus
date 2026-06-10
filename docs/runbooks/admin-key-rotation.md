# Admin Key Rotation Runbook

**Purpose**: Rotate the bootstrap admin key for the Conscience server.
**Severity**: High (security-sensitive)
**Estimated Time**: 5 minutes

---

## Overview

Conscience uses API keys for authentication (SPEC-015 §2). The bootstrap admin key is created on first startup with a configurable TTL (default: 90 days). When it expires or needs rotation, use this runbook to generate a new one.

Admin keys have full CRUD access to all endpoints. Key rotation should be performed:
- **Routinely**: Before the current key expires
- **Emergency**: When a key is compromised

---

## Check Current Key Status

```bash
# List API keys (admin only)
curl -s http://localhost:8090/api/v1/auth/keys \
  -H "Authorization: Bearer $CURRENT_ADMIN_KEY" | jq .

# Expected response snippet:
# {
#   "keys": [
#     {
#       "key_prefix": "cs_ak_a1b2",
#       "scope": "admin",
#       "created_at": "2026-03-01T00:00:00Z",
#       "expires_at": "2026-05-30T00:00:00Z"
#     }
#   ]
# }

# Check expiry
curl -s http://localhost:8090/api/v1/auth/keys \
  -H "Authorization: Bearer $CURRENT_ADMIN_KEY" | jq -r '.keys[] | select(.scope=="admin") | "Expires: \(.expires_at)"'
```

---

## Routine Rotation

### Step 1: Generate New Admin Key

```bash
# If server is running, use the API
curl -s -X POST http://localhost:8090/api/v1/auth/keys \
  -H "Authorization: Bearer $CURRENT_ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"scope":"admin","ttl_hours":2160}' | jq .

# Output will contain the full key — save it immediately:
# {"id":"...","key":"cs_ak_<full_key>","scope":"admin","expires_at":"2026-08-27T00:00:00Z"}
```

### Step 2: Verify New Key Works

```bash
# Test the new key
curl -s http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer cs_ak_<new_key>" | jq .

# Expected: {"sessions":[],"total":0} (or session list)
```

### Step 3: Update All Clients Using the Old Key

- CI/CD pipelines (GitHub Actions secrets)
- Monitoring systems
- Scripts and automation tools
- Developer `.env` files

### Step 4: Revoke Old Key

```bash
# Get the old key ID
OLD_KEY_ID=$(curl -s http://localhost:8090/api/v1/auth/keys \
  -H "Authorization: Bearer $NEW_ADMIN_KEY" | \
  jq -r '.keys[] | select(.key_prefix == "cs_ak_a1b2") | .id')

# Delete the old key
curl -s -X DELETE "http://localhost:8090/api/v1/auth/keys/$OLD_KEY_ID" \
  -H "Authorization: Bearer $NEW_ADMIN_KEY"

# Verify the old key is revoked
curl -s http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer cs_ak_<old_key>"
# Expected: 401 Unauthorized
```

---

## Emergency Rotation (Compromised Key)

```bash
# 1. Immediately delete the compromised key
curl -X DELETE "http://localhost:8090/api/v1/auth/keys/$OLD_KEY_ID" \
  -H "Authorization: Bearer $NON_COMPROMISED_KEY"

# 2. If no non-compromised key exists, regenerate via init:
./bin/conscience init --db-url postgres://user:pass@host:5432/conscience
# The new key is printed to stdout

# 3. Audit all recent activity
curl -s http://localhost:8090/api/v1/metrics \
  -H "Authorization: Bearer $NEW_KEY"
```

---

## Bootstrap Key Special Notes

The bootstrap admin key (created by `EnsureFirstAdminKey` at startup) has a configurable TTL:

```bash
# Set TTL to 7 days (168 hours)
CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS=168 ./bin/conscience serve

# Set TTL to 0 (never expires — not recommended for production)
CONSCIENCE_BOOTSTRAP_KEY_TTL_HOURS=0 ./bin/conscience serve
```

Default TTL is **2160 hours (90 days)**. After the TTL expires, the key is still valid (SQL does not auto-delete expired keys). Expired keys are rejected by the auth middleware.

---

## Verification Checklist

- [ ] New key created and tested
- [ ] Old key revoked
- [ ] All clients updated with new key
- [ ] New key saved to secrets manager
- [ ] Audit log shows key change activity

---

> **Trace**: `axiom:trace work_item=WI-019 spec=specs/015-api-and-mcp.md#req-bootstrap-ttl-002,specs/023-adr-bootstrap-key-expiry.md doc=docs/runbooks/admin-key-rotation.md`
