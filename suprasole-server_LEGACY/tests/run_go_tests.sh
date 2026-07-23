#!/bin/bash
set -e

# Resolve paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Running all Go tests (unit and integration)..."
cd "$PROJECT_ROOT"
go test -v -count=1 ./source/... ./tests/integration/...
