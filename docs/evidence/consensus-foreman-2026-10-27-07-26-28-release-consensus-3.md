# RELEASE-CONSENSUS-3 verification evidence

Tick: consensus-foreman-2026-10-27-07-26-28
Repository: totalwindupflightsystems/consensus
Tested commit: 5ab7b5bcfe3e334b6b8d64db69d9b947f2451ec2
Remote: origin/main = 5ab7b5bcfe3e334b6b8d64db69d9b947f2451ec2 (0 ahead, 0 behind)

Verdict: READY BUT NOT AUTHORIZED; row remains pending.

Owner gate
- `RELEASE-CONSENSUS-3` explicitly says the first v0.1.0 cut awaits explicit owner authorization.
- Board search found no `RELEASE-cut` row and no authorization record.
- No tag or release was created because authorization is absent.

Acceptance probes
- `git tag -l v0.1.0`: no output; tag absent.
- `git ls-remote origin refs/tags/v0.1.0`: no output; remote tag absent.
- `gh release view v0.1.0 --repo totalwindupflightsystems/consensus`: `release not found`.
- `VERSION`: `0.1.0`.
- `CHANGELOG.md`: `## [0.1.0] — 2026-10-27`; no `[Unreleased]` heading remains.
- `go build ./...`: exit 0.
- `go test ./internal/shim/opencode/ -run TestDeclaredUnimplementedRoutesAnswerTypedEnvelope -count=1`: exit 0.
- `gh run list --repo totalwindupflightsystems/consensus --limit 3 --json databaseId,status,conclusion,displayTitle,headBranch,headSha,createdAt`: latest exact-head run 40463975286, head SHA 5ab7b5bcfe3e334b6b8d64db69d9b947f2451ec2, completed success; two preceding listed runs also completed success.
- `bash scripts/release.sh --dry-run`: exit 1 because this verification tick inherited uncommitted board/cache changes (`.coding-hermes/board/tasks.jsonl`, `.vfs/graph/.symbols_cache.json`). This is a clean-tree preflight refusal, not a release-surface failure; the exact refusal is preserved here.
- Release surfaces present: `scripts/release.sh`, `docs/release.md`, `.github/workflows/release.yml`, `.goreleaser.yaml`.

Evidence classification
- Technical readiness probes passed on the tested HEAD.
- The destructive/publishing step is owner-controlled and was not performed.
- No worker, implementation change, or GitReins task was created; this is a verification-only owner-gated closeout.

ch:trace row=RELEASE-CONSENSUS-3 spec=docs/release.md#order-of-operations wave=.coding-hermes/waves/consensus-foreman-2026-10-27-07-26-28.json#census test=scripts/release.sh:--dry-run doc=docs/release.md prompt=~/.hermes/skills/coding-hermes-foreman/references/verification-only-closeout.md evidence=docs/evidence/consensus-foreman-2026-10-27-07-26-28-release-consensus-3.md witness=github:run/40463975286 verdict=none:verification-only-owner-authorization-absent commit=5ab7b5b memory=none:foreman-writes
STATUS: pending TIER: T3 EVIDENCE: technical readiness verified; publication withheld pending explicit owner authorization

Next owner action: authorize the v0.1.0 cut by filing a RELEASE-cut row or explicit authorization naming the repository and version. Do not create or push the tag before that authorization.
