# bootstrap-output-stream-01 — Meta Planning

## Summary

`consensus init` prints the bootstrap admin key to stdout, while `consensus serve` prints it to stderr. That may be acceptable, but it is currently undocumented and easy for scripts/operators to miss. This work item decides and implements a consistent operator-facing behavior.

## Scope

In scope:
- Decide whether init and serve should use the same output stream or explicitly document different streams.
- Update code and/or docs accordingly.
- Add tests where practical.

Out of scope:
- Changing key generation or storage semantics.

## Acceptance Criteria

1. Bootstrap key output behavior is explicit and consistent with CLI/operator expectations.
2. Tests or docs prove where the key appears for `init` and `serve`.
3. No raw key is written into committed evidence.

axiom:trace work_item=bootstrap-output-stream-01 spec=specs/016-cli-interface.md plan=.memory-bank/work-items/bootstrap-output-stream-01/meta-planning.md evidence=.memory-bank/work-items/bootstrap-output-stream-01/verification.md
