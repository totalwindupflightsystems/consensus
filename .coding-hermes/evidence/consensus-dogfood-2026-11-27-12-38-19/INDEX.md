# Dogfood evidence index

Tick: consensus-dogfood-2026-11-27-12-38-19
Product tree: consensus HEAD 4454ee73d1010bac6264529aed4e6b410881d615; origin/main matched at launch and close.

## Live real-use arm

Command: `PORT=18528 bash scripts/e2e/harness-scenarios.sh --scenarios s6`

Result: exit 1 after 8.48 seconds. `s6_approve_resume` printed FAIL: approval was accepted, the session became idle, and the approved write never executed. This reproduces the existing open SCN-CONSENSUS-026 premise. A separate fresh run on PORT=18527 showed session idle, approval approved, zero successful execution, and the log line `approved tool request re-queued` despite no executable request being created.

## Performance

`perf-s6.txt` records the exact command and scope. The arm ended in a correctness failure, so no PERF row was created. No profiler was run because there was no slow successful operation to attribute.

## Fresh install leg

`bunker-qa.sh launch` ran against bunker-mvp with the exact target repository and tick binding. Agent 537e86ac was created and remained running while the detached battery executed. The latest collection synced 16 rows total: target binding OK, launch OK, Go 1.27.4/gcc/make/jq bootstrap OK, fresh-install OK, lint OK, tests OK (`go test ./... -count=1`), docker build OK, compose config OK, and Node/Compose/Buildx harness bootstrap OK. The battery was not terminal and no product-level binary smoke was harvested, so the install verdict is PARTIAL/UNVERIFIED, not passed.

The raw bunker evidence path was `/home/kara/.hermes/qa/consensus/consensus-dogfood-2026-11-27-12-38-19/evidence.jsonl`; the `.spawn` sidecar contains credentials and was intentionally not copied into the repository.