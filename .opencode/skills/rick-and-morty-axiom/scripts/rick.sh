#!/usr/bin/env bash
# rick.sh — Tight watchdog loop for Morty supervision
#
# Rick is the brains. Morty is the muscle.
# This script is the tight inner loop that Rick runs to watch Morty.
#
# Usage:
#   ./rick.sh [options]
#   ./rick.sh [watch_seconds] [report_interval_seconds] [morty_config] [admin_url]
#
# Named options (positional args are supported for backward compat):
#   --watch SECONDS         Max seconds to watch before clean exit (default: 300)
#   --report SECONDS        How often to print a status line (default: 60)
#   --config PATH           Path to morty config file (default: morty.json)
#   --admin URL             Morty admin API URL (default: http://127.0.0.1:9091)
#   --repo PATH             Repo root (default: auto-detect from git or script dir)
#                           Use this when running inside a nested git repo where
#                           git rev-parse resolves to the wrong root.
#
# Exit codes:
#   0 = clean exit — watch window elapsed, report status and re-invoke
#   1 = Morty dead — restart Morty
#   2 = OpenCode down — wait for OpenCode, then restart Morty
#   3 = new commit — report progress, re-invoke
#
# Design principles:
#   - Tight inner loop: 10s check interval, up to 13s per iteration
#     (10s sleep + up to 3s zombie detection: 3 retries × 1s timeout)
#     Worst-case detection latency: 13s (zombie) + 8s (OpenCode health) = up to 21s
#     Note: Zombie detection adds up to 3s per iteration (3 retries × 1s timeout)
#   - Exit on event: exits immediately when something interesting happens
#   - Caller owns actions: this script only observes; caller decides what to do
#   - Blocked but responsive: blocking call that returns quickly on state change
#
# axiom:trace work_item=morty-upgrade-01 spec=specs/67-Go-Agent-Orchestration-Engine.md jira_ref=DEX-455

set -euo pipefail

# --- Repo root detection ---
# Priority: --repo flag > MORTY_REPO env > git rev-parse > script dir parent
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse named options (must come before positional parsing)
REPO_ROOT=""
WATCH_SECONDS=""
REPORT_INTERVAL=""
MORTY_CONFIG=""
ADMIN_URL=""
HELP=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --repo|--repo-path)
            REPO_ROOT="$2"
            shift 2
            ;;
        --repo=*|--repo-path=*)
            REPO_ROOT="${1#*=}"
            shift
            ;;
        --watch)
            WATCH_SECONDS="$2"
            shift 2
            ;;
        --watch=*)
            WATCH_SECONDS="${1#*=}"
            shift
            ;;
        --report)
            REPORT_INTERVAL="$2"
            shift 2
            ;;
        --report=*)
            REPORT_INTERVAL="${1#*=}"
            shift
            ;;
        --config)
            MORTY_CONFIG="$2"
            shift 2
            ;;
        --config=*)
            MORTY_CONFIG="${1#*=}"
            shift
            ;;
        --admin)
            ADMIN_URL="$2"
            shift 2
            ;;
        --admin=*)
            ADMIN_URL="${1#*=}"
            shift
            ;;
        --help|-h)
            HELP=true
            shift
            ;;
        --*)
            echo "[rick] Unknown option: $1" >&2
            exit 1
            ;;
        *)
            # Positional args (backward compat): assume watch/report/config/admin in order
            if [[ -z "$WATCH_SECONDS" ]]; then WATCH_SECONDS="$1"
            elif [[ -z "$REPORT_INTERVAL" ]]; then REPORT_INTERVAL="$1"
            elif [[ -z "$MORTY_CONFIG" ]]; then MORTY_CONFIG="$1"
            elif [[ -z "$ADMIN_URL" ]]; then ADMIN_URL="$1"
            else echo "[rick] Unexpected positional argument: $1" >&2; exit 1
            fi
            shift
            ;;
    esac
done

if $HELP; then
    echo "Usage: $0 [options]"
    echo "  --watch SECONDS         Watch window (default: 300)"
    echo "  --report SECONDS        Report interval (default: 60)"
    echo "  --config PATH           Morty config file path (default: morty.json)"
    echo "  --admin URL             Admin API URL (default: http://127.0.0.1:9091)"
    echo "  --repo PATH             Repo root override (default: auto-detect)"
    echo ""
    echo "Positional args (legacy): watch report config admin"
    exit 0
fi

# Resolve repo root
if [[ -z "$REPO_ROOT" ]]; then
    REPO_ROOT="${MORTY_REPO:-}"
fi
if [[ -z "$REPO_ROOT" ]]; then
    # Try git first, fall back to script dir parent
    REPO_ROOT="$(git -C "${SCRIPT_DIR}" rev-parse --show-toplevel 2>/dev/null || echo "")"
fi
if [[ -z "$REPO_ROOT" ]]; then
    REPO_ROOT="${SCRIPT_DIR}/../.."
fi
REPO_ROOT="$(cd "${REPO_ROOT}" 2>/dev/null && pwd || echo "${REPO_ROOT}")"

cd "${REPO_ROOT}"

echo "[rick] Repo root: ${REPO_ROOT}"

# --- Configuration ---
WATCH_SECONDS="${WATCH_SECONDS:-300}"
REPORT_INTERVAL="${REPORT_INTERVAL:-60}"
MORTY_CONFIG="${MORTY_CONFIG:-morty.json}"
ADMIN_URL="${ADMIN_URL:-http://127.0.0.1:9091}"
OPENCODE_URL="${OPENCODE_URL:-http://127.0.0.1:5192}"

# --- State ---
START_TIME=$(date +%s)
LAST_REPORT_TIME=$START_TIME
LAST_COMMIT=""

# Capture the current HEAD commit at startup so we can detect new commits.
if git rev-parse --git-dir >/dev/null 2>&1; then
    LAST_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "")
fi

# --- Helpers ---

# is_morty_running: returns 0 if Morty is running (lock file + live process).
# Returns 1 immediately if no lock file exists — the lock file is the sole
# authoritative mechanism. The pgrep fallback was removed.
# The lock file path is resolved relative to REPO_ROOT, matching Morty's
# behavior (lockfile.MortyDir = ".morty").
is_morty_running() {
    local config_name
    config_name=$(basename "${MORTY_CONFIG}" .json)
    config_name=$(basename "${config_name}" .yaml)
    local lock_file="${REPO_ROOT}/.morty/${config_name}.lock"

    if [[ -f "${lock_file}" ]]; then
        local pid
        pid=$(cat "${lock_file}" 2>/dev/null | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('pid',0))" 2>/dev/null || echo "0")
        if [[ "${pid}" != "0" ]]; then
            kill -0 "${pid}" 2>/dev/null || return 1
            local try
            for try in 1 2 3; do
                curl -sf --max-time 1 "${ADMIN_URL}/api/status" >/dev/null 2>&1 && return 0
                sleep 1
            done
            echo "[rick] Morty process alive but admin API unresponsive after 3s (zombie detected). Treating as dead."
            return 1
        fi
    fi

    return 1
}

# is_opencode_healthy: returns 0 if OpenCode server responds to /global/health
is_opencode_healthy() {
    curl -sf --max-time 15 "${OPENCODE_URL}/global/health" >/dev/null 2>&1
}

# get_morty_status: returns rich status from admin API (cycle/stage/state) or "unknown"
get_morty_status() {
    local status_json
    status_json=$(curl -sf --max-time 3 "${ADMIN_URL}/api/status" 2>/dev/null) || true
    if [[ -z "${status_json}" ]]; then
        echo "unknown"
        return
    fi
    python3 -c "
import json, sys
d = json.loads('''${status_json}''')
state = d.get('state', 'unknown')
cycle = d.get('cycle', '?')
stage = d.get('current_stage', d.get('stage', '?'))
print(f'{state} (cycle={cycle} stage={stage})')
" 2>/dev/null || echo "unknown"
}

# get_current_commit: returns current HEAD commit hash
get_current_commit() {
    git rev-parse HEAD 2>/dev/null || echo ""
}

# elapsed: returns seconds since START_TIME
elapsed() {
    echo $(( $(date +%s) - START_TIME ))
}

# since_last_report: returns seconds since LAST_REPORT_TIME
since_last_report() {
    echo $(( $(date +%s) - LAST_REPORT_TIME ))
}

# print_status: print a human-readable status line
print_status() {
    local morty_state="dead"
    local opencode_state="down"

    if is_morty_running; then
        morty_state=$(get_morty_status)
    fi

    if is_opencode_healthy; then
        opencode_state="up"
    fi

    local elapsed_s
    elapsed_s=$(elapsed)
    local remaining=$(( WATCH_SECONDS - elapsed_s ))

    echo "[rick] t+${elapsed_s}s | morty=${morty_state} | opencode=${opencode_state} | window=${remaining}s remaining"
    LAST_REPORT_TIME=$(date +%s)
}

# --- Main watchdog loop ---

echo "[rick] Starting watchdog: watch=${WATCH_SECONDS}s report=${REPORT_INTERVAL}s config=${MORTY_CONFIG}"
print_status

while true; do
    # 1. Check if watch window has elapsed → clean exit.
    local_elapsed=$(elapsed)
    if (( local_elapsed >= WATCH_SECONDS )); then
        echo "[rick] Watch window complete (${local_elapsed}s elapsed). Exiting cleanly."
        print_status
        exit 0
    fi

    # 2. Check if OpenCode is down → exit 2.
    if ! is_opencode_healthy; then
        echo "[rick] OpenCode is DOWN at ${OPENCODE_URL}. Exiting with code 2."
        exit 2
    fi

    # 3. Check if Morty is dead → exit 1.
    if ! is_morty_running; then
        echo "[rick] Morty is DEAD (no live process for config ${MORTY_CONFIG}). Exiting with code 1."
        exit 1
    fi

    # 4. Check for new commits → exit 3.
    if [[ -n "${LAST_COMMIT}" ]]; then
        current_commit=$(get_current_commit)
        if [[ -n "${current_commit}" && "${current_commit}" != "${LAST_COMMIT}" ]]; then
            commit_msg=$(git log --oneline -1 "${current_commit}" 2>/dev/null || echo "${current_commit:0:8}")
            echo "[rick] New commit detected: ${commit_msg}. Exiting with code 3."
            exit 3
        fi
    fi

    # 5. Periodic status report.
    since_report=$(since_last_report)
    if (( since_report >= REPORT_INTERVAL )); then
        print_status
    fi

    # 6. Sleep 10s (tight inner loop).
    sleep 10
done
