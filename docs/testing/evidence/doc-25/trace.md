ch:trace row=DOC-25 spec=docs/DEBUGGING.md wave=none:serial-tick test=none:doc-grep-gates doc=docs/DEBUGGING.md prompt=~/.hermes/skills/coding-hermes-docs/SKILL.md evidence=4641a62508d1e14d43176916c875a4df6ad4ba9b witness=none:doc-grep-artifact verdict=none:pending-foreman-judge commit=4641a62508d1e14d43176916c875a4df6ad4ba9b memory=none:not-written

DOC-25 closeout (consensus-foreman-2026-10-23-09-49-00):

CHANGE: worker (glm-5.3-flash @ zai-glm) corrected all operator-facing pprof port drift in
docs/DEBUGGING.md — 12 stale lines: the config-table Default row (now cites the real assertion
at internal/config/config_test.go:83-85), the YAML example, the log-line example, default-port
prose, every curl/pprof command example, refused-address examples in the Security section, and
the SSH tunnel example — all 127.0.0.1:8095 → 127.0.0.1:18090. The code default at
internal/config/config.go:165 (PprofAddr "127.0.0.1:18090", server.port+10000 derivation, DF-58)
was already correct and is now matched by the doc.

The dated 2026-09-29 live-probe record (~lines 224-243) kept its own per-run scratch numbers
(18915/18443/8095) — those are historical run values, not default claims. A sibling sweep found
other 8095 mentions in docs/INTEGRATION.md and docs/dogfood/* — those belong to the h3shim's own
port and dated run records, not the consensus pprof default; untouched, verified as not drift.

STATUS: complete (DOC-25)
TIER: T3 (foreman-independent verification, live)
EVIDENCE (against the committed implementation tree 4641a62):
- git grep -c '127.0.0.1:8095' 4641a62 -- docs/DEBUGGING.md -> 0 (was 15 on 69a09e5)
- git grep -c '127.0.0.1:18090' 4641a62 -- docs/DEBUGGING.md -> 15
- internal/config/config_test.go:83-85 asserts 127.0.0.1:18090 (doc citation now matches)
- git show --stat 4641a62: docs/DEBUGGING.md only, +18/-18, docs-only
- commit 4641a62508d1e14d43176916c875a4df6ad4ba9b carried one Co-authored-by trailer
- gitreins guard on commit: secrets PASS, Go gates N/A (docs-only)
- gitreins task doc-25: tier1 PASS + tier2 PASS -> verdict 3162f4e3
  (.gitreins/history/2026-10-23/3162f4e3, bound to this commit)