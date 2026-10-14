#!/usr/bin/env bash
# check-deployed-artifact.sh — DEPLOY-CONSENSUS-1 gate.
#
# The live `dexdat-consensus` container is NOT built from this repo's working
# tree: it is built by the dexdat-core repo (consensus-sidecar/Dockerfile.quickstart)
# from a PINNED upstream consensus commit. Nothing recorded which commit was
# live, and nothing failed when master moved past the running image — so the
# artifact silently served code days behind master (board row DEPLOY-CONSENSUS-1,
# measured 2026-09-28: live image built 2026-09-24 while master was 30abbc3).
#
# This gate reads the revision stamp OUT OF THE RUNNING ARTIFACT — the OCI
# revision label written at build time from the same variable that feeds the
# upstream ADD (Dockerfile.quickstart), falling back to the in-image file
# /usr/local/share/consensus-upstream-sha — then:
#
#   1. IDENTITY  the stamp equals the deploy recorded in deploy/deployed-artifact.json
#   2. DRIFT     the stamp equals master HEAD (policy equal) or is an ancestor
#                of it (policy ancestor, drift reported with its distance)
#   3. REPAIR    the deployed BINARY contains the Postgres budget-column drift
#                repair (`add budget_limit_cents`), i.e. the artifact really
#                carries the fix this deployment exists to ship. An artifact
#                with no stamp is a RED, never a pass: an unrecorded deployment
#                is exactly the state this gate exists to catch.
#
# Exit contract (shared with scripts/battery/*):
#   0 GREEN — live artifact is the recorded one and matches master under the
#             active policy, and the drift repair is compiled into it
#   1 RED   — drift/mismatch: identity, master distance, unknown revision, or a
#             missing repair in the deployed binary
#   2 VOID  — nothing observable (docker absent/daemon down, container gone,
#             master unresolvable): NOT green — a gate that measured nothing
#             must never report success ("an idle service produces 'no errors'
#             for free" — the row's own words).
#
# Usage:
#   scripts/check-deployed-artifact.sh [--policy equal|ancestor] [--json]
#                                      [--container NAME] [--master REF]
#                                      [--record PATH] [--skip-binary-check]
set -uo pipefail

POLICY=equal
JSON=0
CONTAINER=dexdat-core-consensus-1
MASTER_REF=""
RECORD=""
SKIP_BINARY=0

usage() {
    sed -n '2,45p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
    case "$1" in
        --policy) POLICY="${2:-}"; shift 2 ;;
        --json) JSON=1; shift ;;
        --container) CONTAINER="${2:-}"; shift 2 ;;
        --master) MASTER_REF="${2:-}"; shift 2 ;;
        --record) RECORD="${2:-}"; shift 2 ;;
        --skip-binary-check) SKIP_BINARY=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "check-deployed-artifact: unknown argument: $1" >&2; exit 2 ;;
    esac
done

case "$POLICY" in equal|ancestor) ;; *) echo "check-deployed-artifact: --policy must be equal|ancestor" >&2; exit 2 ;; esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$ROOT" ]; then
    echo "check-deployed-artifact: VOID — not inside a git checkout" >&2
    exit 2
fi
[ -n "$RECORD" ] || RECORD="$ROOT/deploy/deployed-artifact.json"

REDS=()
VOID_REASON=""
IMAGE_ID=""
IMAGE_CREATED=""
CONTAINER_STATUS=""
STAMP=""
STAMP_SOURCE="none"
RECORDED=""
MASTER=""
DISTANCE=""
REPAIR="not-checked"

red() { REDS+=("$1"); }
void() { [ -z "$VOID_REASON" ] && VOID_REASON="$1"; }

emit_json() {
    local verdict="$1" reasons=""
    local r
    for r in "${REDS[@]:-}"; do
        [ -z "$r" ] && continue
        r="${r//\\/\\\\}"; r="${r//\"/\\\"}"
        if [ -n "$reasons" ]; then reasons="$reasons,"; fi
        reasons="$reasons\"$r\""
    done
    printf '{"verdict":"%s","void_reason":"%s","reasons":[%s],"container":"%s","container_status":"%s","image_id":"%s","image_created":"%s","stamp":"%s","stamp_source":"%s","recorded_commit":"%s","master":"%s","policy":"%s","commits_behind_master":"%s","drift_repair_in_binary":"%s"}\n' \
        "$verdict" "$VOID_REASON" "$reasons" "$CONTAINER" "$CONTAINER_STATUS" "$IMAGE_ID" "$IMAGE_CREATED" \
        "$STAMP" "$STAMP_SOURCE" "$RECORDED" "$MASTER" "$POLICY" "$DISTANCE" "$REPAIR"
}

verdict() {
    if [ -n "$VOID_REASON" ]; then echo VOID; elif [ "${#REDS[@]}" -gt 0 ]; then echo RED; else echo GREEN; fi
}

# ── 1. observe the running artifact ─────────────────────────────────────────
command -v docker >/dev/null 2>&1 || void "docker CLI not found"
if [ -z "$VOID_REASON" ] && ! docker info >/dev/null 2>&1; then void "docker daemon unreachable (docker info failed)"; fi

if [ -z "$VOID_REASON" ]; then
    read -r CONTAINER_STATUS IMAGE_ID <<<"$(docker inspect "$CONTAINER" \
        --format '{{.State.Status}} {{.Image}}' 2>/dev/null || true)"
    if [ -z "$IMAGE_ID" ]; then
        void "container '$CONTAINER' not present on this host"
    else
        IMAGE_CREATED="$(docker image inspect "$IMAGE_ID" --format '{{.Created}}' 2>/dev/null || true)"
        STAMP="$(docker image inspect "$IMAGE_ID" \
            --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' 2>/dev/null || true)"
        if [ -n "$STAMP" ]; then
            STAMP_SOURCE="image-label:org.opencontainers.image.revision"
        else
            STAMP="$(docker exec "$CONTAINER" cat /usr/local/share/consensus-upstream-sha 2>/dev/null | tr -d '[:space:]' || true)"
            if [ -n "$STAMP" ]; then
                STAMP_SOURCE="in-image-file:/usr/local/share/consensus-upstream-sha"
            fi
        fi
        [ -n "$STAMP" ] || red "artifact carries no revision stamp (image '$IMAGE_ID' has no org.opencontainers.image.revision label and no /usr/local/share/consensus-upstream-sha) — deployment unrecorded"
    fi
fi

# ── 2. recorded deployment ──────────────────────────────────────────────────
if [ -z "$VOID_REASON" ]; then
    if [ ! -r "$RECORD" ]; then
        red "no recorded deployment: '$RECORD' is missing or unreadable"
    else
        RECORDED="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1])).get("upstream_commit",""))' "$RECORD" 2>/dev/null || true)"
        if [ -z "$RECORDED" ]; then
            red "recorded deployment unreadable: '$RECORD' has no upstream_commit field"
        elif [ -n "$STAMP" ] && [ "$STAMP" != "$RECORDED" ]; then
            red "identity mismatch: live artifact stamp $STAMP != recorded $RECORDED (someone deployed an unrecorded revision, or the record is stale)"
        fi
    fi
fi

# ── 3. master, drift ────────────────────────────────────────────────────────
if [ -z "$VOID_REASON" ] && [ -n "$STAMP" ]; then
    if [ -n "$MASTER_REF" ]; then
        MASTER="$(git -C "$ROOT" rev-parse --verify --quiet "${MASTER_REF}^{commit}" 2>/dev/null || true)"
    else
        for ref in origin/master master main; do
            MASTER="$(git -C "$ROOT" rev-parse --verify --quiet "${ref}^{commit}" 2>/dev/null || true)"
            [ -n "$MASTER" ] && break
        done
    fi
    if [ -z "$MASTER" ]; then
        MASTER=""
        void "master unresolvable in $ROOT (no origin/master, master or main)"
    else
        if ! git -C "$ROOT" cat-file -e "${STAMP}^{commit}" 2>/dev/null; then
            red "unknown revision: deployed $STAMP is not a commit in $ROOT — cannot verify drift"
        else
            DISTANCE="$(git -C "$ROOT" rev-list --count "${STAMP}..${MASTER}" 2>/dev/null || echo '?')"
            if [ "$STAMP" = "$MASTER" ]; then
                DISTANCE=0
            elif [ "$POLICY" = equal ]; then
                red "drift: deployed $STAMP is ${DISTANCE} commits behind master $MASTER (policy=equal)"
            elif git -C "$ROOT" merge-base --is-ancestor "$STAMP" "$MASTER" 2>/dev/null; then
                : # ancestor policy: drift accepted, reported via commits_behind_master
            else
                red "diverged: deployed $STAMP is NOT an ancestor of master $MASTER"
            fi
        fi
    fi
fi

# ── 4. the repair must be IN the deployed binary ────────────────────────────
if [ -z "$VOID_REASON" ] && [ "$SKIP_BINARY" -eq 0 ]; then
    TMP_BIN="$(mktemp -t consensus-deployed-bin.XXXXXX)"
    if docker cp "$CONTAINER:/usr/local/bin/consensus-sidecar" "$TMP_BIN" >/dev/null 2>&1 && [ -s "$TMP_BIN" ]; then
        # grep -aF, not `strings`: count LINES containing the sentinel; the
        # repair's own SQL literal is the thing the drift fix adds
        # (internal/migrate/migrate.go: ADD COLUMN IF NOT EXISTS
        # budget_limit_cents), and it survives -ldflags "-s -w" stripping
        # because it is a string constant.
        if grep -aqF 'add budget_limit_cents' "$TMP_BIN"; then
            REPAIR=present
        else
            REPAIR=absent
            red "drift repair ABSENT from the deployed binary (no 'add budget_limit_cents' literal in $CONTAINER:/usr/local/bin/consensus-sidecar)"
        fi
    else
        REPAIR=unreadable
        void "could not extract the deployed binary from $CONTAINER (docker cp failed)"
    fi
    rm -f "$TMP_BIN"
fi

V="$(verdict)"

if [ "$JSON" -eq 1 ]; then
    emit_json "$V"
else
    echo "=== dexdat-consensus deployed-artifact check (DEPLOY-CONSENSUS-1) ==="
    echo "repo                  : $ROOT"
    echo "container             : $CONTAINER (state: ${CONTAINER_STATUS:-unknown})"
    echo "image                 : ${IMAGE_ID:-unknown} (created ${IMAGE_CREATED:-unknown})"
    echo "deployed stamp        : ${STAMP:-<none>}  [source: $STAMP_SOURCE]"
    echo "recorded deployment   : ${RECORDED:-<none>}  ($RECORD)"
    echo "master                : ${MASTER:-<unresolved>}  (policy: $POLICY)"
    echo "commits behind master : ${DISTANCE:-n/a}"
    echo "drift repair in image : $REPAIR"
    echo "verdict               : $V"
    if [ -n "$VOID_REASON" ]; then echo "void reason           : $VOID_REASON"; fi
    for r in "${REDS[@]:-}"; do [ -n "$r" ] && echo "reason                : $r"; done
fi

case "$V" in
    GREEN) exit 0 ;;
    RED) exit 1 ;;
    *) exit 2 ;;
esac
