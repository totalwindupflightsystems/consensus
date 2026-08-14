# AGENTS.md — Consensus Specifications

This directory is the contract (root AGENTS.md: "Product specs live in `specs/` — this is the contract").

- Specs are normative: if behavior changes, update `specs/` first, then implement.
- Code lives at the repo root — Go module `github.com/wojons/consensus` (`cmd/consensus`, `internal/`).
- GitReins quality gates (secrets, build, vet, tests) run on every commit; CI runs the same gates.
- This is a code repository with a real test suite and a runnable demo (`make smoke`) — not a documentation-only project.
- Start with `README.md` (spec inventory) and `_index.md`.
