# Consensus — Build targets
# axiom:trace work_item=repo-bootstrap-01 spec=specs/021-repository-layout.md plan=phase-1/task-1/step-4
# axiom:trace work_item=WI-016 spec=specs/018-openapi-contract.md plan=phase-5/task-5-2/step-5-2-1

BINARY    := bin/consensus
PKG       := ./...
GO        := go
CGO_FLAGS := CGO_ENABLED=0

.PHONY: build dev dev-pg test test-short smoke lint clean run docker

# --- Build ---

build:
	$(CGO_FLAGS) $(GO) build -o $(BINARY) ./cmd/consensus

# --- Development ---

dev:
	$(GO) run ./cmd/consensus serve --db-url sqlite://dev.db

dev-pg:
	$(GO) run ./cmd/consensus serve --db-url postgres://localhost:5432/consensus

# --- Test ---

test:
	$(CGO_FLAGS) $(GO) test $(PKG) -v -count=1

test-short:
	$(CGO_FLAGS) $(GO) test $(PKG) -v -short -count=1

# Keyless end-to-end smoke (C-GAP-019): real server + mocked LLM, no API key.
# Validates the install in under 60 seconds. See demo/README.md.
smoke:
	timeout 60 $(GO) test -run Smoke ./demo/

# --- Lint ---

lint:
	golangci-lint run $(PKG) || echo "golangci-lint not installed — skipping"

# --- Clean ---

clean:
	rm -rf bin/
	rm -f dev.db

# --- Docker ---

docker:
	docker build -t consensus .

# --- OpenAPI Spec (SPEC-018) ---

.OPENAPI_SPEC := specs/openapi/openapi.yaml
.OPENAPI_BUNDLED := specs/openapi/bundled.yaml

.PHONY: bundle-spec lint-spec contract-test

# Bundle split OpenAPI files into single output file (SPEC-018 §5.1)
bundle-spec:
	@echo "==> Bundling OpenAPI spec..."
	if command -v npx &>/dev/null && npx --yes @redocly/cli bundle $(.OPENAPI_SPEC) --output $(.OPENAPI_BUNDLED) 2>/dev/null; then \
		echo "==> Spec bundled to $(.OPENAPI_BUNDLED)"; \
	elif command -v yq &>/dev/null; then \
		echo "Warning: redocly not available; attempting yq merge..."; \
		yq eval-all '. as $$item ireduce ({}; . * $$item)' specs/openapi/paths/*.yaml > specs/openapi/_merged_paths.yaml && \
		echo "==> Partial merge complete. Install redocly for full bundle: npm install -g @redocly/cli"; \
	else \
		echo "==> Skipping bundle (no bundler available). Install redocly: npm install -g @redocly/cli"; \
	fi

# Validate the bundled spec (SPEC-018 §5.1)
lint-spec:
	@echo "==> Linting OpenAPI spec..."
	@if command -v npx &>/dev/null; then \
		npx --yes @redocly/cli lint $(.OPENAPI_BUNDLED) || true; \
	else \
		echo "redocly not available. Install: npm install -g @redocly/cli"; \
	fi

# Run contract tests against a running server (SPEC-018 §6)
# Requires: consensus server running, API key in CONSENSUS_API_KEY env var
contract-test: build
	@echo "==> Running contract tests..."
	@chmod +x bin/contract-test.sh 2>/dev/null || true
	@if [ -z "$$CONSENSUS_API_KEY" ]; then \
		echo "ERROR: CONSENSUS_API_KEY environment variable required"; \
		echo "Usage: CONSENSUS_API_KEY=cs_ak_... make contract-test"; \
		exit 1; \
	fi
	@SERVER_URL="http://localhost:8090"; \
	echo "Testing $$SERVER_URL..."; \
	\
	echo "  [1/5] GET /api/v1/health..."; \
	HEALTH=$$(curl -s -o /dev/null -w "%{http_code}" "$$SERVER_URL/api/v1/health" 2>/dev/null); \
	if [ "$$HEALTH" = "200" ]; then echo "    ✓ Health endpoint (200)"; else echo "    ✗ Health endpoint: $$HEALTH (expected 200)"; fi; \
	\
	echo "  [2/5] POST /api/v1/sessions (no auth)..."; \
	NOAUTH=$$(curl -s -o /dev/null -w "%{http_code}" -X POST "$$SERVER_URL/api/v1/sessions" -H "Content-Type: application/json" -d '{}' 2>/dev/null); \
	if [ "$$NOAUTH" = "401" ]; then echo "    ✓ Unauthenticated rejected (401)"; else echo "    ✗ Expected 401, got $$NOAUTH"; fi; \
	\
	echo "  [3/5] GET /api/v1/sessions (with auth)..."; \
	SESSIONS=$$(curl -s -o /dev/null -w "%{http_code}" "$$SERVER_URL/api/v1/sessions" -H "Authorization: Bearer $$CONSENSUS_API_KEY" 2>/dev/null); \
	if [ "$$SESSIONS" = "200" ]; then echo "    ✓ Sessions list (200)"; else echo "    ✗ Expected 200, got $$SESSIONS"; fi; \
	\
	echo "  [4/5] POST /api/v1/sessions (create)..."; \
	CREATE=$$(curl -s -o /tmp/_contract_create.json -w "%{http_code}" -X POST "$$SERVER_URL/api/v1/sessions" -H "Authorization: Bearer $$CONSENSUS_API_KEY" -H "Content-Type: application/json" -d '{"agent_name":"test","goal":"contract test"}' 2>/dev/null); \
	if [ "$$CREATE" = "201" ] || [ "$$CREATE" = "200" ]; then \
		echo "    ✓ Session created ($$CREATE)"; \
		cat /tmp/_contract_create.json 2>/dev/null | head -c 200; echo; \
	else echo "    ✗ Expected 201/200, got $$CREATE"; fi; \
	\
	echo "  [5/5] OpenAPI spec serving..."; \
	SPEC=$$(curl -s -o /dev/null -w "%{http_code}" "$$SERVER_URL/openapi.yaml" 2>/dev/null); \
	if [ "$$SPEC" = "200" ]; then echo "    ✓ OpenAPI spec served (200)"; else echo "    ✗ Expected 200, got $$SPEC"; fi; \
	\
	echo ""; \
	echo "==> Contract test complete."

# --- Postgres Integration Tests ---
# Requires: docker compose up -d (starts postgres:16)
# Runs: Postgres-specific migration bootstrap test (skips gracefully when PG unavailable)
.PHONY: test-pg
test-pg:
	CONSENSUS_TEST_POSTGRES_URL=postgres://consensus:consensus_test_pw@localhost:5432/consensus_test?sslmode=disable \
		$(CGO_FLAGS) $(GO) test ./internal/migrate -run TestPostgres -v -count=1

# Full Postgres integration test — applies all migrations, verifies tables/indexes/triggers,
# exercises CRUD, FK constraints, and append-only enforcement.
# Requires: docker compose up -d (postgres:16-alpine on port 5432)
# axiom:trace work_item=WI-postgres-full-integration spec=specs/003-database.md plan=phase-1/task-1/step-1
.PHONY: test-pg-full
test-pg-full:
	CONSENSUS_TEST_POSTGRES_URL=postgres://consensus:consensus@localhost:5432/consensus?sslmode=disable \
		$(CGO_FLAGS) $(GO) test ./internal/migrate -run TestPostgresFullIntegration -v -count=1

# --- Run built binary ---

run: build
	$(BINARY)

# --- GitReins ---
# One-command setup: pip install gitreins && make gitreins-install
# Tier 1: make gitreins-guard     (secrets, vet, test-short — fast, no LLM)
# Tier 2: make gitreins-eval       (LLM judge — costs tokens, takes 2-4 min)
# Full:   make gitreins            (guard first, eval after — block on guard fail)

.PHONY: gitreins-install gitreins-guard gitreins-eval gitreins

# Install the pre-commit hook and verify setup
gitreins-install:
	@echo "==> Installing GitReins pre-commit hook..."
	@if [ ! -f .git/hooks/pre-commit ]; then \
		cp .gitreins/pre-commit .git/hooks/pre-commit && \
		chmod +x .git/hooks/pre-commit && \
		echo "✓ Hook installed — GitReins Tier 1 will run on every commit"; \
	elif diff .gitreins/pre-commit .git/hooks/pre-commit > /dev/null 2>&1; then \
		echo "✓ Hook already installed and up to date"; \
	else \
		cp .gitreins/pre-commit .git/hooks/pre-commit && \
		chmod +x .git/hooks/pre-commit && \
		echo "✓ Hook updated to latest version"; \
	fi
	@if [ -z "$$GITREINS_LLM_API_KEY" ]; then \
		echo "⚠ GITREINS_LLM_API_KEY not set — Tier 2 evaluator won't run"; \
		echo "  Set it in ~/.hermes/.env or export it for Tier 2"; \
	fi
	@echo "✓ GitReins installed — run 'make gitreins-guard' to verify"

# Tier 1: Static guards only — fast, no LLM tokens
gitreins-guard:
	@echo "━━━ GitReins Tier 1: Static Guards ━━━"
	@bash .gitreins/pre-commit

# Tier 2: Agentic evaluator — LLM judges work against criteria
# Usage: make gitreins-eval TASK=build-gate
#         make gitreins-eval TASK=all
gitreins-eval:
	@if [ -z "$(TASK)" ]; then echo "Usage: make gitreins-eval TASK=<task-id|all> [BUDGET=200k]"; echo "Tasks: build-gate, db-migrations, secrets-hygiene, all"; exit 1; fi
	@if [ ! -d ~/gitreins-poc ]; then \
		echo "GitReins engine not found. Cloning..."; \
		git clone https://github.com/totalwindupflightsystems/gitreins.git ~/gitreins-poc; \
	fi
	@if [ "$(TASK)" = "all" ]; then \
		cd ~/gitreins-poc && PYTHONPATH=. python3 ~/consensus/.gitreins/eval-runner.py --all "$(BUDGET)"; \
	else \
		cd ~/gitreins-poc && PYTHONPATH=. python3 ~/consensus/.gitreins/eval-runner.py $(TASK) "$(BUDGET)"; \
	fi

# Full pipeline: guard first, then evaluate
gitreins: gitreins-guard
	@echo ""
	@echo "━━━ GitReins Tier 2: Agentic Evaluator (all tasks) ━━━"
	@cd ~/gitreins-poc && PYTHONPATH=. python3 ~/consensus/.gitreins/eval-runner.py --all
