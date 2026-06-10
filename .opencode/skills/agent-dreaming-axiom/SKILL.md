# Skill: agent-dreaming-axiom

Proactive background session review and memory self-improvement. Extends forensics-axiom with Conductor-based background execution to achieve Claude Code "Dreaming" parity — but using Axiom's own infrastructure (Conductor + Stash + Forensics).

## When to Load

Load when:
- An agent is idle and wants to proactively improve its memory
- Conductor is available and you want to schedule a background pattern review
- A work item completes and you want to capture lessons learned
- The memory bank feels stale or missing context from recent sessions

## How It Works

"Dreaming" in Axiom is NOT a new system. It's a **recipe** that combines three existing capabilities:

1. **forensics-axiom** — already reads sessions, traces hierarchies, analyzes costs, finds patterns
2. **Conductor (spec 107)** — spawns invisible background agents that don't pollute primary context
3. **Context Stash (spec 106)** — stores findings as working memory before promoting to memory bank

The pattern:
```
conductor.spawn --name "dream-cycle" \
  --task "Review recent sessions, surface patterns, propose memory updates" \
  --stash "dream-findings" \
  --detach \
  --timeout "15m"
```

The background agent (forensics-axiom or a general agent with this skill loaded) then:
1. Queries the OpenCode session database for sessions since last dream cycle
2. Identifies recurring patterns: repeated mistakes, converging workflows, common blockers
3. Checks `.memory-bank/agents/<agent>/reflection.md` for already-known patterns
4. Writes NEW findings to the dream stash
5. Proposes memory bank updates (but does NOT auto-write — proposals only)
6. Calls `conductor.done --summary "Found 3 new patterns, 1 recurring mistake"`

The primary agent reviews findings via `stash.peek --id dream-findings` when ready.

## Dream Cycle Types

### Quick Dream (5 min, low cost)
- Review last 5 sessions only
- Surface obvious errors or repeated tool failures
- Check if any recent work contradicts memory bank entries

### Deep Dream (15 min, medium cost)
- Review sessions since last dream cycle
- Cross-session pattern analysis
- Memory bank staleness check
- Propose memory updates and findings

### Team Dream (30 min, high cost — multi-agent)
- Review sessions across multiple agents
- Surface cross-agent coordination failures
- Identify workflow convergence (agents independently choosing similar approaches)
- Propose shared best practices or skill updates

## Integration with Existing Systems

| System | Role in Dreaming |
|--------|-----------------|
| `forensics-axiom` | Does the actual session analysis (read-only DB access) |
| Conductor (spec 107) | Manages the background dream agent invisibly |
| Context Stash (spec 106) | Stores dream findings before promotion |
| Memory Bank | Final destination for confirmed patterns |
| `.memory-bank/agents/<agent>/reflection.md` | Where confirmed self-improvement notes live |
| `.memory-bank/findings/` | Where recurring patterns get documented |

## Scheduling Dream Cycles

Using Graph Harness triggers (spec 102 §17b):
```yaml
nodes:
  - id: dream-cycle
    title: "Periodic dream cycle"
    execution_mode: agent
    trigger:
      on: "idle"
      every: "6h"
      cancel_on: "active"
    description: "Load agent-dreaming-axiom skill. Run a quick dream cycle."
```

Or via Conductor detach (runs after session ends):
```
conductor.spawn --name "end-of-session-dream" --detach --task "Quick dream: review this session's work and capture lessons"
```

## What This Skill Does NOT Do

- Does NOT auto-write to memory bank (proposals only — human or primary agent approves)
- Does NOT require new infrastructure (uses forensics + conductor + stash)
- Does NOT run without Conductor (if Conductor unavailable, falls back to inline forensics call)
- Does NOT replace the adversarial review system (dreaming finds patterns; adversarial review finds flaws)

## Evidence Requirements

Dream findings MUST:
- Link to specific session IDs (for forensics drill-down)
- Label confidence: `confirmed_pattern` | `possible_pattern` | `single_observation`
- Include "how to verify" for each proposed memory update
- Never invent patterns without at least 2 supporting sessions

<!-- axiom:trace spec=specs/101-Harness-Engineering.md#REQ-HLU-007,REQ-HLU-009 -->
