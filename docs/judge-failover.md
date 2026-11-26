# Standing Tier-2 Judge-Lane Failover (QA-CONSENSUS-039)

Status: standing doctrine
Date: 2026-11-24
Applies to: the GitReins Tier-2 evaluator (LLM judge) used to grade board rows in this repo.

## 1. Purpose

The Tier-2 evaluator grades every board row that lands in this repo, so a dead
or unentitled judge lane does not just fail one task — it silently parks the
whole board in "ungraded" state. Three incidents on record (two iteration-cap
aborts that burned over a million input tokens each while grading nothing, and
one 403 where the evaluator fell through to an OpenRouter fallback model the
key is not entitled to) showed that lane selection is not a per-run decision:
without a standing, documented failover, every new run re-discovers the same
dead lanes and re-pays for the discovery. This document is that failover: the
resolution rules, the probed lanes on this host, and the criteria-authoring
rules that prevent the cap-abort failure class in the first place.

## 2. Incident evidence

| Run id | Date | Failure shape | Tokens | Model named in the error |
|---|---|---|---|---|
| `af4510fb` (QA-CONSENSUS-33) | 2026-11 | Iteration-cap abort at 120 iterations; 0 of 1 criteria graded | 1,302,708 input tokens | — |
| `71446a06` (INT-CI-CONSENSUS-1) | 2026-11 | Iteration-cap abort at 120.7 iterations; 0 of 1 criteria graded | 1,019,049 input tokens | — |
| `c7fb8bf1` (RELEASE-CONSENSUS-009) | 2026-11-22 | Tier 2 died with `403 Client Error: Forbidden` on `https://openrouter.ai/api/v1/chat/completions` | — | `moonshotai/kimi-k3` (provider=openai, via openrouter) |

The two cap aborts share one root cause: the criterion was SHAPE, not size —
one bundled criterion carrying five live-process assertions let the evaluator
iterate unboundedly against processes that never converge on a stable
verdict. See section 5.

## 3. Judge-lane resolution rules

The Tier-2 evaluator reads ONLY these three environment variables:

- `GITREINS_LLM_PROVIDER` — must be `openai` or `anthropic`
- `GITREINS_LLM_BASE_URL` — the chat-completions endpoint base
- `GITREINS_LLM_API_KEY` — the credential for that endpoint

The `defaults` block in `.gitreins/config.yaml` is decoration; it does not
drive the judge. A lane override is THREE values as ONE unit — changing the
provider, the base URL, or the key alone produces a mismatched lane that
401s/403s at run time. Always set all three together.

HTTP status semantics when a lane dies:

- `403` — the key is valid at *some* endpoint but not entitled at the endpoint
  being called (wrong key for that provider, or plan does not cover the
  model). This is what RELEASE-CONSENSUS-009 hit: the fallback model
  `moonshotai/kimi-k3` was routed through openrouter under a key not entitled
  to it.
- `402` / `412` — money or account state (out of credit, account suspended).
  Do not retry; the account needs attention, not the job.
- `429` / `5xx` — capacity. Retry later or on another lane, but never
  re-dispatch the same job against the same dead provider in the same tick.

Standing rule: **never re-dispatch the same job against the same dead
provider.** One failure proves the lane; a second identical attempt proves
nothing new and doubles the risk of another token-burning cap abort.

## 4. Failover lanes (probe of record)

Probed 2026-11-24T23:20Z by the foreman via the fleet probe script
(`/home/kara/.hermes/skills/devops/judge-provider-failover/scripts/probe-judge-lanes.sh`;
keys are read from the environment and never printed). Keys are referenced by
env-var NAME only in this document — never write a literal key value here or
anywhere in the repo.

Re-probed 2026-11-26T08:10Z (same script + direct /models + completion curls):
z.ai ambient triple HTTP 429 (weekly plan cap), deepseek HTTP 402
(insufficient balance), ollama-cloud HTTP 429 (weekly usage limit),
openrouter HTTP 403 (key weekly limit exceeded), fireworks HTTP 412
(suspended). The ONLY 200 lane is xai, but `grok-4-fast` is now ABSENT from
`https://api.x.ai/v1/models` — the gitreins lane-coherence check refuses it.
Current xai ids: grok-4.3 / grok-4.5 / grok-4.6 / grok-4.7 (completion probe
of grok-4.7 returned HTTP 200). Use `GITREINS_LLM_MODEL=grok-4.7` for the xai
lane until the next re-probe; the table below reflects the 2026-11-24 probe.

| Provider | Base URL | Model id | Key env-var name | Probe result |
|---|---|---|---|---|
| deepseek | `https://api.deepseek.com` | `deepseek-flash` | `DEEPSEEK_API_KEY` | HTTP 200 |
| ollama-cloud | `https://ollama.com/v1` | `glm-5.3-flash` | `OLLAMA_CLOUD_API_KEY` | HTTP 200 |
| xai | `https://api.x.ai/v1` | `grok-4-fast` (served `grok-4.3`) | `XAI_API_KEY` | HTTP 200 |
| openrouter | `https://openrouter.ai/api/v1` | `deepseek/deepseek-v4-flash` | `GITREINS_OPENROUTER_KEY` | HTTP 200 |
| fireworks | `https://api.fireworks.ai/inference/v1` | `accounts/fireworks/models/kimi-k3` | `FIREWORKS_KEY_GITREINS` | HTTP 412 (account suspended) |

Standing re-probe rule: the probe results in this table are POINT-IN-TIME
claims — provider entitlements and account states change, and a lane listed
as HTTP 200 on probe day can be dead today. Re-probe a lane (one curl to its
chat/completions endpoint, or the probe script above) immediately before
dispatching a judge on it, and update the probe date in this table after
every re-probe. FAILIF a judge is dispatched on a lane whose status was not
re-verified in the same session.

Model-id form rules:

- **openrouter** takes the PREFIXED `org/model` form (`deepseek/deepseek-v4-flash`).
- **deepseek** takes the BARE model name (`deepseek-flash`).

First choice on this host: the ambient z.ai coding-plan triple already set in
the environment —

- `GITREINS_LLM_BASE_URL=https://api.z.ai/api/coding/paas/v4`
- `GITREINS_LLM_MODEL=glm-5.3-flash`
- `GITREINS_LLM_PROVIDER=openai`

curl-probed HTTP 200 with a real completion at 2026-11-24T23:2xZ. This is the
lane the evaluator should be left on; the other 200 lanes above are the
failover order, and fireworks stays off the rotation until its account is
reinstated.

## 5. Criteria-authoring rules (the cap preventer)

The two cap aborts (af4510fb, 71446a06) were caused by criterion SHAPE, not by
a slow model. Rules for authoring tier-2 criteria:

- **One assertion per criterion.** Never bundle five assertions into one
  criterion; a bundled criterion gives the evaluator no discrete step to
  settle on and drives unbounded iteration.
- **Every criterion carries two fields:**
  - `PROOF:` — how to verify it (the evidence the judge should look for).
  - `FAILIF:` — what makes it fail (the exact condition that flips the verdict).
- **Command-shaped criteria beat live-process witnesses.** "grep this file",
  "run this test and assert exit 0" — these converge. A live-process witness
  ("curl the running server", "check a daemon is up") makes the evaluator
  re-probe forever because a live process can always change state between
  probes; that is exactly how af4510fb and 71446a06 hit the 120-iteration cap
  with 1.3M/1.0M input tokens and zero criteria graded. When a row genuinely
  needs live-state evidence, reduce it to a command the foreman runs once and
  records the output of, and make the criterion grade the RECORDED output.
- **A cap abort must be reported as ungraded, never as a substantive FAIL.**
  An iteration-cap abort means the judge ran out of budget, not that the work
  failed — recording it as FAIL poisons the board with a verdict nobody
  verified.

## 6. Verdict binding

- Verdicts live at `.gitreins/history/YYYY-MM-DD/<run-dir>/verdict.json`.
- **Match the verdict's `.task_id` to the board row id before citing it.** A
  verdict from a different row (or a stale one) binding to this row's board
  closeout is exactly the kind of false evidence the history tree exists to
  prevent.
- **Close order:**
  1. `gitreins judge <task-id>` — mint the verdict first.
  2. `boardctl complete` with `--evidence-run-id` naming the history
     DIRECTORY (the bare hex dir under `.gitreins/history/<date>/`, not the
     "Verdict saved" id the judge CLI prints — those two ids can differ).
  3. `gitreins task complete <task-id>` — this adopts the stored verdict when
     the tree, criteria, and lane are identical to what the verdict was minted
     against. If any of the three drifted, re-judge instead of forcing the
     adoption.
