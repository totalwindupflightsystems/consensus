#!/usr/bin/env bash
# rick-loop.sh — Outer supervisor loop for Morty
#
# Rick is the brains. Morty is the muscle.
# This script is the outer loop that Rick runs between user message checks.
# It calls rick.sh for one watch window, then acts on the exit code:
#
#   0 (tick)     — window elapsed cleanly; loop again
#   1 (restart)  — Morty is dead; restart it and wait for it to come up
#   2 (oc-down)  — OpenCode is down; wait, then loop again
#   3 (commit)   — new commit detected; report it and loop again
#   4 (error)    — Morty in terminal Error state; kill, clean up, restart
#
# Usage:
#   rick-loop.sh [options]
#
# Options (all have environment-variable equivalents):
#   --morty-bin PATH        Path to morty binary (default: morty on PATH)
#                           Env: MORTY_BIN
#   --morty-config PATH     Path to morty config file (default: morty.json)
#                           Env: MORTY_CONFIG
#   --morty-log PATH        Path for morty stdout/stderr on restart (default: .morty/morty.log)
#                           Env: MORTY_LOG
#   --rick PATH             Path to rick.sh (default: same dir as this script)
#                           Env: RICK_SH
#   --repo PATH             Repo root (passed to rick.sh). Use when running inside
#                           a nested git repo. Env: MORTY_REPO
#   --opencode-url URL      OpenCode server URL (default: http://127.0.0.1:4096)
#                           Env: OPENCODE_URL
#   --admin-url URL         Morty admin API URL (default: http://127.0.0.1:9091)
#                           Env: ADMIN_URL
#   --admin-port PORT       Admin port for morty restart (default: derived from --admin-url)
#                           Env: MORTY_ADMIN_PORT
#   --watch-seconds N       Watch window per rick.sh call (default: 300)
#                           Env: RICK_WATCH_SECONDS
#   --report-interval N     Status report interval in seconds (default: 60)
#                           Env: RICK_REPORT_INTERVAL
#   --restart-wait N        Seconds to wait after restarting Morty (default: 10)
#                           Env: MORTY_RESTART_WAIT
#   --oc-down-wait N        Seconds to wait when OpenCode is down (default: 30)
#                           Env: MORTY_OC_DOWN_WAIT
#   --log-level LEVEL       Log level for morty restart (default: info)
#                           Env: MORTY_LOG_LEVEL
#   --server-url URL        OpenCode server URL passed to morty run on restart
#                           (default: same as --opencode-url)
#                           Env: MORTY_SERVER_URL
#   --max-loops N           Maximum outer loop iterations before exiting (default: 0 = unlimited)
#                           Env: RICK_MAX_LOOPS
#   --max-error-restarts N  Max consecutive error-induced restarts before giving up (default: 5)
#                           Env: RICK_MAX_ERROR_RESTARTS
#
# Restart backoff (ADV-02 fix):
#   Consecutive error restarts (exit 1 from monitoring-degraded or exit 4 from terminal error)
#   use exponential backoff: delay = min(N² × 10s, 300s) where N = consecutive restart count.
#   The counter resets on a clean tick (exit 0) or commit (exit 3).
#   After RICK_MAX_ERROR_RESTARTS consecutive restarts, rick-loop.sh exits with code 1
#   and requires manual intervention.
#
# Exit codes (mirror rick.sh):
#   0 = clean exit (max-loops reached or SIGINT)
#   1 = Morty dead and restart failed, or max-error-restarts exceeded
#   2 = OpenCode persistently down
#
# Examples:
#   # Minimal — uses morty on PATH, morty.json in CWD, default ports
#   rick-loop.sh
#
#   # Custom binary and config
#   rick-loop.sh --morty-bin /opt/morty/bin/morty --morty-config /etc/morty/prod.json
#
#   # Non-default ports (e.g., second morty instance)
#   rick-loop.sh --opencode-url http://127.0.0.1:4097 --admin-url http://127.0.0.1:9100
#
#   # Run for exactly 3 windows then exit
#   rick-loop.sh --max-loops 3
#
# axiom:trace work_item=morty-supervision-fixes-01 spec=specs/67-Go-Agent-Orchestration-Engine.md plan=phase-5/task-5-1/step-5-1-2 jira_ref=SWDE-21

set -euo pipefail

# --- Defaults (overridable by flags or env vars) ---
MORTY_BIN="${MORTY_BIN:-morty}"
MORTY_CONFIG="${MORTY_CONFIG:-morty.json}"
MORTY_LOG="${MORTY_LOG:-}"          # derived below if empty
MORTY_REPO="${MORTY_REPO:-}"        # repo root (passed to rick.sh)
RICK_SH="${RICK_SH:-}"              # derived below if empty
OPENCODE_URL="${OPENCODE_URL:-http://127.0.0.1:4096}"
ADMIN_URL="${ADMIN_URL:-http://127.0.0.1:9091}"
MORTY_ADMIN_PORT="${MORTY_ADMIN_PORT:-}"  # derived from ADMIN_URL if empty
RICK_WATCH_SECONDS="${RICK_WATCH_SECONDS:-300}"
RICK_REPORT_INTERVAL="${RICK_REPORT_INTERVAL:-60}"
MORTY_RESTART_WAIT="${MORTY_RESTART_WAIT:-10}"
MORTY_OC_DOWN_WAIT="${MORTY_OC_DOWN_WAIT:-30}"
MORTY_LOG_LEVEL="${MORTY_LOG_LEVEL:-info}"
MORTY_SERVER_URL="${MORTY_SERVER_URL:-}"  # derived from OPENCODE_URL if empty
RICK_MAX_LOOPS="${RICK_MAX_LOOPS:-0}"
RICK_MAX_ERROR_RESTARTS="${RICK_MAX_ERROR_RESTARTS:-5}"  # max consecutive error restarts before giving up

# --- Parse flags ---
while [[ $# -gt 0 ]]; do
    case "$1" in
        --morty-bin)       MORTY_BIN="$2";          shift 2 ;;
        --morty-config)    MORTY_CONFIG="$2";        shift 2 ;;
        --morty-log)       MORTY_LOG="$2";           shift 2 ;;
        --repo)            MORTY_REPO="$2";           shift 2 ;;
        --opencode-url)    OPENCODE_URL="$2";        shift 2 ;;
        --admin-url)       ADMIN_URL="$2";           shift 2 ;;
        --admin-port)      MORTY_ADMIN_PORT="$2";    shift 2 ;;
        --watch-seconds)   RICK_WATCH_SECONDS="$2";  shift 2 ;;
        --report-interval) RICK_REPORT_INTERVAL="$2"; shift 2 ;;
        --restart-wait)    MORTY_RESTART_WAIT="$2";  shift 2 ;;
        --oc-down-wait)    MORTY_OC_DOWN_WAIT="$2";  shift 2 ;;
        --log-level)       MORTY_LOG_LEVEL="$2";     shift 2 ;;
        --server-url)      MORTY_SERVER_URL="$2";    shift 2 ;;
        --max-loops)       RICK_MAX_LOOPS="$2";      shift 2 ;;
        --max-error-restarts) RICK_MAX_ERROR_RESTARTS="$2"; shift 2 ;;
        --help|-h)
            sed -n '2,/^set -/p' "$0" | grep '^#' | sed 's/^# \?//'
            exit 0 ;;
        *)
            echo "[rick-loop] Unknown flag: $1" >&2
            exit 1 ;;
    esac
done

# --- Derive defaults that depend on other values ---

# rick.sh lives next to this script by default.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -z "${RICK_SH}" ]]; then
    RICK_SH="${SCRIPT_DIR}/rick.sh"
fi

# Morty log defaults to .morty/morty.log relative to the config file's directory.
if [[ -z "${MORTY_LOG}" ]]; then
    CONFIG_DIR="$(dirname "$(realpath "${MORTY_CONFIG}" 2>/dev/null || echo "${MORTY_CONFIG}")")"
    MORTY_LOG="${CONFIG_DIR}/.morty/morty.log"
fi

# Admin port: extract from ADMIN_URL if not explicitly set.
if [[ -z "${MORTY_ADMIN_PORT}" ]]; then
    MORTY_ADMIN_PORT="$(echo "${ADMIN_URL}" | sed 's|.*:\([0-9]*\)$|\1|')"
    # If extraction failed or produced a non-numeric result, fall back to 9091.
    if ! [[ "${MORTY_ADMIN_PORT}" =~ ^[0-9]+$ ]]; then
        MORTY_ADMIN_PORT="9091"
    fi
fi

# Server URL for morty run defaults to OPENCODE_URL.
if [[ -z "${MORTY_SERVER_URL}" ]]; then
    MORTY_SERVER_URL="${OPENCODE_URL}"
fi

# --- Validate ---
if [[ ! -f "${RICK_SH}" ]]; then
    echo "[rick-loop] rick.sh not found at ${RICK_SH}" >&2
    echo "[rick-loop] Use --rick PATH to specify its location." >&2
    exit 1
fi

if [[ ! -x "${RICK_SH}" ]]; then
    echo "[rick-loop] rick.sh is not executable: ${RICK_SH}" >&2
    echo "[rick-loop] Run: chmod +x ${RICK_SH}" >&2
    exit 1
fi

# --- Helpers ---

restart_morty() {
    local config_dir
    config_dir="$(dirname "$(realpath "${MORTY_CONFIG}" 2>/dev/null || echo "${MORTY_CONFIG}")")"
    local log_dir
    log_dir="$(dirname "${MORTY_LOG}")"
    mkdir -p "${log_dir}"

    echo "[rick-loop] Restarting Morty..."
    echo "[rick-loop]   bin:    ${MORTY_BIN}"
    echo "[rick-loop]   config: ${MORTY_CONFIG}"
    echo "[rick-loop]   server: ${MORTY_SERVER_URL}"
    echo "[rick-loop]   admin:  ${MORTY_ADMIN_PORT}"
    echo "[rick-loop]   log:    ${MORTY_LOG}"

    # Create log file with restricted permissions before redirecting output (AC-14).
    # Prevents bearer/CSRF tokens in startup logs from being world-readable.
    # axiom:trace work_item=rick-morty-hardening-01 spec=specs/67-Go-Agent-Orchestration-Engine.md plan=phase-1/task-1-3/step-1-3-1
    touch "${MORTY_LOG}" && chmod 600 "${MORTY_LOG}"

    # --config is a global flag (before the subcommand); the config file is a
    # positional argument to `morty run [flags] <config-file>`.  Passing it as
    # --config after `run` causes "unknown flag: --config".
    nohup "${MORTY_BIN}" run \
        --server-url "${MORTY_SERVER_URL}" \
        --log-level "${MORTY_LOG_LEVEL}" \
        --admin \
        --admin-port "${MORTY_ADMIN_PORT}" \
        "${MORTY_CONFIG}" \
        >> "${MORTY_LOG}" 2>&1 &

    local pid=$!
    echo "[rick-loop] Morty restarted PID=${pid}"
    echo "[rick-loop] Waiting ${MORTY_RESTART_WAIT}s for Morty to come up..."
    sleep "${MORTY_RESTART_WAIT}"
}

# --- Main outer loop ---

echo "[rick-loop] Starting outer supervisor loop"
echo "[rick-loop]   config:       ${MORTY_CONFIG}"
echo "[rick-loop]   opencode-url: ${OPENCODE_URL}"
echo "[rick-loop]   admin-url:    ${ADMIN_URL}"
echo "[rick-loop]   watch:        ${RICK_WATCH_SECONDS}s per window"
echo "[rick-loop]   max-loops:    ${RICK_MAX_LOOPS} (0=unlimited)"
echo "[rick-loop]   max-error-restarts: ${RICK_MAX_ERROR_RESTARTS}"

LOOP_COUNT=0
# Consecutive error restart counter — reset on clean tick or commit.
# Used for exponential backoff (ADV-02 fix).
# axiom:trace work_item=morty-supervision-fixes-01 spec=specs/67-Go-Agent-Orchestration-Engine.md plan=phase-5/task-5-1/step-5-1-2 jira_ref=SWDE-21
CONSECUTIVE_ERROR_RESTARTS=0

while true; do
    LOOP_COUNT=$(( LOOP_COUNT + 1 ))

    if [[ "${RICK_MAX_LOOPS}" -gt 0 && "${LOOP_COUNT}" -gt "${RICK_MAX_LOOPS}" ]]; then
        echo "[rick-loop] Max loops (${RICK_MAX_LOOPS}) reached. Exiting."
        exit 0
    fi

    # Run one rick.sh watch window.
    EXIT=0
    OPENCODE_URL="${OPENCODE_URL}" \
        "${RICK_SH}" \
        --watch "${RICK_WATCH_SECONDS}" \
        --report "${RICK_REPORT_INTERVAL}" \
        --config "${MORTY_CONFIG}" \
        --admin "${ADMIN_URL}" \
        ${MORTY_REPO:+--repo "${MORTY_REPO}"} \
        || EXIT=$?

    case "${EXIT}" in
        0)
            echo "[rick-loop] [TICK] Window complete (loop ${LOOP_COUNT})"
            # Clean tick — reset consecutive error restart counter.
            CONSECUTIVE_ERROR_RESTARTS=0
            ;;
        1)
            # Morty dead or monitoring degraded (admin API unreachable) — apply backoff.
            CONSECUTIVE_ERROR_RESTARTS=$(( CONSECUTIVE_ERROR_RESTARTS + 1 ))
            if [[ "${RICK_MAX_ERROR_RESTARTS}" -gt 0 && "${CONSECUTIVE_ERROR_RESTARTS}" -gt "${RICK_MAX_ERROR_RESTARTS}" ]]; then
                echo "[rick-loop] [FATAL] Max consecutive error restarts (${RICK_MAX_ERROR_RESTARTS}) exceeded. Manual intervention required." >&2
                exit 1
            fi
            # Exponential backoff: min(N²×10s, 300s). Applies from restart #1 onward.
            # N=1→10s, N=2→40s, N=3→90s, N=4→160s, N=5→250s, N≥6→300s (cap).
            # Note: 'local' is not valid outside a function in bash; use plain assignment.
            BACKOFF_S=$(( CONSECUTIVE_ERROR_RESTARTS * CONSECUTIVE_ERROR_RESTARTS * 10 ))
            [[ "${BACKOFF_S}" -gt 300 ]] && BACKOFF_S=300
            echo "[rick-loop] [RESTART] Morty is dead or monitoring degraded (restart #${CONSECUTIVE_ERROR_RESTARTS}/${RICK_MAX_ERROR_RESTARTS})"
            if [[ "${BACKOFF_S}" -gt 0 ]]; then
                echo "[rick-loop] [BACKOFF] Waiting ${BACKOFF_S}s before restart (exponential backoff)"
                sleep "${BACKOFF_S}"
            fi
            restart_morty
            ;;
        2)
            echo "[rick-loop] [WAIT] OpenCode is down — sending SIGTERM to Morty and waiting ${MORTY_OC_DOWN_WAIT}s"
            # AC-3 (RES-003): Kill Morty gracefully when OpenCode is down.
            # Morty is likely hung in WaitForCompletion. Kill it so it does not run
            # unsupervised while OpenCode is unavailable. restart_morty() will start
            # a fresh Morty when OpenCode comes back up.
            # axiom:trace work_item=rick-morty-hardening-01 spec=specs/67-Go-Agent-Orchestration-Engine.md plan=phase-4/task-4-1/step-4-1-1
            CONFIG_NAME=$(basename "${MORTY_CONFIG}" .json)
            CONFIG_NAME=$(basename "${CONFIG_NAME}" .yaml)
            OC_DOWN_LOCK=".morty/${CONFIG_NAME}.lock"
            if [[ -f "${OC_DOWN_LOCK}" ]]; then
                OC_DOWN_PID=$(cat "${OC_DOWN_LOCK}" 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('pid',0))" 2>/dev/null || echo "0")
                if [[ "${OC_DOWN_PID}" != "0" ]]; then
                    echo "[rick-loop] Sending SIGTERM to Morty PID=${OC_DOWN_PID} (OpenCode down)"
                    kill -TERM "${OC_DOWN_PID}" 2>/dev/null || true
                fi
            fi
            sleep "${MORTY_OC_DOWN_WAIT}"
            ;;
        3)
            echo "[rick-loop] [COMMIT] New commit detected — continuing"
            # New commit — reset consecutive error restart counter.
            CONSECUTIVE_ERROR_RESTARTS=0
            ;;
        4)
            # axiom:trace work_item=morty-supervision-fixes-01 spec=specs/67-Go-Agent-Orchestration-Engine.md plan=phase-5/task-5-1/step-5-1-2 jira_ref=SWDE-21
            # Terminal Error state — apply backoff before restart.
            CONSECUTIVE_ERROR_RESTARTS=$(( CONSECUTIVE_ERROR_RESTARTS + 1 ))
            if [[ "${RICK_MAX_ERROR_RESTARTS}" -gt 0 && "${CONSECUTIVE_ERROR_RESTARTS}" -gt "${RICK_MAX_ERROR_RESTARTS}" ]]; then
                echo "[rick-loop] [FATAL] Max consecutive error restarts (${RICK_MAX_ERROR_RESTARTS}) exceeded. Manual intervention required." >&2
                exit 1
            fi
            # Exponential backoff: min(N²×10s, 300s). Applies from restart #1 onward.
            BACKOFF_S=$(( CONSECUTIVE_ERROR_RESTARTS * CONSECUTIVE_ERROR_RESTARTS * 10 ))
            [[ "${BACKOFF_S}" -gt 300 ]] && BACKOFF_S=300
            echo "[rick-loop] [RESTART-ERROR] Morty in terminal Error state (restart #${CONSECUTIVE_ERROR_RESTARTS}/${RICK_MAX_ERROR_RESTARTS})"
            if [[ "${BACKOFF_S}" -gt 0 ]]; then
                echo "[rick-loop] [BACKOFF] Waiting ${BACKOFF_S}s before restart (exponential backoff)"
                sleep "${BACKOFF_S}"
            fi
            # Kill only the specific Morty instance for this config (AC-9).
            # Using PID from lock file instead of pkill -f to avoid killing
            # unrelated Morty instances on the same machine.
            # axiom:trace work_item=rick-morty-hardening-01 spec=specs/67-Go-Agent-Orchestration-Engine.md plan=phase-1/task-1-3/step-1-3-1
            CONFIG_NAME=$(basename "${MORTY_CONFIG}" .json)
            CONFIG_NAME=$(basename "${CONFIG_NAME}" .yaml)
            LOCK_FILE=".morty/${CONFIG_NAME}.lock"
            if [[ -f "${LOCK_FILE}" ]]; then
                STALE_PID=$(cat "${LOCK_FILE}" 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('pid',0))" 2>/dev/null || echo "0")
                if [[ "${STALE_PID}" != "0" ]]; then
                    echo "[rick-loop] Killing stale Morty PID=${STALE_PID} (config: ${CONFIG_NAME})"
                    kill "${STALE_PID}" 2>/dev/null || true
                    sleep 2
                fi
            fi
            # Remove stale lock files so the new process can start cleanly.
            rm -f ".morty/${CONFIG_NAME}.lock" 2>/dev/null || true
            restart_morty
            ;;
        *)
            echo "[rick-loop] [UNKNOWN] rick.sh exited with unexpected code ${EXIT}" >&2
            ;;
    esac
done
