# Strategy Falsification — Axiom Skill

<!-- axiom:trace work_item=sprint-44-runtime-decision-gates-01 spec=specs/77-Adversarial-Review-System.md#strategy-falsification-stage plan=phase-4/task-4-1/step-4-1-1 evidence=.memory-bank/work-items/sprint-44-runtime-decision-gates-01/verification.md#ac-8 -->

## Purpose

Load this skill when any agent needs to run a Strategy Falsification pass inline — without dispatching `@strategy-falsifier-axiom` as a full subagent. It provides the same five-element output contract in a portable, loadable form so that PM agents, dev agents, and orchestrators can embed the check directly in their own context window.

**When to load this skill:**
- You are a PM or dev agent executing a bug-fix plan step and need to produce a Strategy Falsification section before writing code.
- You are an orchestrator that wants to inline the falsification check rather than dispatch a subagent.
- You are writing a plan and need to know what the Strategy Falsification stage requires.
- You want to verify that a Strategy Falsification section in a verification.md is complete and correct.

**When to dispatch `@strategy-falsifier-axiom` instead:**
- You want a fully isolated, adversarial review with its own context window.
- The work item is high-risk and you want the falsification to run in parallel with other adversarial agents.
- You are running `/axiom-adversary` — the agent is dispatched automatically in the assumptions category.

Spec contract: `specs/77-Adversarial-Review-System.md#strategy-falsification-stage`

---

## Gate Position

**Gate 3** in the bug-fix gate order (`specs/20-Meta-Planning.md#gate-order`):

```
Gate 1: Staleness / Already-Resolved  →  intake/meta-plan
Gate 2: Bug Fix Mode selection         →  meta-plan
Gate 3: Strategy Falsification         →  pre-implementation  ← THIS GATE
Gate 4: Reproduce-or-Flag              →  pre-implementation
Gate 5: Live/Dead Path Check           →  pre-implementation
Gate 6: PR Scope Discipline            →  pre-PR
Gate 7: Post-PR Review Bot Response    →  post-PR
```

Runs **after** Reproduce-or-Flag (Gate 4 in numbering, but Gate 3 runs first in the Phase 0 sequence) and **before** any implementation step is created.

---

## Required Elements

Before any implementation step runs, produce all five elements:

| # | Element | Requirement |
|---|---|---|
| 1 | **Selected hypothesis** | One-sentence testable statement of the proposed root cause and fix approach |
| 2 | **Alternatives** | ≥2 alternatives with name, pros, cons, rejection rationale. MUST include one structurally different approach AND the status-quo option. |
| 3 | **Falsification criteria** | 2–4 concrete conditions under which the proposed fix would fail to solve the problem or introduce a regression |
| 4 | **Blast radius** | Enumerate callers/consumers affected; classify as NONE / WARN / HARD BLOCK |
| 5 | **Existing-fix check** | Check recent commits, open PRs, related ticket comments. Output: CLEAR or DUPLICATE |

---

## Semantics

| Condition | Result |
|---|---|
| ≥1 alternative documented | **WARN** — execution may proceed; warn recorded in verification.md |
| Zero alternatives AND non-mechanical work | **HARD BLOCK** — do not proceed until alternatives are documented |
| Mechanical fix exception applies | **PASS** — abbreviated output is sufficient |
| Existing-fix check returns DUPLICATE | **HARD BLOCK** — stop; recommend closing or verifying the existing fix |

---

## Mechanical Fix Exception

If the fix is a single-line typo, config value, or import correction with no meaningful strategic alternatives, use the abbreviated format:

```markdown
## Strategy Falsification

axiom:trace work_item=<ID> spec=specs/77-Adversarial-Review-System.md#strategy-falsification-stage plan=<REF> evidence=<verification.md#strategy-falsification>

**Verdict**: PASS

Mechanical fix — no alternatives required. Hypothesis: <one sentence>. Blast radius: none. Existing-fix check: <CLEAR or caveat>.
```

This exception applies only when ALL of the following are true:
- Fix is limited to a single-line typo, config value, or import correction
- No behavioral strategy choice is involved
- Blast radius is none or trivially local

---

## Output Format

Produce this Markdown section and write it to the work item's `verification.md` under `## Strategy Falsification`:

```markdown
## Strategy Falsification

axiom:trace work_item=<ID> spec=specs/77-Adversarial-Review-System.md#strategy-falsification-stage plan=<phase/task/step> evidence=<verification.md#strategy-falsification>

**Verdict**: PASS | WARN | HARD BLOCK

### 1. Selected Hypothesis
<one-sentence testable hypothesis>

### 2. Alternatives Considered

*Plain-text summary (for PR descriptions and Jira comments where tables may not render accessibly)*: Alternatives considered: [A] <name> (rejected: <reason>); [B] <name> (rejected: <reason>); Status quo (rejected: <reason>).

| # | Alternative | Pros | Cons | Rejection Rationale |
|---|---|---|---|---|
| A | <name> | <pros> | <cons> | <why rejected> |
| B | <name> | <pros> | <cons> | <why rejected> |
| Status quo | Do nothing / accept current behavior | <pros> | <cons> | <why rejected> |

### 3. Falsification Criteria
1. Fix fails if: <condition>
2. Fix fails if: <condition>
3. Fix fails if: <condition>

### 4. Blast Radius
- **Target**: `<file/function/path>`
- **Callers affected**: <list or "none identified">
- **Impact classification**: NONE | WARN | HARD BLOCK
- **Rationale**: <one sentence>

### 5. Existing-Fix Check
- Recent commits to target: <result>
- Open PRs on same path: <result>
- Related ticket comments: <result>
- **Verdict**: CLEAR | DUPLICATE
```

---

## Blast Radius Classification

| Classification | When to use |
|---|---|
| **NONE** | No live callers, no shared consumers, no externally visible behavior, no production path, no downstream contract change |
| **WARN** | Localized live path, single known consumer, uncertain but bounded caller set, or reversible behavior change |
| **HARD BLOCK** | Multiple live consumers, externally visible behavior, shared runtime infrastructure, auth/security/data/billing/permissions impact, migration risk, or unbounded unknown caller set |

---

## Evidence Integrity Rules

- Do NOT claim a commit, PR, or ticket comment exists unless you have actual evidence.
- If evidence is unavailable, say so explicitly: "Recent commits: unavailable — no git access in this context."
- If unavailable evidence prevents an honest CLEAR verdict, return HARD BLOCK or blocked status.
- Never upgrade inferred facts to verified facts.

---

## Relationship to `@strategy-falsifier-axiom`

This skill and the agent share the same output contract. The difference is execution context:

| | Skill | Agent |
|---|---|---|
| **Execution** | Inline in caller's context window | Isolated subagent context window |
| **When to use** | PM/dev agent embedding the check in a plan step | Adversarial review battery, high-risk changes, parallel dispatch |
| **Output** | Same `## Strategy Falsification` Markdown section | Same `## Strategy Falsification` Markdown section |
| **Auto-dispatched by `/axiom-adversary`** | No | Yes (assumptions category) |

---

## Quick Reference Checklist

Before claiming a Strategy Falsification section is complete, verify:

- [ ] Selected hypothesis is one sentence and testable (can be proven false)
- [ ] ≥2 alternatives for non-mechanical work (or mechanical exception documented)
- [ ] Alternatives include one structurally different approach
- [ ] Alternatives include status-quo option
- [ ] Falsification criteria are concrete and observable (not vague)
- [ ] Blast radius identifies affected paths/callers or explicitly states unknowns
- [ ] Existing-fix check does not claim evidence that was not actually available
- [ ] DUPLICATE maps to HARD BLOCK
- [ ] Verdict is consistent with findings
- [ ] Trace marker is present

---

## Example: Full Non-Mechanical Pack

```markdown
## Strategy Falsification

axiom:trace work_item=PROJ-123 spec=specs/77-Adversarial-Review-System.md#strategy-falsification-stage plan=phase-1/task-1-1/step-1-1-1 evidence=.memory-bank/work-items/PROJ-123/verification.md#strategy-falsification

**Verdict**: WARN

### 1. Selected Hypothesis
Adding a null check at the top of `UserService.getUser()` will prevent the NPE for all callers that pass null, without breaking callers that pass valid IDs.

### 2. Alternatives Considered

| # | Alternative | Pros | Cons | Rejection Rationale |
|---|---|---|---|---|
| A | Add null check at method entry (selected) | Catches all null inputs at the boundary; single change point | Throws at call site — callers relying on null-return semantics will break | Selected because the stack trace confirms null enters here; callers should not pass null |
| B | Return `Optional.empty()` instead of throwing | Caller-friendly; no exception propagation | Requires all callers to handle Optional; larger blast radius | Rejected because 3 callers already handle the exception path |
| Status quo | Do nothing / accept NPE | No implementation cost | NPE propagates to users; confirmed reproduction | Rejected because the bug is confirmed and user-visible |

### 3. Falsification Criteria
1. Fix fails if: adding the null check does not eliminate the NPE in the reproduction test.
2. Fix fails if: a caller legitimately passes null as a sentinel value and the throw breaks their contract.
3. Fix fails if: the NPE originates in a downstream method called by `getUser()`, not at the null parameter itself.

### 4. Blast Radius
- **Target**: `src/UserService.java#getUser`
- **Callers affected**: `UserController.getUser()`, `AdminController.lookupUser()`, `UserService.java` (self-call in `refreshUser`)
- **Impact classification**: WARN
- **Rationale**: Three live callers; all currently handle the exception path; no external contract change.

### 5. Existing-Fix Check
- Recent commits to target: No commits to `src/UserService.java` in the last 30 days addressing null handling.
- Open PRs on same path: No open PRs touching `UserService.java`.
- Related ticket comments: No comments indicating a prior fix was accepted.
- **Verdict**: CLEAR
```
