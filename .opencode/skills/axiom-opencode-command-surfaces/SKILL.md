---
name: axiom-opencode-command-surfaces
description: Portable contract for invoking repo-local OpenCode commands from Axiom — command normalization, validation, allowlist enforcement, argument forwarding, and fail-closed error semantics.
version: "1.0"
synopsis: |
  Defines the contract for invoking repo-local OpenCode commands via CLI (`axiom cmd <command>`)
  and runtime adapter paths. Covers canonical command name normalization (slash stripping, lowercase
  validation), allowlist enforcement (fail-closed), argument forwarding (opaque, never shell-executed),
  API/CLI parity, deterministic error semantics, and runtime evidence requirements.
when-to-use: |
  Load this skill when implementing the `axiom cmd` CLI surface, building command dispatch in the
  runtime adapter, validating command names, enforcing allowlists, or designing fail-closed command
  invocation paths.
tags:
  vertical: [coding, ops]
  category: tooling
  core: false
---

# Axiom OpenCode Command Execution Surfaces (Portable)

This skill defines the contract for invoking repo-local OpenCode commands from Axiom.

Source spec: `specs/57-OpenCode-Command-Execution-Surfaces.md`

---

## CLI Surface

```
axiom cmd <command> [args...]
```

- `<command>` is required
- `[args...]` is optional, forwarded as opaque text
- Help output includes both slash and no-slash invocation examples

---

## Canonical Command Name Normalization

Before validation and dispatch, normalize the raw command name:

1. Trim surrounding whitespace
2. Remove exactly one leading `/` if present
3. Reject empty result

### Equivalence Examples

| Raw Input | Canonical Form |
|---|---|
| `/axiom-batch-commit` | `axiom-batch-commit` |
| `axiom-batch-commit` | `axiom-batch-commit` |

### Rejected Examples

| Raw Input | Reason |
|---|---|
| `/` | Empty after normalization |
| `` (empty) | Empty input |
| `../axiom-step` | Fails character policy |
| `DOES-NOT-EXIST` | Fails character policy (uppercase) |

---

## Validation and Character Policy

Canonical command names MUST match:

```
^[a-z0-9][a-z0-9-]{0,127}$
```

Rules:
- Lowercase ASCII letters, digits, and hyphens only
- Must start with a letter or digit (not hyphen)
- Maximum 128 characters
- Names violating policy are rejected before dispatch

---

## Allowlist Enforcement (Fail-Closed)

| Rule | Detail |
|---|---|
| Validation | Canonical name checked against allowlist before dispatch |
| Unknown command | Invocation MUST fail closed |
| No fallback | No fallback to arbitrary template paths |
| No user paths | MUST NOT read command templates from user-supplied filesystem paths |
| Discovery | Allowlist MAY be runtime-discovered (`GET /command`) or preconfigured |
| Unavailable discovery | Behavior MUST remain fail-closed |

---

## Argument Forwarding

| Rule | Detail |
|---|---|
| Treatment | Arguments are opaque text payload |
| Shell execution | Arguments MUST NOT be shell-executed by Axiom |
| Joining | Arguments MAY be joined into a single string for OpenCode invocation |
| Special characters | Data, not code |

---

## API/CLI Normalization Parity

The same normalization/validation logic MUST be reused by:
- CLI `axiom cmd ...` path
- Runtime adapter command-dispatch path

A change in normalization behavior MUST affect both paths together.

---

## Deterministic Error Semantics

Invalid command invocations return deterministic failures:

| Aspect | Value |
|---|---|
| Exit code | `2` |
| Error class | User-input error (not internal crash) |
| Message | Explains whether failure was `missing_command`, `invalid_format`, or `unknown_command` |

### Error Decision Tree

```
Is command argument provided?
  No  -> exit 2, "missing_command"
  Yes -> Normalize (trim, strip leading /)
         Is result empty?
           Yes -> exit 2, "invalid_format"
           No  -> Does it match ^[a-z0-9][a-z0-9-]{0,127}$ ?
                    No  -> exit 2, "invalid_format"
                    Yes -> Is it in the allowlist?
                             No  -> exit 2, "unknown_command"
                             Yes -> Dispatch (success path)
```

---

## Runtime Evidence Requirement

Work claiming this surface complete MUST include Tier 3 runtime evidence for:
- At least one successful command invocation
- At least one fail-closed rejection path

---

## Valid Examples

```bash
axiom cmd /axiom-batch-commit
axiom cmd axiom-batch-commit --scope staged
```

## Invalid Examples

```bash
axiom cmd /                    # exit 2: invalid_format
axiom cmd "../axiom-step"    # exit 2: invalid_format
axiom cmd DOES-NOT-EXIST       # exit 2: invalid_format (uppercase)
axiom cmd nonexistent-cmd      # exit 2: unknown_command
```

---

## Cross-Spec Alignment

| Topic | Spec |
|---|---|
| Command discovery and naming | `specs/13-Command-Registry.md` |
| Transport-level dispatch | `specs/31-OpenCode-Integration-Contract.md#4-command-execution` |
| Input validation posture | `specs/43-Input-Sanitization-And-Untrusted-Content.md` |
| Hardening gates | `specs/32-Security-Hardening-Roadmap.md` |

---

## Open Decisions

1. Should allowlist discovery results be cached, and if yes, what TTL?
2. Should v1 expose session-control flags (`--session`, `--new-session`) on `axiom cmd`?
