---
description: Reverse-engineer reimplementation-grade specs from an existing codebase (code-first to spec-first).
agent: tower-axiom
---

Deeply understand an existing codebase and produce forward-moving, reimplementation-grade
specs under `specs/`.

**Reimplementation-grade** means: the extracted specs are detailed enough that multiple
independent LLMs (or human teams) could reimplement the entire system from specs alone,
in any programming language, with no questions left unanswered. If a spec leaves a
behavior ambiguous, that is a spec defect — not an acceptable gap.

This is the "code already exists, specs don't" entry point. It works like a senior
engineer joining a new team: explore broadly, form hypotheses, drill into what matters,
build a mental model, then write specs that are contracts for the future — not just
documentation of the status quo.

Use it when:
- Onboarding an existing repo into Axiom for the first time
- A GitHub App connects a repo that has code but no `specs/`
- You need to understand what a codebase does before planning changes
- Converting a code-first project to spec-first governance
- Preparing a codebase for multi-LLM reimplementation or language migration
- Validating that existing specs are complete enough for independent reimplementation

## Skills (load on demand)

- `spec-kickoff-axiom` — for tier definitions when setting `$TARGET_TIER`
- `axiom-xml-protocol` — for output envelope format

Inputs
- `$ARGUMENTS` optional: focus areas, constraints, or scope filters (e.g. `scope=src/api/** focus=api,data_model skip=vendor/`)
- `$WORK_ITEM_ID` optional: default `spec-extract-01`
- `$TARGET_TIER` optional: maturity tier for extracted specs (default: `mvp`). See `spec-kickoff-axiom` skill for tier definitions.
- `$SCOPE_FILTER` optional: glob patterns to limit analysis (e.g. `src/**,lib/**`). Empty = analyze everything.
- `$EXCLUDE_PATHS` optional: glob patterns to skip (e.g. `vendor/**,node_modules/**,dist/**`).
- `$REIMPL_DEPTH` optional: `standard` (default) | `full` | `surgical`. Controls how exhaustive the extraction is:
  - `standard`: produce specs sufficient for a competent team to reimplement with occasional clarification questions
  - `full`: produce specs sufficient for zero-question reimplementation in any language
  - `surgical`: produce specs for a specific subsystem only (requires `$SCOPE_FILTER`)

---

## The Reimplementation-Grade Standard

A spec is reimplementation-grade when an independent team (human or LLM) can:

1. **Build the system from scratch** using only the specs, in any programming language,
   without reading the original source code.
2. **Pass all behavioral tests** defined in the specs without reverse-engineering the
   original implementation.
3. **Produce wire-compatible outputs** — same HTTP responses, same CLI output formats,
   same file formats, same error codes — for all documented inputs.
4. **Handle all error cases** correctly because every error path is explicitly specified
   with trigger conditions, expected behavior, and user-visible output.
5. **Configure the system identically** because every configuration surface is documented
   with types, defaults, validation rules, and behavioral effects.

### What "reimplementation-grade" requires that normal specs don't

| Dimension | Normal spec | Reimplementation-grade spec |
|-----------|------------|----------------------------|
| API endpoints | "POST /users creates a user" | Exact request schema (every field, type, constraint, optionality), exact response schema per status code, exact headers, exact error codes with trigger conditions |
| Data model | "Users have names and emails" | Full schema: field name, type, constraints (length, regex, uniqueness), nullability, default, cardinality, foreign keys, indexes, cascade rules |
| State machines | "Orders go from pending to shipped" | Every state, every transition, trigger conditions, guard conditions, side effects per transition, invalid transition behavior |
| Error handling | "Returns 400 on bad input" | Error taxonomy: every error code, HTTP status, message template, trigger condition, retry semantics, user-visible vs internal distinction |
| Configuration | "Configurable via env vars" | Every config key: name, type, default, validation rule, behavioral effect, required vs optional, precedence when multiple sources exist |
| Algorithms | "Scores are calculated" | Exact formula or pseudocode, input ranges, output ranges, edge cases (division by zero, overflow, empty input), precision requirements |
| Concurrency | "Handles multiple requests" | Thread safety model, locking strategy, race condition mitigations, ordering guarantees, idempotency contracts |
| Wire formats | "Returns JSON" | Exact JSON structure with field ordering expectations, date format (ISO 8601 with timezone?), number precision, null vs absent field semantics, pagination envelope |
| Dependencies | "Uses PostgreSQL" | Required version range, required extensions, connection pooling expectations, transaction isolation level, migration strategy |
| Security | "Requires authentication" | Auth protocol (JWT/OAuth/API key), token format, validation rules, expiry handling, refresh flow, permission model (RBAC/ABAC), every protected resource mapped to required permissions |
| Performance | "Should be fast" | Concrete SLOs: p50/p95/p99 latency targets, throughput targets, resource limits, timeout values, backpressure behavior |
| Observability | "Has logging" | Log format (structured JSON?), log levels per category, metric names and types, trace span hierarchy, health check contract |

---

## How to think (dynamic, not a flat checklist)

This command adapts to the codebase it encounters. You maintain an evolving internal
situation model as you explore:

```yaml
situation_model:
  # Discovered during exploration — not asked upfront
  primary_language: null     # detected from code
  secondary_languages: []    # other languages present
  framework_family: null     # web/cli/library/data-pipeline/mobile/infra/mixed
  architecture_style: null   # monolith/microservices/serverless/modular-monolith/scripts
  data_complexity: null      # none/simple-crud/relational/event-sourced/graph/mixed
  api_surface: null          # none/rest/graphql/grpc/websocket/sse/cli/library-exports/mixed
  api_versioning: null       # none/url-path/header/query-param
  auth_model: null           # none/session/jwt/oauth/api-key/mtls/mixed
  data_classification: null  # none/internal/PII/secrets/financial/regulated
  trust_boundaries: []       # list of identified trust boundaries
  test_maturity: null        # none/smoke/unit/integration/e2e/property/mixed
  ops_maturity: null         # none/logs-only/metrics/alerts/runbooks/slos
  config_complexity: null    # hardcoded/env-vars/config-files/feature-flags/remote-config
  config_sources: []         # list of config source types in precedence order
  deployment_model: null     # unknown/container/serverless/bare-metal/static/hybrid
  team_signals: null         # solo/small-team/large-team (inferred from git/PR patterns)
  concurrency_model: null    # single-threaded/multi-threaded/async/actor/event-loop
  persistence_engines: []    # list of storage systems (postgres, redis, s3, filesystem, etc.)
  message_systems: []        # list of message/event systems (kafka, nats, rabbitmq, sse, etc.)
  external_dependencies: []  # list of external services/APIs the system calls
  state_machines: []         # list of identified state machines with states/transitions
  error_taxonomy_exists: null # whether the codebase has a structured error type system
  wire_format_conventions: null # json/protobuf/msgpack/xml/custom
```

The situation model drives which analysis passes to run, how deep to go, and which
spec categories to produce. You do NOT run every analysis on every repo — you
adapt based on what you discover.

Heuristic: spend 70% of analysis time on the 2-3 areas that matter most for THIS
codebase, not equal time on everything.

---

## Do (fail-closed, iterative deepening)

### Phase 0 — Preflight

1. Verify the repo has code to analyze (fail if repo is empty or has only config files).
2. Check if `specs/` already exists. If it does and has content:
   - Ask the user: "Specs already exist. Do you want to (A) augment existing specs with discovered gaps, (B) regenerate all specs from code, or (C) abort?"
   - Default: A (augment).
3. Ensure `.opencode/` exists (Axiom must be installed).
4. Create `specs/` if missing.
5. Create `specs/README.md` stub if missing.
6. Detect `$REIMPL_DEPTH` and adjust downstream analysis accordingly.

### Phase 1 — Broad Reconnaissance (fast, wide, shallow)

Goal: form a first-pass mental model in minutes, not hours. This is the "walk the
hallways and read the whiteboards" phase.

Delegate to `@repo-researcher-axiom` with mode `reverse_engineer`:

1. **Structural scan** (seconds):
   - File tree shape: top-level dirs, depth, naming conventions
   - Language detection: primary + secondary languages, LOC distribution
   - Dependency manifests: package.json, requirements.txt, go.mod, Cargo.toml, etc.
   - Build system: Makefile, Dockerfile, CI configs, scripts/
   - Entry points: main files, server starts, CLI entry, handler registrations
   - Module/package boundaries: what imports what, dependency direction

2. **Signal harvesting** (fast pattern matching, not deep reading):
   - README/docs: what does the project SAY it does?
   - Git history shape: commit frequency, contributor count, recent activity
   - Test presence: test directories, test file count, test framework config
   - Config surfaces: .env files, config schemas, feature flag references
   - Error patterns: error types, error handling conventions, error code registries
   - Logging/metrics: observability instrumentation presence
   - Migration files: database schema evolution history
   - OpenAPI/AsyncAPI/protobuf: existing machine-readable contracts
   - CI/CD configs: what gets tested, what gets deployed, what environments exist

3. **Hypothesis formation**:
   - Update the situation model with initial values
   - Identify the 2-3 "interesting seams" — the places where the most important
     behavior lives (e.g., "the API layer is the main surface", "the data pipeline
     is the core value", "the auth system is the critical boundary")
   - Rank analysis priorities: what to drill into next

Output: a Reconnaissance Report with the situation model, hypotheses, and a
prioritized drill-down plan. This is NOT the final output — it's the map for Phase 2.

### Phase 2 — Deep Dives (targeted, iterative)

Goal: understand the actual behavior at reimplementation-grade depth. This is the
"read the code, trace the flows, extract every contract" phase.

Run deep dives IN PRIORITY ORDER based on Phase 1 hypotheses. Each deep dive
produces understanding that may reprioritize remaining dives.

Every deep dive MUST produce structured extraction artifacts, not just prose summaries.
The artifacts feed directly into Phase 4 spec generation.

---

#### Deep Dive: Architecture & Module Boundaries
(Always run — but depth varies by architecture_style)

**Structural extraction:**
- Trace the dependency graph between modules/packages — produce a directed graph
- Identify the "spine" — the core flow that everything hangs off of
- Map module responsibilities and boundaries
- Find the "load-bearing walls" — code that everything depends on
- Identify extension points and plugin patterns
- Note architectural decisions (even if undocumented) — these become ADR candidates

**Interface contract extraction** (for every module boundary):
```yaml
module_contract:
  name: "<module name>"
  purpose: "<one sentence>"
  public_interface:
    - name: "<function/class/export name>"
      signature: "<full type signature>"
      parameters:
        - name: "<param>"
          type: "<type>"
          required: true|false
          default: "<value or null>"
          constraints: "<validation rules>"
      returns:
        type: "<return type>"
        nullable: true|false
        error_cases:
          - condition: "<when this error occurs>"
            error_type: "<exception/error type>"
            message_pattern: "<message template>"
      side_effects: ["<list of side effects>"]
      idempotent: true|false
      thread_safe: true|false
  dependencies:
    - module: "<dependency module>"
      interface_used: ["<which functions/classes>"]
      required: true|false
  invariants:
    - "<what must always be true about this module's state>"
```

---

#### Deep Dive: Public API Surface
(Run if api_surface != none)

**Endpoint catalog** — for EVERY endpoint, extract:
```yaml
endpoint:
  path: "/api/v1/resource"
  method: "POST"
  summary: "<one sentence>"
  authentication:
    required: true|false
    schemes: ["bearer", "api-key"]
    permissions: ["resource:create"]
  rate_limiting:
    enabled: true|false
    limits: "<requests per window>"
  request:
    content_type: "application/json"
    headers:
      required:
        - name: "Authorization"
          format: "Bearer <token>"
        - name: "Content-Type"
          value: "application/json"
      optional:
        - name: "X-Request-ID"
          format: "UUID v4"
          default_behavior: "server generates one"
        - name: "X-Idempotency-Key"
          format: "string, max 128 chars"
    body_schema:
      type: "object"
      required_fields:
        - name: "field_name"
          type: "string"
          constraints:
            min_length: 1
            max_length: 255
            pattern: "^[a-zA-Z0-9_-]+$"
          description: "<what this field means>"
      optional_fields:
        - name: "optional_field"
          type: "integer"
          default: 0
          constraints:
            minimum: 0
            maximum: 1000
          description: "<what this field means>"
    query_parameters: []
    path_parameters:
      - name: "id"
        type: "string"
        format: "UUID v4"
        constraints: "must exist in database"
  responses:
    "201":
      description: "Resource created successfully"
      headers:
        - name: "Location"
          value: "/api/v1/resource/{id}"
      body_schema:
        type: "object"
        fields:
          - name: "id"
            type: "string"
            format: "UUID v4"
          - name: "created_at"
            type: "string"
            format: "ISO 8601 with timezone (e.g., 2026-03-24T10:00:00Z)"
          # ... every field
    "400":
      description: "Validation error"
      body_schema:
        type: "object"
        fields:
          - name: "error"
            type: "object"
            fields:
              - name: "code"
                type: "string"
                enum: ["invalid_request", "missing_field", "invalid_format"]
              - name: "message"
                type: "string"
              - name: "details"
                type: "array"
                items:
                  type: "object"
                  fields:
                    - name: "field"
                      type: "string"
                    - name: "reason"
                      type: "string"
    "401":
      description: "Authentication required"
      trigger: "Missing or invalid Authorization header"
      body_schema: { error: { code: "unauthorized", message: "string" } }
    "403":
      description: "Insufficient permissions"
      trigger: "Valid auth but missing required permission"
      body_schema: { error: { code: "forbidden", message: "string" } }
    "404":
      description: "Resource not found"
      trigger: "ID does not exist in database"
      body_schema: { error: { code: "not_found", message: "string" } }
    "409":
      description: "Conflict"
      trigger: "Duplicate idempotency key with different payload"
      body_schema: { error: { code: "conflict", message: "string" } }
    "429":
      description: "Rate limited"
      trigger: "Exceeded rate limit"
      headers:
        - name: "Retry-After"
          value: "<seconds>"
      body_schema: { error: { code: "rate_limited", message: "string" } }
    "500":
      description: "Internal server error"
      trigger: "Unhandled exception"
      body_schema: { error: { code: "internal_error", message: "string" } }
  middleware_chain:
    - "request_id_injection"
    - "authentication"
    - "authorization"
    - "rate_limiting"
    - "input_validation"
    - "handler"
    - "response_serialization"
    - "error_handling"
    - "access_logging"
  idempotency:
    supported: true|false
    mechanism: "idempotency key header"
    window: "24 hours"
  pagination:
    style: "cursor|offset|page"
    default_page_size: 20
    max_page_size: 100
    cursor_field: "next_cursor"
```

**Request lifecycle trace** — for the primary API surface:
- Document the exact middleware/interceptor chain in execution order
- For each middleware: what it does, what it can reject, what headers/context it adds
- Document request validation: what is validated, in what order, what errors are returned
- Document response serialization: how objects become JSON, date formatting, null handling

**API conventions extraction:**
- Naming conventions (camelCase vs snake_case in JSON)
- Date/time format (ISO 8601? Unix timestamp? With timezone?)
- Null vs absent field semantics (is `"field": null` different from field being absent?)
- Pagination envelope structure
- Error response envelope structure
- Versioning strategy (URL path? Header? Query param?)
- CORS configuration
- Content negotiation rules

---

#### Deep Dive: Data Model & State
(Run if data_complexity != none)

**Full data dictionary** — for EVERY persistent entity:
```yaml
entity:
  name: "User"
  table_name: "users"  # if SQL
  collection_name: "users"  # if NoSQL
  description: "<what this entity represents>"
  fields:
    - name: "id"
      type: "UUID"
      primary_key: true
      generated: true
      generation_strategy: "UUID v4 at insert time"
    - name: "email"
      type: "VARCHAR(255)"
      nullable: false
      unique: true
      constraints:
        format: "RFC 5322 email"
        max_length: 255
        case_sensitivity: "stored lowercase, compared case-insensitive"
      indexed: true
      index_type: "btree unique"
    - name: "created_at"
      type: "TIMESTAMP WITH TIME ZONE"
      nullable: false
      default: "NOW()"
      immutable: true  # never updated after creation
    - name: "status"
      type: "ENUM('active', 'suspended', 'deleted')"
      nullable: false
      default: "'active'"
      state_machine: "user_lifecycle"  # reference to state machine definition
  relationships:
    - type: "has_many"
      target: "Order"
      foreign_key: "user_id"
      cascade_delete: false
      cascade_behavior: "SET NULL on orders.user_id"
    - type: "has_one"
      target: "UserProfile"
      foreign_key: "user_id"
      cascade_delete: true
  indexes:
    - name: "idx_users_email"
      columns: ["email"]
      type: "btree"
      unique: true
    - name: "idx_users_status_created"
      columns: ["status", "created_at"]
      type: "btree"
      unique: false
      purpose: "efficient filtering of active users by creation date"
  constraints:
    - type: "check"
      expression: "email ~* '^[^@]+@[^@]+\\.[^@]+$'"
    - type: "check"
      expression: "created_at <= updated_at"
  soft_delete:
    enabled: true|false
    mechanism: "status='deleted' + deleted_at timestamp"
  audit:
    created_at: true
    updated_at: true
    created_by: false
    updated_by: false
```

**State machine definitions** — for EVERY entity with lifecycle states:
```yaml
state_machine:
  name: "order_lifecycle"
  entity: "Order"
  initial_state: "pending"
  terminal_states: ["completed", "cancelled", "refunded"]
  states:
    - name: "pending"
      description: "Order created, awaiting payment"
      allowed_transitions:
        - to: "paid"
          trigger: "payment_confirmed"
          guard: "payment_amount == order_total"
          side_effects:
            - "send_confirmation_email"
            - "reserve_inventory"
          rollback_on_failure: "remain in pending"
        - to: "cancelled"
          trigger: "user_cancel OR payment_timeout(30min)"
          guard: "none"
          side_effects:
            - "release_inventory_hold"
            - "send_cancellation_email"
    - name: "paid"
      description: "Payment confirmed, awaiting fulfillment"
      allowed_transitions:
        - to: "shipped"
          trigger: "shipment_created"
          guard: "tracking_number is not null"
          side_effects:
            - "send_shipping_notification"
            - "deduct_inventory"
        - to: "refunded"
          trigger: "admin_refund OR dispute_won"
          guard: "refund_amount <= paid_amount"
          side_effects:
            - "initiate_payment_refund"
            - "release_inventory"
            - "send_refund_notification"
  invalid_transition_behavior:
    action: "reject with error"
    error_code: "invalid_state_transition"
    http_status: 409
    message_template: "Cannot transition {entity} from {current_state} to {target_state}"
```

**Data flow mapping:**
- Where data enters the system (API endpoints, file uploads, message queues, scheduled jobs)
- How data transforms between entry and storage (validation, normalization, enrichment)
- Where data exits the system (API responses, exports, notifications, event streams)
- Data invariants: what MUST be true across related entities (referential integrity, business rules)

**Migration strategy extraction:**
- How schema changes are applied (migration framework, raw SQL, ORM auto-migrate)
- Migration ordering and dependency rules
- Rollback capability per migration type
- Data backfill patterns used

---

#### Deep Dive: Configuration & Environment
(Run if config_complexity != hardcoded)

**Complete configuration catalog** — for EVERY config surface:
```yaml
config_entry:
  key: "DATABASE_URL"
  source: "environment variable"
  type: "string (PostgreSQL connection URI)"
  format: "postgresql://user:pass@host:port/dbname?sslmode=require"
  required: true
  default: null
  validation:
    pattern: "^postgresql://.*"
    must_be_reachable: true  # validated at startup
  behavioral_effect: "Determines which PostgreSQL instance the application connects to"
  sensitive: true  # contains credentials
  precedence: 3  # env var overrides config file which overrides default
  affects_components: ["database_pool", "migration_runner"]
  restart_required: true  # changing this requires restart, not hot-reload
  
config_entry:
  key: "LOG_LEVEL"
  source: "environment variable"
  type: "enum"
  enum_values: ["debug", "info", "warn", "error"]
  required: false
  default: "info"
  validation:
    case_sensitive: false  # "INFO" and "info" both accepted
  behavioral_effect: "Controls minimum severity of log output"
  sensitive: false
  precedence: 3
  affects_components: ["logger"]
  restart_required: false  # hot-reloadable
```

**Configuration precedence rules:**
```yaml
config_precedence:
  # Highest priority first
  1: "CLI flags / command-line arguments"
  2: "Environment variables"
  3: "Config file (e.g., config.yaml, .env)"
  4: "Hardcoded defaults in source code"
  merge_strategy: "last-writer-wins per key (no deep merge)"
  conflict_resolution: "higher precedence source wins silently"
  unknown_keys: "ignored with warning log"
```

**Environment matrix:**
- What differs between dev/staging/prod
- Which configs are environment-specific vs universal
- Feature flags and their current states per environment (if discoverable)

---

#### Deep Dive: Error Handling & Error Taxonomy
(Always run — depth varies)

**Error taxonomy extraction** — catalog EVERY distinct error the system can produce:
```yaml
error_catalog:
  - code: "invalid_request"
    category: "client_error"
    http_status: 400
    message_template: "Invalid request: {details}"
    trigger_conditions:
      - "Request body fails JSON schema validation"
      - "Required field is missing"
      - "Field value is outside allowed range"
    user_visible: true
    retry_semantics: "Do not retry — fix the request"
    log_level: "warn"
    includes_details: true  # response includes field-level error details
    
  - code: "rate_limited"
    category: "client_error"
    http_status: 429
    message_template: "Rate limit exceeded. Retry after {retry_after} seconds."
    trigger_conditions:
      - "Client exceeds {limit} requests per {window}"
    user_visible: true
    retry_semantics: "Retry after Retry-After header value"
    log_level: "info"
    response_headers:
      - "Retry-After: {seconds}"
      - "X-RateLimit-Remaining: 0"
    
  - code: "internal_error"
    category: "server_error"
    http_status: 500
    message_template: "An internal error occurred. Reference: {request_id}"
    trigger_conditions:
      - "Unhandled exception in request handler"
      - "Database connection failure during request"
    user_visible: true  # but message is generic
    internal_details_logged: true  # full stack trace in server logs
    retry_semantics: "May retry with exponential backoff"
    log_level: "error"
    alerts: true  # triggers alerting
```

**Error propagation model:**
- How errors bubble up through layers (exceptions? Result types? Error codes?)
- Where errors are caught vs where they propagate
- Error wrapping/chaining conventions
- How internal errors are sanitized before reaching the user
- Panic/crash handling and recovery

---

#### Deep Dive: Security Boundaries
(Run if auth_model != none OR data_classification includes PII/secrets/financial)

**Authentication contract:**
```yaml
authentication:
  primary_scheme: "JWT Bearer"
  token_format:
    type: "JWT"
    algorithm: "RS256"
    issuer: "https://auth.example.com"
    audience: "https://api.example.com"
    required_claims: ["sub", "iat", "exp", "scope"]
    max_lifetime: "1 hour"
    refresh_mechanism: "refresh token rotation"
    refresh_token_lifetime: "30 days"
  validation_steps:
    1: "Extract token from Authorization header (Bearer scheme)"
    2: "Verify JWT signature against public key from JWKS endpoint"
    3: "Verify exp claim is in the future (with 30s clock skew tolerance)"
    4: "Verify iss claim matches expected issuer"
    5: "Verify aud claim matches expected audience"
    6: "Extract scope claim for authorization"
  failure_responses:
    missing_token: { status: 401, code: "unauthorized", message: "Authentication required" }
    invalid_token: { status: 401, code: "invalid_token", message: "Token is invalid or expired" }
    expired_token: { status: 401, code: "token_expired", message: "Token has expired" }
```

**Authorization model:**
```yaml
authorization:
  model: "RBAC"  # or ABAC, or custom
  roles:
    - name: "admin"
      permissions: ["*"]
    - name: "user"
      permissions: ["resource:read", "resource:create", "resource:update_own"]
    - name: "readonly"
      permissions: ["resource:read"]
  resource_permission_map:
    "GET /api/v1/resources": ["resource:read"]
    "POST /api/v1/resources": ["resource:create"]
    "PUT /api/v1/resources/{id}": ["resource:update_own"]  # ownership check
    "DELETE /api/v1/resources/{id}": ["resource:delete", "admin"]
  ownership_model:
    enabled: true
    owner_field: "created_by"
    ownership_check: "resource.created_by == authenticated_user.id"
    admin_bypass: true
  enforcement_point: "middleware (before handler)"
```

**Trust boundary map:**
- What inputs are trusted vs untrusted
- Where input validation occurs
- What sanitization is applied and where
- Secrets handling: how secrets enter, where they're used, how they're stored
- CORS policy details
- CSP headers (if web)
- Rate limiting per endpoint/role

---

#### Deep Dive: Concurrency & Threading Model
(Run if concurrency_model != single-threaded)

**Concurrency contract:**
```yaml
concurrency:
  model: "async event loop"  # or thread-pool, actor, etc.
  runtime: "asyncio (Python) / tokio (Rust) / goroutines (Go)"
  thread_safety:
    shared_mutable_state:
      - resource: "in-memory cache"
        protection: "RWLock"
        contention_behavior: "readers proceed concurrently, writers block all"
      - resource: "connection pool"
        protection: "bounded semaphore"
        max_concurrent: 20
        overflow_behavior: "queue with 30s timeout, then reject"
    database_transactions:
      isolation_level: "READ COMMITTED"
      deadlock_handling: "retry up to 3 times with exponential backoff"
  ordering_guarantees:
    - "Events for the same entity are processed in order"
    - "Cross-entity ordering is not guaranteed"
  idempotency:
    mechanism: "idempotency key stored in database with 24h TTL"
    scope: "per-endpoint"
    collision_behavior: "return cached response"
```

---

#### Deep Dive: Observability & Ops
(Run if ops_maturity != none OR deployment_model != unknown)

**Observability contract:**
```yaml
observability:
  logging:
    format: "structured JSON"
    fields:
      always_present: ["timestamp", "level", "message", "request_id", "service"]
      on_request: ["method", "path", "status_code", "duration_ms", "user_id"]
      on_error: ["error_code", "error_message", "stack_trace"]
    levels: ["debug", "info", "warn", "error"]
    sensitive_field_redaction:
      - field_pattern: "password|token|secret|authorization"
        replacement: "[REDACTED]"
  metrics:
    exposition_format: "prometheus"
    endpoint: "/metrics"
    metrics:
      - name: "http_requests_total"
        type: "counter"
        labels: ["method", "path", "status_code"]
      - name: "http_request_duration_seconds"
        type: "histogram"
        labels: ["method", "path"]
        buckets: [0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]
      - name: "db_connections_active"
        type: "gauge"
        labels: ["pool_name"]
  health_check:
    endpoint: "/health"
    method: "GET"
    authentication: "none"
    response_schema:
      healthy: { status: 200, body: { status: "ok", version: "string", uptime_seconds: "number" } }
      degraded: { status: 200, body: { status: "degraded", checks: { db: "fail", cache: "ok" } } }
      unhealthy: { status: 503, body: { status: "unhealthy", checks: { db: "fail" } } }
    checks:
      - name: "database"
        type: "tcp_connect"
        timeout: "5s"
        critical: true  # unhealthy if this fails
      - name: "cache"
        type: "ping"
        timeout: "2s"
        critical: false  # degraded if this fails
  tracing:
    protocol: "OpenTelemetry"
    propagation: "W3C TraceContext"
    span_naming: "{http.method} {http.route}"
```

**Deployment contract:**
- Container image expectations (base image, exposed ports, signal handling)
- Startup sequence and readiness criteria
- Graceful shutdown behavior (drain connections, finish in-flight requests, timeout)
- Resource requirements (CPU, memory, disk)
- Scaling triggers and limits

---

#### Deep Dive: Test Contracts
(Run if test_maturity != none)

**Test oracle extraction** — for each major behavior, extract what constitutes correct behavior:
```yaml
test_oracle:
  behavior: "Create user via API"
  preconditions:
    - "Database is accessible"
    - "No user with the same email exists"
  input:
    method: "POST"
    path: "/api/v1/users"
    body: { name: "Alice", email: "alice@example.com" }
  expected_output:
    status: 201
    body:
      id: "any UUID v4"
      name: "Alice"
      email: "alice@example.com"
      created_at: "any ISO 8601 timestamp within 5s of now"
    headers:
      Location: "/api/v1/users/{id}"
  postconditions:
    - "User exists in database with matching fields"
    - "User status is 'active'"
    - "created_at equals updated_at"
  negative_cases:
    - input: { name: "", email: "alice@example.com" }
      expected: { status: 400, error_code: "invalid_request" }
    - input: { name: "Alice", email: "not-an-email" }
      expected: { status: 400, error_code: "invalid_format" }
    - input: { name: "Alice", email: "alice@example.com" }  # duplicate
      expected: { status: 409, error_code: "conflict" }
```

**Coverage gap analysis:**
- What behaviors are tested vs untested
- What error paths are tested vs untested
- What integration boundaries are tested vs untested
- Implicit contracts revealed by test assertions that aren't documented anywhere

---

#### Deep Dive: CLI Surface
(Run if framework_family includes cli)

**CLI contract extraction** — for EVERY command/subcommand:
```yaml
cli_command:
  name: "myapp run"
  description: "<what it does>"
  usage: "myapp run [flags] <work-item>"
  arguments:
    - name: "work-item"
      position: 1
      required: true
      type: "string"
      description: "Work item identifier"
  flags:
    - name: "--repo"
      short: "-r"
      type: "string"
      default: "."
      description: "Repository path"
      env_var: "MYAPP_REPO"
    - name: "--verbose"
      short: "-v"
      type: "boolean"
      default: false
      description: "Enable verbose output"
  stdin:
    accepts: false
  stdout:
    format: "structured text with progress indicators"
    machine_parseable: false
  stderr:
    usage: "error messages and debug logs"
  exit_codes:
    0: "Success"
    1: "General error"
    2: "Invalid arguments"
    3: "Configuration error"
    130: "Interrupted (SIGINT)"
  environment_variables:
    - name: "MYAPP_REPO"
      overrides_flag: "--repo"
    - name: "MYAPP_LOG_LEVEL"
      overrides_flag: "--log-level"
  signal_handling:
    SIGINT: "Graceful shutdown — finish current step, save checkpoint, exit 130"
    SIGTERM: "Immediate shutdown — save checkpoint, exit 143"
    SIGHUP: "Reload configuration"
```

---

#### Deep Dive: Dependency Contracts
(Always run at `full` depth)

**External dependency catalog** — for EVERY external system the code calls:
```yaml
dependency:
  name: "PostgreSQL"
  type: "database"
  required: true
  version_constraint: ">=14.0"
  connection:
    protocol: "TCP"
    default_port: 5432
    connection_string_format: "postgresql://user:pass@host:port/dbname"
    pool_size:
      min: 2
      max: 20
      idle_timeout: "300s"
  required_extensions: ["uuid-ossp", "pg_trgm"]
  required_permissions: ["SELECT", "INSERT", "UPDATE", "DELETE", "CREATE TABLE"]
  failure_behavior:
    connection_refused: "retry 3 times with exponential backoff, then fail startup"
    query_timeout: "30s default, configurable per query"
    connection_lost_mid_request: "retry transaction once, then return 503"
  health_check:
    query: "SELECT 1"
    interval: "10s"
    timeout: "5s"
```

---

### Phase 3 — Pattern Synthesis (the "aha" phase)

Goal: synthesize deep dive findings into a coherent understanding. This is where
a senior engineer says "oh, I see — this is actually a [pattern] system with
[these key invariants]."

1. **Cross-cutting pattern identification**:
   - What design patterns recur? (repository pattern, event sourcing, CQRS, etc.)
   - What conventions are followed consistently vs inconsistently?
   - Where are the "seams" between different architectural eras? (refactored vs legacy)
   - What implicit contracts exist that aren't documented anywhere?

2. **Invariant catalog** — the most critical output for reimplementation:
   ```yaml
   invariants:
     data_invariants:
       - id: "INV-D-001"
         statement: "Every order must have exactly one associated user"
         enforcement: "foreign key constraint + application validation"
         violation_behavior: "database rejects insert; API returns 400"
         test_oracle: "SELECT count(*) FROM orders WHERE user_id IS NULL = 0"
       - id: "INV-D-002"
         statement: "User email addresses are globally unique (case-insensitive)"
         enforcement: "unique index on lower(email)"
         violation_behavior: "database rejects insert; API returns 409"
     
     behavioral_invariants:
       - id: "INV-B-001"
         statement: "All API responses include a request_id header"
         enforcement: "middleware"
         test_oracle: "Every response has X-Request-ID header matching UUID format"
       - id: "INV-B-002"
         statement: "State transitions are atomic — no partial transitions"
         enforcement: "database transaction wrapping state change + side effects"
         violation_behavior: "transaction rollback; state unchanged"
     
     security_invariants:
       - id: "INV-S-001"
         statement: "No endpoint returns data belonging to a different user unless admin"
         enforcement: "ownership check in authorization middleware"
         test_oracle: "User A cannot read User B's resources (returns 404, not 403)"
     
     performance_invariants:
       - id: "INV-P-001"
         statement: "API p99 latency < 500ms for read endpoints"
         enforcement: "query optimization + connection pooling + caching"
         measurement: "prometheus histogram http_request_duration_seconds"
   ```

3. **Gap analysis** (code vs what specs SHOULD say):
   - Behaviors that exist but have no clear contract
   - Edge cases handled inconsistently
   - Security boundaries that are implicit rather than explicit
   - Operational concerns with no runbook or alert
   - Configuration that's undocumented or has no validation
   - Error paths that are handled differently across endpoints
   - Missing input validation (what's NOT checked that should be)

4. **Forward-looking insights** (what specs should DRIVE, not just document):
   - Technical debt worth calling out as spec items
   - Missing abstractions that would improve the codebase
   - Untested invariants that need test contracts
   - Operational gaps that need runbooks/alerts
   - Security hardening opportunities
   - Performance optimization opportunities with measurable targets

### Phase 4 — Spec Generation (reimplementation-grade, forward-moving)

Goal: produce specs that are contracts for the FUTURE, not just a mirror of the
present. Every spec should answer: "what must remain true, what should change,
and how do we verify it?" — AND be detailed enough for zero-question reimplementation.

Delegate to `@specwriter-axiom`:

1. **Spec architecture decision**:
   Based on the situation model, decide which spec files to produce. Not every
   repo needs every category. A CLI tool doesn't need an API contract spec.
   A data pipeline doesn't need a UI spec.

   Required for all repos:
   - `specs/00-PRD.md` — product overview, goals, non-goals, actors, key flows
   - Architecture spec — module boundaries, key decisions, dependency rules, invariant catalog

   Produce ONLY when relevant (based on situation model):
   - API contracts — when api_surface != none (MUST include full endpoint catalog at reimpl depth)
   - Data model spec — when data_complexity != none (MUST include full data dictionary + state machines)
   - Configuration spec — when config_complexity > env-vars (MUST include complete config catalog)
   - Security spec — when auth_model != none or sensitive data exists (MUST include auth/authz contracts)
   - Error taxonomy spec — when the system has user-facing error surfaces (MUST include full error catalog)
   - CLI spec — when framework_family includes cli (MUST include full command catalog)
   - Ops/deployment spec — when ops_maturity > none (MUST include observability + health contracts)
   - Concurrency spec — when concurrency_model != single-threaded (MUST include threading/locking contracts)
   - Dependency contracts spec — when external_dependencies exist (MUST include failure behavior)
   - Additional specs as warranted by the codebase

2. **Spec content rules** (reimplementation-grade requirements):

   Each spec MUST include:
   - **Testable acceptance criteria** — not vague descriptions. Every AC must have a
     concrete verification procedure that produces a binary pass/fail result.
   - **`axiom:trace` markers** linking back to source code locations
   - **Epistemic labels** on every requirement:
     - `[DISCOVERED]` — directly observed in code/tests/config
     - `[INFERRED]` — deduced from patterns, naming, or context (needs human validation)
     - `[RECOMMENDED]` — not in the code today, but should be (forward-moving)
   - **Open decisions section** for ambiguous behaviors — with explicit options and tradeoffs
   - **"Realized by" pointers** to the code that implements each requirement
   - **Negative cases** — what the system should NOT do (especially security)
   - **Wire format examples** — concrete JSON/XML/protobuf examples for every request/response
   - **Pseudocode for algorithms** — any non-trivial computation must have language-agnostic
     pseudocode or a mathematical formula, not just "calculates the score"
   - **Boundary conditions** — what happens at limits (empty input, max size, zero, negative,
     Unicode, null, concurrent access)
   - **Default behavior catalog** — what happens when optional inputs are omitted
   - **Ordering and determinism** — whether output ordering is guaranteed, and if so, by what key

   For `full` reimplementation depth, additionally require:
   - **Complete type definitions** — every data structure with every field, type, and constraint
   - **Protocol-level wire format** — exact bytes/headers/framing for non-HTTP protocols
   - **Startup/shutdown sequence** — exact order of initialization and teardown steps
   - **Migration/upgrade path** — how to go from version N to N+1 without data loss
   - **Compatibility guarantees** — what is guaranteed stable across versions

3. **Forward-moving content** (what makes this more than documentation):
   - `[RECOMMENDED]` items: things the code SHOULD do but doesn't yet
   - Technical debt callouts with severity and blast radius
   - Missing test contracts that should exist
   - Invariants that are implicit and should be made explicit
   - Operational requirements that don't exist yet but should

4. **Augment mode** (when specs already exist):
   If augmenting existing specs (mode A from preflight):
   - Identify gaps between existing specs and discovered behavior
   - Add only the missing parts — do not rewrite what's already good
   - Flag contradictions between existing specs and actual code behavior
   - Preserve existing spec IDs and numbering
   - Add reimplementation-grade detail to existing specs that lack it

### Phase 5 — Verification & Challenge

Goal: ensure the extracted specs are accurate, complete, and useful for reimplementation.

1. **Spec verification** (delegate to `@spec-verifier-axiom`):
   - Verify internal consistency of generated specs
   - Cross-reference specs against actual code to flag:
     - Specs that claim behavior the code doesn't implement
     - Code behavior not captured in any spec
   - Produce a coverage report: what percentage of the codebase is spec-covered
   - **Reimplementation readiness check**: for each spec, answer:
     - "Could an independent team implement this from the spec alone?" (yes/no/partially)
     - If no/partially: what's missing?

2. **Assumption busting** (delegate to `@assumption-buster-axiom`):
   - Review generated specs for:
     - Untestable acceptance criteria
     - Missing prerequisites
     - Ambiguous boundaries
     - Implicit assumptions that would trip up a reimplementer
     - Missing error cases
     - Unspecified edge cases
   - Inject corrections or flag for human review

3. **Security review** (delegate to `@security-review-axiom` when security_impact > none):
   - Review security-related specs for completeness
   - Identify threat model gaps
   - Flag security assumptions that need validation
   - Verify auth/authz contracts are complete enough for reimplementation

4. **Reimplementation gap analysis** (new — specific to reimpl-grade extraction):
   For each spec file, produce a gap scorecard:
   ```markdown
   ## Reimplementation Readiness: specs/30-API-Contract.md
   
   | Dimension | Score | Gap |
   |-----------|-------|-----|
   | Endpoint completeness | 95% | Missing PATCH /users/{id} |
   | Request schema completeness | 90% | 2 endpoints missing field constraints |
   | Response schema completeness | 85% | Error detail schema varies |
   | Error taxonomy completeness | 70% | 5 error codes undocumented |
   | Auth contract completeness | 100% | — |
   | State machine completeness | 80% | Refund flow missing guard conditions |
   | Config completeness | 60% | 8 env vars undocumented |
   | Test oracle completeness | 75% | Missing negative test oracles for 3 endpoints |
   | **Overall reimpl readiness** | **82%** | |
   ```

### Phase 6 — Inventory & Memory

1. **Spec inventory update**:
   - Update `specs/README.md` with the full spec inventory
   - Create/update `specs/_index.md` if the repo uses indexes

2. **Memory bank storage** (if `.memory-bank/` exists):
   - Store the Reconnaissance Report at `.memory-bank/work-items/$WORK_ITEM_ID/inputs/reconnaissance.md`
   - Store the situation model at `.memory-bank/work-items/$WORK_ITEM_ID/inputs/situation-model.yaml`
   - Store deep dive findings at `.memory-bank/work-items/$WORK_ITEM_ID/inputs/deep-dives/`
   - Store the coverage report at `.memory-bank/work-items/$WORK_ITEM_ID/runs/<RUN_ID>/verification.md`
   - Store the reimplementation readiness scorecard at `.memory-bank/work-items/$WORK_ITEM_ID/runs/<RUN_ID>/reimpl-readiness.md`

### Phase 7 — Human Review Handoff

Produce a summary that respects the human's time:

1. **Executive summary** (3-5 sentences):
   - What the codebase IS (one sentence)
   - What the situation model reveals (architecture style, key surfaces, maturity)
   - What the specs capture and what they don't
   - Reimplementation readiness score (overall percentage)
   - The single most important thing the human should validate

2. **Specs created/updated** (with paths)

3. **Reimplementation readiness dashboard**:
   ```markdown
   | Spec | Reimpl Score | Top Gap |
   |------|-------------|---------|
   | specs/00-PRD.md | 95% | Missing actor permissions matrix |
   | specs/10-Architecture.md | 88% | Module dependency graph incomplete |
   | specs/30-API-Contract.md | 82% | 5 error codes undocumented |
   | specs/40-Data-Model.md | 78% | State machine guard conditions missing |
   | **Overall** | **85%** | |
   ```

4. **Coverage dashboard**:
   - What percentage of code is now spec-covered
   - Which areas have strong coverage vs gaps
   - Visual: a simple table mapping code areas → spec coverage

5. **Human validation queue** (sorted by risk, not alphabetically):
   - `[INFERRED]` items that need human confirmation
   - `[RECOMMENDED]` items that need human buy-in
   - Open decisions that need human input
   - Contradictions between code and any existing documentation

6. **Forward-moving recommendations**:
   - Top 3-5 things the specs recommend changing/adding
   - Suggested next command: `/axiom-kickoff` or `/axiom-bootstrap`
   - If the codebase has significant gaps, suggest a phased approach
   - If reimplementation readiness is below 80%, suggest targeted deep-dive reruns

---

## Adaptation rules (how depth scales)

The command adapts its depth based on codebase size, complexity, and `$REIMPL_DEPTH`:

**Small codebase** (< 5K LOC, < 20 files):
- Phase 1 and Phase 2 can merge — do one thorough pass
- Produce fewer, more focused specs (maybe just PRD + one domain spec)
- Skip deep dives that don't apply
- Even at `standard` depth, aim for near-`full` coverage (small codebases are cheap to fully specify)

**Medium codebase** (5K-50K LOC):
- Full Phase 1 → Phase 2 flow
- 3-5 deep dives based on situation model
- Standard spec set
- At `full` depth: all deep dives, all extraction templates filled

**Large codebase** (50K+ LOC):
- Phase 1 is critical — must form good hypotheses before drilling
- Respect `$SCOPE_FILTER` aggressively — don't try to spec everything
- Produce a "spec map" showing which areas are covered and which need future passes
- Consider recommending multiple `/axiom-spec-extract` runs with different scopes
- At `full` depth: require `$SCOPE_FILTER` to prevent unbounded analysis

**Monorepo**:
- Identify service/package boundaries first
- Ask the user which service to focus on (or use `$SCOPE_FILTER`)
- Produce per-service specs with a top-level architecture spec

---

## Stop conditions

- If the repo has no analyzable code: emit BLOCKED with reason.
- If `@repo-researcher-axiom` cannot determine the primary language/framework: ask the user (max 3 questions) before proceeding.
- If spec generation produces fewer than 2 spec files: emit FAIL with gap analysis.
- If Phase 1 reveals the codebase is too large for a single pass without `$SCOPE_FILTER`: ask the user to scope (max 3 questions).
- If reimplementation readiness score is below 50% after Phase 5: emit FAIL with specific gap list and remediation plan.

---

## Output (machine-consumable)

Emit a `<axiom>` XML envelope (per `.opencode/skills/axiom-xml-protocol/SKILL.md`).

Use:
- `<command>/axiom-spec-extract</command>`
- `<status>ok|fail|blocked</status>`
- `<summary>` one sentence
- `<evidence>` include:
  - `<work_item_id>`
  - `<reimpl_depth>` (standard|full|surgical)
  - `<specs_created>` (semicolon-separated paths)
  - `<specs_updated>` (semicolon-separated paths)
  - `<coverage_percent>` (0-100, code coverage by specs)
  - `<reimpl_readiness_percent>` (0-100, reimplementation readiness score)
  - `<discovered_count>` (number of [DISCOVERED] items)
  - `<inferred_count>` (number of [INFERRED] items needing validation)
  - `<recommended_count>` (number of [RECOMMENDED] forward-moving items)
  - `<open_decisions_count>`
  - `<invariant_count>` (number of extracted invariants)
  - `<error_catalog_count>` (number of cataloged error types)
  - `<endpoint_count>` (number of API endpoints fully specified)
  - `<entity_count>` (number of data entities fully specified)
  - `<state_machine_count>` (number of state machines extracted)
  - `<config_entry_count>` (number of config entries cataloged)
  - `<test_oracle_count>` (number of test oracles defined)
  - `<dependency_count>` (number of external dependencies cataloged)
  - `<verifier_status>` (PASS|FAIL|BLOCKED)
  - `<situation_model>` (YAML summary of detected codebase characteristics)
## Output Contract (what to return to the caller)

### For Human Consumption
- Summary: one sentence stating how many specs were created and the reimplementation readiness score.
- Confidence: 0-100

### For Agent Consumption (MUST include)
- `evidence.files_changed`: list of ALL spec files created/modified (full paths, semicolon-separated)
  - Typically: `specs/*.md` files, `specs/README.md`, `specs/_index.md`
- `evidence.specs_created`: list of new spec file paths
- `evidence.specs_updated`: list of updated spec file paths
- `evidence.reimpl_readiness_percent`: overall reimplementation readiness score (0-100)
- `evidence.coverage_percent`: percentage of codebase now spec-covered
- `evidence.inferred_count`: count of [INFERRED] items needing human validation
- `evidence.work_item_path`: full path to the work item folder
- `related_commands`: suggested follow-up commands
  - "To review extracted specs, read: `specs/README.md`"
  - "To run full onboarding after extraction, run: `/axiom-onboarding`"
  - "To refresh specs after new code lands, run: `/axiom-spec-refresh`"
  - "To validate spec-to-code alignment, run: `/axiom-verify --work-item <id>`"

### Cross-References
- "Spec inventory is at: `specs/README.md`"
- "Work item artifacts are at: `.memory-bank/work-items/<id>/`"
- "Companion command: `/axiom-backfill-git` (backfills memory from git history)"

axiom:trace spec=specs/13-Command-Registry.md work_item=command-quality-01
