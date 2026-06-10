# bootstrap-output-stream-01 — Plan

Make the first-admin-key output stream behavior deliberate. The preferred fix is to use stdout for explicit `init` output and a clear stderr startup notice for `serve`, with documentation that operators must capture stderr during first serve. If code unification is safer, change both paths and test it.

## Verification

- CLI/init output test or documented manual command.
- Serve startup output capture test or documented manual command.
- `make test`.

## Rollback

Revert stream changes/docs. Key creation remains functional.

axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md plan=.memory-bank/work-items/bootstrap-output-stream-01/plan.md evidence=.memory-bank/work-items/bootstrap-output-stream-01/verification.md
