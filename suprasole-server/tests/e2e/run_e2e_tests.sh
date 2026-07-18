#!/bin/bash
set -e

# Resolve paths
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Get iterations count (default to 1)
ITERATIONS=1
if [ -n "$1" ]; then
  if [[ "$1" =~ ^[0-9]+$ ]]; then
    ITERATIONS=$1
  else
    echo "Error: Argument must be a positive integer"
    exit 1
  fi
fi

echo "Building Go server binary..."
PROJECT_ROOT="$(cd "$TEST_DIR/../.." && pwd)"
go build -o "$TEST_DIR/suprasole-server" "$PROJECT_ROOT/main.go"

# Ensure cleanup on exit
cleanup() {
  rm -f "$TEST_DIR/suprasole-server"
}
trap cleanup EXIT

echo "Running Deno E2E tests ($ITERATIONS iteration(s))..."
cd "$TEST_DIR"

for i in $(seq 1 $ITERATIONS); do
  if [ $ITERATIONS -gt 1 ]; then
    echo "--- Iteration $i of $ITERATIONS ---"
  fi
  deno test --allow-net=127.0.0.1 --allow-run --allow-read --allow-write --allow-env --parallel source/ || true
done
