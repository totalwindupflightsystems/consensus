#!/usr/bin/env bash
# run-env.sh — SG-2. The single committed place where every variable the 284 manifest
# commands reference is DEFINED or NAMED. Before this file existed the plan used 27
# variables with zero assignments anywhere, which made ~200 command slots silently
# skippable (Quorum #1 C3, contradicted 5/5).
#
# Source it, do not execute it:   . scripts/battery/run-env.sh
#
# Two classes of variable, and the difference matters:
#   BASE_*     — set here, with real defaults.
#   RUN_STATE  — produced by an earlier tier (a session id, an agent id). These are
#                NAMED here but must be non-empty before the tier that consumes them
#                runs; `require_run_state` enforces that loudly instead of letting a
#                command proceed with an empty string.

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/../.." && pwd)}"
export REPO_ROOT

# ---------- identity of the run (freeze it) ----------
export RUN_ID="${RUN_ID:-$(date -u '+%Y%m%dT%H%M%SZ')}"
export RUN="${RUN:-$RUN_ID}"
export TARGET_SHA="${TARGET_SHA:-$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null)}"

# ---------- layout ----------
export RUN_ROOT="${RUN_ROOT:-$HOME/consensus-reports/run-$RUN}"
export EV="${EV:-$RUN_ROOT/evidence}"
export RUN_STATE_FILE="${RUN_STATE_FILE:-$RUN_ROOT/run-state.env}"

# ---------- the server under test ----------
export BASE="${BASE:-http://127.0.0.1:8090}"
export SRV="${SRV:-$RUN_ROOT/consensus.pid}"          # pid file, not the pid
export DB="${DB:-$RUN_ROOT/db/consensus.db}"
export P="${P:-8090}"                                  # port

# ---------- credentials: PATHS ONLY, never values ----------
export KEY="${KEY:-$RUN_ROOT/.keys/user.key}"          # chmod 600, written by the key preflight
export ADMIN_KEY="${ADMIN_KEY:-$RUN_ROOT/.keys/admin.key}"
export A="${A:-$ADMIN_KEY}"
export K="${K:-$KEY}"
export LIVE="${LIVE:-$RUN_ROOT/.keys/live.key}"        # a provider key, staged chmod 600

# ---------- crier (T7) ----------
export CRIER_BASE="${CRIER_BASE:-http://127.0.0.1:9119}"
export CRIER_TOKEN="${CRIER_TOKEN:-$RUN_ROOT/.keys/crier.token}"

# ---------- load tooling (T9) ----------
export LOAD1="${LOAD1:-$(dirname "${BASH_SOURCE[0]:-$0}")/bin/loadgen}"
export B="${B:-$RUN_ROOT/load}"                        # load output dir

# ---------- run-state variables: NAMED here, produced by earlier tiers ----------
# `code`, `p` are command-locals and are NOT in this list.
RUN_STATE_VARS="S1 S2 S3 S4 AG"
export RUN_STATE_VARS

# put the shims on PATH so a declared prerequisite resolves on the run host
export PATH="$(dirname "${BASH_SOURCE[0]:-$0}")/bin:$PATH"

mkdir -p "$RUN_ROOT" "$EV" "$RUN_ROOT/.keys" "$RUN_ROOT/db" 2>/dev/null || true
chmod 700 "$RUN_ROOT/.keys" 2>/dev/null || true

# ---------- helpers a tier uses instead of hoping ----------
require_run_state() {  # require_run_state VAR... -> VOID if any is empty
  local v missing=""
  for v in "$@"; do
    eval "local val=\${$v:-}"
    [ -z "$val" ] && missing="$missing $v"
  done
  if [ -n "$missing" ]; then
    echo "run-env: VOID — run-state variable(s) not set:${missing}. The tier that produces them must run first." >&2
    return 2
  fi
  return 0
}
export -f require_run_state 2>/dev/null || true

ALL_DECLARED_VARS="RUN_ID RUN TARGET_SHA RUN_ROOT EV RUN_STATE_FILE BASE SRV DB P KEY ADMIN_KEY A K LIVE CRIER_BASE CRIER_TOKEN LOAD1 B $RUN_STATE_VARS HOME PATH PWD"
export ALL_DECLARED_VARS
