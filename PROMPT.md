# Consensus — Morty Improvement Queue Builder

Morty is now running a staged improvement queue. Do not choose broad work. Use the current Morty stage name to select exactly one work item, implement the smallest useful slice, and stop after producing verifiable evidence for that stage.

## Stage-to-work-item map

1. `build-serve-bootstrap-test` → `.memory-bank/work-items/runtime-serve-bootstrap-test-01/plan.md`
2. `build-bootstrap-output-stream` → `.memory-bank/work-items/bootstrap-output-stream-01/plan.md`
3. `build-postgres-bootstrap-test` → `.memory-bank/work-items/postgres-bootstrap-verification-01/plan.md`
4. `build-bootstrap-key-policy` → `.memory-bank/work-items/bootstrap-admin-key-policy-01/plan.md`
5. `build-api-rate-limit-scope` → `.memory-bank/work-items/api-rate-limit-scope-01/plan.md`
6. `build-repo-hygiene-generated-artifacts` → `.memory-bank/work-items/repo-hygiene-generated-artifacts-01/plan.md`

If the current stage is not listed, read `.morty/consensus.json` and select the work item referenced by the closest matching stage name. If still unclear, stop with `STATUS: BLOCKED` and explain the ambiguity.

## Builder rules

1. Read the selected work item’s `meta-planning.md`, `plan.md`, `plan.yaml`, and `verification.md` before editing code.
2. Keep scope fenced to the selected work item. Do not opportunistically fix other findings unless the selected plan explicitly requires it.
3. Update specs first if the intended behavior changes the contract.
4. Add or update tests before claiming completion.
5. Run the verification commands listed in the selected work item.
6. Update that work item’s `verification.md` with commands run, results, limitations, and trace refs.
7. Use trace markers near changed behavior boundaries: `axiom:trace work_item=<id> spec=<ref> plan=<ref> impl=<path> test=<path> evidence=<path>`.

## Default verification commands

Run the work-item-specific commands first. Unless the work item says otherwise, also run:

```bash
go test ./internal/bootstrap -v -count=1
go test ./internal/api -v -count=1
make test
```

If a command is not relevant or cannot run locally, record why in the selected work item’s `verification.md` and return `STATUS: BLOCKED` if it is required for that stage.

## Output contract

End every builder response with a short summary of files changed and verification evidence. Do not invent test output. If blocked, provide exact next steps.

axiom:trace work_item=morty-work-queue-stages-01 spec=specs/015-api-and-mcp.md,specs/016-cli-interface.md plan=.memory-bank/work-items/*/plan.md prompt=PROMPT.md
