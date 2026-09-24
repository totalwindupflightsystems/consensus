#!/bin/bash
# Consensus — Local Postgres Deployment
# axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-2/step-1-2-2
set -euo pipefail

DB_URL="${1:-postgres://postgres:password@localhost:5432/consensus}"
PORT="${2:-8090}"

echo "=== Consensus Local (Postgres) ==="
echo "Database: $DB_URL"
echo "Port:     $PORT"
echo ""

# Build if binary doesn't exist
if [ ! -f "./consensus" ]; then
    echo "Building consensus binary..."
    go build -o consensus ./cmd/consensus
fi

echo "Starting server on :$PORT..."
./consensus serve --db "$DB_URL" --listen ":$PORT"
