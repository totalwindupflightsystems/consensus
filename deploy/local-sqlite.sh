#!/bin/bash
# Conscience — Local SQLite Deployment
# axiom:trace work_item=deployment-ops-01 spec=specs/009-deployment.md plan=phase-1/task-1-2/step-1-2-2
set -euo pipefail

DB_FILE="${1:-conscience.db}"
PORT="${2:-8090}"

echo "=== Conscience Local (SQLite) ==="
echo "Database: $DB_FILE"
echo "Port:     $PORT"
echo ""

# Build if binary doesn't exist
if [ ! -f "./conscience" ]; then
    echo "Building conscience binary..."
    go build -o conscience ./cmd/conscience
fi

# Initialize if database doesn't exist
if [ ! -f "$DB_FILE" ]; then
    echo "Initializing new database..."
    ./conscience init --db "sqlite://$DB_FILE"
fi

echo "Starting server on :$PORT..."
./conscience serve --db "sqlite://$DB_FILE" --listen ":$PORT"
