#!/usr/bin/env bash
# deploy_artifact_check_selfproof.sh — prove check-deployed-artifact.sh can fail.
#
# `scripts/check-deployed-artifact.sh` decides whether the LIVE dexdat-consensus
# artifact is the recorded deployment and still matches master. A gate that has
# only ever been run against a healthy target proves nothing, so this harness
# drives it with a stubbed `docker` (PATH-injected) and a throwaway git repo, and
# asserts BOTH the exit contract (0 GREEN / 1 RED / 2 VOID) and the verdict voice
# for every arm — including the RED arms the gate exists to catch:
#
#   GREEN  identity ok + master equal + repair in binary
#   RED    master ahead under --policy equal (the drift this row was filed for)
#   GREEN  same drift accepted under --policy ancestor (drift reported)
#   RED    live stamp != recorded deployment
#   RED    artifact carries no revision stamp at all
#   GREEN  stamp read from the in-image file fallback
#   RED    drift repair absent from the deployed binary
#   RED    deployed revision unknown to this repo
#   RED    deployed revision diverged from master (not an ancestor)
#   RED    no recorded deployment file
#   VOID   container absent         (measured nothing => not green)
#   VOID   docker daemon unreachable
#   VOID   master unresolvable
#   GREEN  --json machine-readable verdict
#
# usage: bash scripts/battery/deploy_artifact_check_selfproof.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK="$REPO_ROOT/scripts/check-deployed-artifact.sh"
[ -r "$CHECK" ] || { echo "selfproof: gate not found: $CHECK" >&2; exit 2; }

TMP="$(mktemp -d -t deploy-artifact-selfproof.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

# ── stub docker ─────────────────────────────────────────────────────────────
mkdir -p "$TMP/stub"
cat > "$TMP/stub/docker" <<'STUB'
#!/usr/bin/env bash
# argv shapes the gate uses:
#   docker info
#   docker inspect <container> --format '{{.State.Status}} {{.Image}}'
#   docker image inspect <id> --format '{{.Created}}'
#   docker image inspect <id> --format '{{index .Config.Labels "..."}}'
#   docker exec <container> cat /usr/local/share/consensus-upstream-sha
#   docker cp <container>:<path> <dest>
case "${1:-}" in
    info)
        exit "${STUB_DOCKER_INFO_EXIT:-0}" ;;
    inspect)
        [ "${STUB_CONTAINER_ABSENT:-0}" = 1 ] && exit 1
        printf '%s %s\n' "${STUB_STATUS:-running}" "${STUB_IMAGE_ID:-sha256:cafe}"
        ;;
    image)
        case "${5:-}" in
            *Created*) printf '%s\n' "${STUB_CREATED:-2026-10-13T00:00:00.000000000Z}" ;;
            *)         printf '%s\n' "${STUB_LABEL:-}" ;;
        esac
        ;;
    exec)
        cat "${STUB_FILE_STAMP:-/dev/null}" 2>/dev/null || true ;;
    cp)
        cp "${STUB_BINARY:-/dev/null}" "${3:?dest}" ;;
    *)
        exit 0 ;;
esac
STUB
chmod +x "$TMP/stub/docker"
export PATH="$TMP/stub:$PATH"

# ── fixtures ────────────────────────────────────────────────────────────────
# the repair's own error-message literal (internal/migrate/migrate.go:
# fmt.Errorf("add budget_limit_cents: %w", err)) — the gate's sentinel
printf 'binary-with-repair ... add budget_limit_cents ... ADD COLUMN IF NOT EXISTS budget_limit_cents ...\n' > "$TMP/bin-repair"
printf 'binary-without-repair ... no sentinel here ...\n'                        > "$TMP/bin-norepair"

# ── throwaway git repo: A -> B (master), and C on a side branch off A ───────
TREPO="$TMP/repo"
git init -q "$TREPO"
git -C "$TREPO" config user.email selfproof@example.com
git -C "$TREPO" config user.name selfproof
echo one > "$TREPO/f"
git -C "$TREPO" add f && git -C "$TREPO" commit -qm A
A="$(git -C "$TREPO" rev-parse HEAD)"
echo two > "$TREPO/f"
git -C "$TREPO" commit -qam B
B="$(git -C "$TREPO" rev-parse HEAD)"
git -C "$TREPO" branch -f master "$B"     # the gate's fallback ref resolution finds `master`
git -C "$TREPO" checkout -q -b side "$A"
echo three > "$TREPO/g"
git -C "$TREPO" add g && git -C "$TREPO" commit -qm C
C="$(git -C "$TREPO" rev-parse HEAD)"
git -C "$TREPO" checkout -q master
UNKNOWN="ffffffffffffffffffffffffffffffffffffffff"

rec() { printf '{"upstream_commit":"%s"}\n' "$1" > "$TMP/rec-$1.json"; echo "$TMP/rec-$1.json"; }
REC_A="$(rec "$A")"; REC_B="$(rec "$B")"; REC_C="$(rec "$C")"; REC_U="$(rec "$UNKNOWN")"
printf '%s\n' "$A" > "$TMP/stamp-A"

# env_live <label> <binary> <file-stamp>
env_live() {
    export STUB_LABEL="$1" STUB_BINARY="$2" STUB_FILE_STAMP="${3:-/dev/null}"
    unset STUB_CONTAINER_ABSENT STUB_DOCKER_INFO_EXIT
}

PASS=0; FAIL=0
run_arm() {  # run_arm <name> <expected-exit> <expected-verdict> <expected-substring> [gate args...]
    local name="$1" want_rc="$2" want_verdict="$3" want_sub="$4"; shift 4
    local out rc verdict ok=1
    out="$( cd "$TREPO" && "$CHECK" "$@" 2>&1 )"; rc=$?
    verdict="$(printf '%s\n' "$out" | sed -n 's/^verdict *: *//p' | head -1)"
    [ -n "$verdict" ] || verdict="$(printf '%s' "$out" | sed -n 's/.*"verdict":"\([A-Z]*\)".*/\1/p')"
    [ "$rc" = "$want_rc" ] || ok=0
    [ "$verdict" = "$want_verdict" ] || ok=0
    if [ -n "$want_sub" ]; then printf '%s' "$out" | grep -qF "$want_sub" || ok=0; fi
    if [ "$ok" = 1 ]; then
        printf 'PASS  %-44s rc=%s verdict=%s\n' "$name" "$rc" "$verdict"; PASS=$((PASS+1))
    else
        printf 'FAIL  %-44s rc=%s (want %s) verdict=%s (want %s)\n' "$name" "$rc" "$want_rc" "$verdict" "$want_verdict"
        printf '      want substring: %s\n      output:\n' "$want_sub"
        printf '%s\n' "$out" | sed 's/^/      | /'
        FAIL=$((FAIL+1))
    fi
}

echo "=== check-deployed-artifact.sh self-proof (DEPLOY-CONSENSUS-1) ==="
echo "gate : $CHECK"
echo "repo : $TREPO   A=${A:0:9} B=${B:0:9} C=${C:0:9}"
echo

env_live "$B" "$TMP/bin-repair" /dev/null
run_arm "green: identity+master+repair (ref fallback)" 0 GREEN "commits behind master : 0" \
    --record "$REC_B"
env_live "$A" "$TMP/bin-repair" /dev/null
run_arm "red: drift, master ahead (policy equal)" 1 RED "commits behind master" \
    --record "$REC_A"
env_live "$A" "$TMP/bin-repair" /dev/null
run_arm "green: same drift accepted (policy ancestor)" 0 GREEN "commits behind master : 1" \
    --record "$REC_A" --policy ancestor
env_live "$B" "$TMP/bin-repair" /dev/null
run_arm "red: live stamp != recorded deployment" 1 RED "identity mismatch" \
    --record "$REC_A" --master "$B"
env_live "" "$TMP/bin-repair" /dev/null
run_arm "red: artifact carries no revision stamp" 1 RED "no revision stamp" \
    --record "$REC_A" --master "$A"
env_live "" "$TMP/bin-repair" "$TMP/stamp-A"
run_arm "green: in-image file stamp fallback" 0 GREEN "in-image-file" \
    --record "$REC_A" --master "$A"
env_live "$A" "$TMP/bin-norepair" /dev/null
run_arm "red: repair absent from deployed binary" 1 RED "drift repair ABSENT" \
    --record "$REC_A" --master "$A"
env_live "$UNKNOWN" "$TMP/bin-repair" /dev/null
run_arm "red: deployed revision unknown to repo" 1 RED "unknown revision" \
    --record "$REC_U" --master "$A"
env_live "$C" "$TMP/bin-repair" /dev/null
run_arm "red: diverged from master (not ancestor)" 1 RED "NOT an ancestor" \
    --record "$REC_C" --policy ancestor --master "$B"
env_live "$A" "$TMP/bin-repair" /dev/null
run_arm "red: no recorded deployment file" 1 RED "no recorded deployment" \
    --record "$TMP/nope.json" --master "$A"
env_live "$A" "$TMP/bin-repair" /dev/null; export STUB_CONTAINER_ABSENT=1
run_arm "void: container absent" 2 VOID "not present" \
    --record "$REC_A" --master "$A"
env_live "$A" "$TMP/bin-repair" /dev/null; export STUB_DOCKER_INFO_EXIT=1
run_arm "void: docker daemon unreachable" 2 VOID "docker daemon unreachable" \
    --record "$REC_A" --master "$A"
env_live "$A" "$TMP/bin-repair" /dev/null
run_arm "void: master unresolvable" 2 VOID "master unresolvable" \
    --record "$REC_A" --master no-such-ref
env_live "$A" "$TMP/bin-repair" /dev/null
run_arm "green: --json machine-readable verdict" 0 GREEN '"verdict":"GREEN"' \
    --record "$REC_A" --master "$A" --json

echo
echo "self-proof: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
