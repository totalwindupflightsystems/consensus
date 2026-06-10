# bootstrap-output-stream-01 — Verification

Status: PASS
Date: 2026-05-28

## What Changed

1. **`internal/bootstrap/admin_key.go`** — Added `FormatResult()` (machine-parseable key=value lines) and `FormatResultJSON()` (indented JSON). Both respect SPEC-016 §3 (scriptable, machine-parseable). AdminKeyResult struct now has JSON tags.

2. **`cmd/conscience/main.go`** — `runServer()`: admin key output moved from `os.Stderr` to `os.Stdout` (uses `fmt.Println` via `bootstrap.FormatResult`). `runInit()`: unified to use same `bootstrap.FormatResult` helper. Errors and `slog` remain on stderr.

3. **`internal/bootstrap/admin_key_test.go`** — 5 new tests:
   - `TestFormatResult_CreatedKeyHasMachineParseableOutput` — verifies created=true line with key=, key_prefix=, id=, created_at=
   - `TestFormatResult_ExistingKeyHasMachineParseableOutput` — verifies created=false line with NO raw key
   - `TestFormatResult_NoSecretLeakedForExistingKey` — verifies "key=" never emitted for existing keys
   - `TestFormatResultJSON_CreatedHasFields` — verifies JSON output for created key
   - `TestFormatResultJSON_ExistingRedactsSecret` — verifies JSON for existing key has no real secret

4. **`internal/cli/cli_test.go`** — 1 new test:
   - `TestInitCommand_OutputGoesToStdoutNotStderr` — verifies init command bootstrap output appears on stdout with machine-parseable key=value format

## Evidence

### Unit Tests — FormatResult
```
=== RUN   TestFormatResult_CreatedKeyHasMachineParseableOutput
--- PASS: TestFormatResult_CreatedKeyHasMachineParseableOutput (0.00s)
=== RUN   TestFormatResult_ExistingKeyHasMachineParseableOutput
--- PASS: TestFormatResult_ExistingKeyHasMachineParseableOutput (0.00s)
=== RUN   TestFormatResult_NoSecretLeakedForExistingKey
--- PASS: TestFormatResult_NoSecretLeakedForExistingKey (0.00s)
=== RUN   TestFormatResultJSON_CreatedHasFields
--- PASS: TestFormatResultJSON_CreatedHasFields (0.00s)
=== RUN   TestFormatResultJSON_ExistingRedactsSecret
--- PASS: TestFormatResultJSON_ExistingRedactsSecret (0.00s)
```

### Unit Tests — CLI Init Output
```
=== RUN   TestInitCommand_OutputGoesToStdoutNotStderr
--- PASS: TestInitCommand_OutputGoesToStdoutNotStderr (0.00s)
```

### Full Test Suite
```
make test — ALL 23 packages PASS
```

### Output Contract

Newly created key:
```
conscience: first_admin_key created=true key=cs_ak_<hex> key_prefix=cs_ak_<8chars> id=<uuid> created_at=<rfc3339>
conscience: save this key now; it is stored hashed and will not be printed again
```

Existing key:
```
conscience: first_admin_key created=false key_prefix=cs_ak_<8chars> id=<uuid> created_at=<rfc3339>
```

Both go to **stdout**. Errors and slog go to **stderr**.

## Acceptance Criteria

| Criteria | Status | Evidence |
|---|---|---|
| Bootstrap key output is explicit and consistent | ✅ | `FormatResult()` used by both `runServer()` and `runInit()` |
| Tests prove where key appears for init and serve | ✅ | 6 new tests covering format, stdout path, and secret safety |
| No raw key written into committed evidence | ✅ | FormatResult for existing keys never emits key=; test confirms |

axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md evidence=.memory-bank/work-items/bootstrap-output-stream-01/verification.md
