---
name: decision-archaeology-axiom
description: >
  Reconstruct why choices were made and why the model or team went down a particular path.
  Traces decisions back through specs, ADRs, plans, code comments, memory bank notes, git
  history, and agent outputs to build a causal narrative. Load this skill when someone asks
  "why did we do it this way?" or when a decision's rationale is unclear.
version: "1.1"
created: "2026-03-26"
primary_spec: specs/00-PRD.md
secondary_specs:
  - specs/07-Mission-North-Star.md
  - specs/21-Traceability-Doctrine.md
tags:
  vertical: [planning, writing]
  category: planning
  core: false
---

# Decision Archaeology

> **"Every system is a fossil record of the decisions that built it. Learn to read the layers."**

This skill provides a systematic method for understanding why choices were made — whether by
humans, AI agents, or the combination of both. It reconstructs the decision chain from the
current state back to the original intent, surfacing the reasoning, constraints, alternatives
considered, and context that existed at decision time.

axiom:trace work_item=decision-archaeology-01 spec= plan= test= doc=.opencode/skills/decision-archaeology-axiom/SKILL.md evidence= commit=

---

## Activation

Load this skill when:
- Someone asks "why did we do it this way?" or "why was X chosen over Y?"
- A decision seems arbitrary and the rationale is missing
- You need to understand whether a past decision still applies given changed context
- An agent made a choice during a loop and the reasoning wasn't captured
- You're evaluating whether to change an existing approach and need to understand the original tradeoffs
- During onboarding to understand historical decisions
- Before superseding an ADR — you must understand the original decision first
- A contradiction is found and you need to determine which side was the deliberate choice

**When NOT to load this skill** (skip to avoid overhead):
- The decision rationale is already documented in an ADR or spec and you've read it
- You're making a new decision with no relevant prior art (use `hypothetical-alternatives-axiom` instead)
- The question is "what should we do?" not "why did we do this?" — archaeology answers the past, not the future

---

## The Decision Archaeology Method

### Phase 1: Identify the Decision Point

Start with the observable outcome — the code, config, spec, or behavior that exists today.

**What to capture**:
- **The artifact**: what file, line, config value, or behavior are we examining?
- **The question**: what specifically is unclear? (e.g., "why Postgres instead of SQLite?", "why does this retry 5 times?", "why is this feature flag off by default?")
- **The scope**: is this about a single decision or a chain of decisions?

### Phase 2: Dig Through the Layers

Search for evidence in this order (most authoritative first):

#### Layer 1: ADRs (Architecture Decision Records)
```
docs/decisions/*.md  OR  specs/decisions/*.md
```
- Search by subject keyword
- Check the "Alternatives Considered" section — this is the richest source of "why not"
- Check the "Context" section — this captures the forces that drove the decision
- Check "Related" links for connected decisions

#### Layer 2: Specs
```
specs/*.md
```
- Search for the subject in requirements, invariants, and non-goals
- Check "Open Decisions" and "Resolved Decisions" sections
- Look for `realized-by` links that connect requirements to implementation
- Check spec history (git blame) for when requirements changed

#### Layer 3: Memory Bank
```
.memory-bank/projects/*/
.memory-bank/topics/*/
.memory-bank/work-items/*/
.memory-bank/agents/*/
```
- Work item notes often capture in-the-moment reasoning
- Decision logs record choices made during implementation
- Agent reflection notes may explain why an agent chose a particular approach
- Implementation plan comments capture plan-time reasoning

#### Layer 4: Git History
```bash
# Find when a file/line was introduced
git log --follow -p -- <file>
git blame <file>

# Search commit messages for decision context
git log --grep="<keyword>" --oneline

# Find the PR that introduced a change
git log --merges --oneline -- <file>
```
- Commit messages (especially well-written ones) capture intent
- PR descriptions often contain the richest context
- Review comments may capture objections and resolutions

#### Layer 5: Code Comments and Documentation
- Inline comments near the decision point (especially "why" comments, not "what" comments)
- README sections, doc comments, docstrings
- TODO/FIXME/HACK comments that explain workarounds

#### Layer 6: External Context
- Jira tickets (via `jira_ref` in trace markers)
- Notion pages (via `notion_ref` in trace markers)
- Slack threads (if referenced in commits or tickets)
- External documentation, RFCs, or blog posts referenced in code/specs

### Phase 3: Handle Evidence Gaps

Not every decision has recoverable rationale. Evidence may be lost (Slack threads expired,
people left, meetings had no notes, early commits had terse messages). When you hit a dead end,
don't fabricate rationale — classify the gap and choose the right response.

**Evidence gap classification**:

| Gap Type | Signal | Response |
|----------|--------|----------|
| **Partial** | Some layers have evidence, others don't | Report what you found; mark missing layers as `evidence: not_found` |
| **Inferrable** | No explicit rationale, but constraints + context strongly imply the reason | State the inference clearly as a hypothesis, not a fact: "Likely because X, based on [evidence]" |
| **Opaque** | No evidence at any layer; decision appears to have been made without documentation | Report `rationale: unrecoverable` and recommend documenting the current understanding as a retroactive ADR |
| **Contradictory** | Different layers suggest different reasons | Report all found rationales; flag the contradiction; load `contradiction-detection-axiom` |

**Evidence gap output format**:

```yaml
decision_chain:
  - layer: "ADR"
    source: null
    finding: "No ADR found for this decision"
    confidence: none
    gap_type: "not_found"

  - layer: "git"
    source: "commit def5678 — 'switch to postgres'"
    finding: "Commit message gives no rationale beyond the change itself"
    confidence: low
    gap_type: "partial"

  - layer: "inference"
    source: "specs/12-Data-Layer.md + .memory-bank/projects/core/notes.md"
    finding: "SQLite was hitting concurrent write limits; Postgres was the only supported alternative per spec"
    confidence: medium
    gap_type: "inferrable"
    caveat: "This is an inference, not a documented decision. Verify with the team."

evidence_completeness:
  layers_searched: 6
  layers_with_evidence: 2
  confidence: "medium — inference-based; no authoritative source found"
  recommendation: "Create a retroactive ADR to document the current understanding before it's lost further"
```

**Key rule**: never present an inference as a fact. Always label the confidence level and the
basis for the inference. If confidence is `none` or `low` across all layers, say so plainly:
"The rationale for this decision could not be recovered from available evidence."

### Phase 4: Reconstruct the Decision Chain

Build a causal narrative that answers:

1. **What was the context?** — What problem was being solved? What constraints existed?
2. **What alternatives were considered?** — What other paths were available?
3. **Why was this path chosen?** — What made it better than the alternatives at the time?
4. **What tradeoffs were accepted?** — What downsides were known and accepted?
5. **What has changed since?** — Has the context shifted enough to reconsider?

### Phase 5: Assess Current Validity

For each reconstructed decision, evaluate:

| Question | If Yes | If No |
|----------|--------|-------|
| Does the original context still apply? | Decision likely still valid | May need revisiting |
| Are the original constraints still active? | Decision likely still valid | Alternatives may now be viable |
| Have the tradeoffs shifted? | Consider re-evaluation | Decision likely still valid |
| Is there new information that wasn't available? | Consider re-evaluation | Decision likely still valid |
| Has the system grown beyond the decision's assumptions? | Likely needs updating | Decision likely still valid |

---

## Output Format: Decision Archaeology Report

```yaml
decision_archaeology:
  question: "Why does the CLI use in-process mode by default instead of spawning a subprocess?"
  artifact: ".axiom/cli.py:142"
  
  decision_chain:
    - layer: "ADR"
      source: "docs/decisions/0012-in-process-cli.md"
      finding: "Chose in-process for v1 to reduce complexity and startup latency"
      date: "2026-02-15"
      confidence: high
      
    - layer: "spec"
      source: "specs/15-CLI-Contract.md#execution-modes"
      finding: "Spec defines both in-process and subprocess modes; in-process is default"
      date: "2026-02-10"
      confidence: high
      
    - layer: "git"
      source: "commit abc1234 — 'feat: add --in-process flag as default'"
      finding: "PR #42 discussion shows team agreed subprocess adds 2s startup overhead"
      date: "2026-02-16"
      confidence: medium
      
    - layer: "memory_bank"
      source: ".memory-bank/work-items/cli-modes-01/notes.md"
      finding: "Agent noted subprocess mode failed on CI due to port conflicts"
      date: "2026-02-14"
      confidence: medium

  reconstructed_rationale: |
    In-process mode was chosen as the default because:
    1. Subprocess mode added ~2s startup latency (unacceptable for interactive use)
    2. Subprocess mode had port conflict issues in CI environments
    3. In-process mode was simpler to implement for v1
    The tradeoff was accepting tighter coupling between CLI and runtime.

  alternatives_considered:
    - name: "Subprocess mode as default"
      why_rejected: "Startup latency and CI port conflicts"
    - name: "Hybrid mode (in-process for local, subprocess for CI)"
      why_rejected: "Added complexity without clear benefit for v1"

  current_validity:
    original_context_still_applies: true
    constraints_still_active: true
    tradeoffs_shifted: false
    new_information: "Subprocess mode port conflicts were later fixed (PR #67)"
    recommendation: "Decision still valid for v1. Revisit for v2 when multi-tenant support is needed."

  trace: "axiom:trace work_item=<ID> spec=specs/15-CLI-Contract.md doc=docs/decisions/0012-in-process-cli.md"
```

---

## Special Case: Agent Decision Archaeology

When the question is "why did the AI agent do X?", the evidence layers shift:

1. **Agent prompt/skill** — What instructions was the agent operating under?
2. **Input context** — What information was the agent given? (work packet, spec refs, constraints)
3. **Memory bank agent notes** — Did the agent record its reasoning?
4. **Conversation/run history** — What was the sequence of tool calls and decisions?
5. **Model behavior patterns** — Is this a known model tendency? (e.g., over-engineering, premature optimization)

Key questions for agent decisions:
- Was the agent given conflicting instructions? (→ load `contradiction-detection-axiom`)
- Was the agent missing context that would have changed the decision?
- Did the agent follow its skill/prompt correctly, or did it deviate?
- Was the decision a reasonable interpretation of ambiguous instructions?

---

## Integration with Other Skills and Agents

**Standalone usage**: This skill works independently. You do not need to have found a contradiction
first or to generate alternatives afterward. The most common use case is simply "why did we do it
this way?" — which needs no other skill. If archaeology reveals a contradiction, you *may* load
`contradiction-detection-axiom`. If you want to explore alternatives to the past decision, you
*may* load `hypothetical-alternatives-axiom`. Both are optional follow-ups.

| Skill/Agent | Integration Point |
|-------------|-------------------|
| `adr-manager-axiom` | ADRs are the primary source of decision rationale |
| `contradiction-detection-axiom` | Contradictions often require archaeology to resolve |
| `hypothetical-alternatives-axiom` | After understanding why, explore what-if |
| `@assumption-buster-axiom` | Surfaces assumptions that drove decisions |
| `@devils-advocate-axiom` | Challenges whether past decisions still hold |
| `@trace-auditor-axiom` | Trace markers help locate decision artifacts |
| `@memory-bank-axiom` | Memory bank stores decision context |
| `axiom-gap-analysis` | Missing rationale is a gap |

---

## Anti-Patterns

| Anti-Pattern | Why Bad | Fix |
|-------------|---------|-----|
| Assuming a decision was arbitrary | Most decisions had reasons; you just haven't found them yet | Dig deeper before concluding "no rationale" |
| Assuming rationale is always recoverable | Some decisions were made in ephemeral channels, by people who left, or before documentation practices existed | Use the evidence gap classification; accept `unrecoverable` as a valid outcome and create a retroactive ADR |
| Fabricating rationale to fill gaps | Invented rationale is worse than no rationale — it creates false confidence | Label inferences explicitly; never present a guess as a fact |
| Judging past decisions with current context | Hindsight bias; the decision may have been correct at the time | Reconstruct the original context first |
| Stopping at the first explanation found | The real reason may be deeper or different | Check multiple layers |
| Treating agent decisions as oracular | Agents follow instructions and context; their decisions are traceable | Trace the input context and prompt |
| Rewriting history to make past decisions look better | Destroys the learning value of the record | Record what actually happened |
| Skipping archaeology before changing something | You may reintroduce a problem the original decision solved | Always understand before changing |
| Dropping `[BACKFILLED]` caveats when summarizing | Downstream agents lose the epistemic warning and treat reconstructed rationale as authoritative | When source material carries a `[BACKFILLED]` label, propagate the label and confidence level into your output — e.g., "Rationale (backfilled, medium confidence): ..." |

---

## One-Line Reminder

Before you change something, understand why it exists. The answer is in the layers.
