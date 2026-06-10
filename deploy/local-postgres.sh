#!/bin/bash
# Conscience — Local Postgres Deployment
# axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-2/step-1-2-2
set -euo pipefail

DB_URL="${1:-postgres://postgres:password@localhost:5432/conscience}"
PORT="${2:-8090}"

echo "=== Conscience Local (Postgres) ==="
echo "Database: $DB_URL"
echo "Port:     $PORT"
echo ""

# Build if binary doesn't exist
if [ ! -f "./conscience" ]; then
    echo "Building conscience binary..."
    go build -o conscience ./cmd/conscience
fi

echo "Starting server on :$PORT..."
./conscience serve --db "$DB_URL" --listen ":$PORT"
