# SCN-CONSENSUS-024 — live no-selector run with --auto-approve (remaining acceptance arm)

- Tick: consensus-foreman-2026-11-25-10-24-51
- Date: 2026-11-25 (UTC-05)
- Tree: main d5e0d53d (fix landed upstream as 47daac7e, merged via c846e197)
- Command: `PORT=18462 bash scripts/e2e/harness-scenarios.sh --auto-approve`
- Exit: 0 — Elapsed: 41s total (each scenario well under the 180s idle timeout)
- Provider: funded DeepSeek key from ~/.hermes/.env (auto-sourced by the runner);
  server booted with default consensus.yaml (provider openai -> api.deepseek.com/v1, model deepseek-v4-flash)
- Premise note: the row note's "every provider call on this host still answers HTTP 402"
  (SCN-CONSENSUS-017 premise, 2026-11-24) is STALE as of this run — DeepSeek answers 200.

## Runner output (verbatim)

```
server up on :18462 (pid 531396)
auto-approve: seeded requires_approval=0 for every enabled registry tool
--- s1_write_file
{"scenario":"s1","status":"pass","detail":"wrote /mnt/ssd/tmp/consensus-scn.3jp9BC/tmp/consensus-workspaces/7776609d-9aab-46b7-8508-bda14c54dcd7/scn1.txt"}
--- s2_read_file
{"scenario":"s2","status":"pass","detail":"read tool returned the seeded content"}
--- s3_bash
{"scenario":"s3","status":"pass","detail":"bash executed"}
--- s4_conversation_loop
{"scenario":"s4","status":"pass","detail":"two turns, two files"}
--- s5_containment
{"scenario":"s5","status":"pass","detail":"no escape file outside the session workspace"}
=== summary ===
s1_write_file=PASS
s2_read_file=PASS
s3_bash=PASS
s4_conversation_loop=PASS
s5_containment=PASS
scenarios run: 5
failures: 0
```

## Vacuous-green check (real LLM traffic proven)

The runner logs scenario JSON to stdout but server logs to a throwaway state dir
(rm -rf'd on exit), so stdout has no llm: lines. A second single-scenario run with
--keep (`PORT=18463 ... --scenarios s1 --keep`, also PASS) preserved the server log;
its LLM lines:

```
level=INFO msg="llm: calling provider" url=https://api.deepseek.com/v1/chat/completions model=deepseek-v4-flash messages=3
level=INFO msg="llm: response received" model=deepseek-flash elapsed_ms=1676 prompt_tokens=1688 completion_tokens=267 cached_tokens=0 reasoning_promoted=false
level=INFO msg="llm: calling provider" url=https://api.deepseek.com/v1/chat/completions model=deepseek-v4-flash messages=5
level=INFO msg="llm: response received" model=deepseek-flash elapsed_ms=3590 prompt_tokens=1792 completion_tokens=596 cached_tokens=1408 reasoning_promoted=false
```

Zero "paused" lines and zero timeout burns in the full-run log. The failures: 0 line is
0 because every scenario printed pass — the acceptance's anti-vacuous condition holds.
