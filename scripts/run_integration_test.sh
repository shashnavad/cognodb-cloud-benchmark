#!/usr/bin/env bash
set -euo pipefail

# Navigate to project root
cd "$(dirname "$0")/.." || exit 1

# Load .env file automatically if present
if [ -f .env ]; then
  echo "Loading environment variables from .env..."
  set -a
  source .env
  set +a
fi

pushd harness >/dev/null
echo "Building integration binary..."
mkdir -p bin
go build -o bin/integration ./cmd/integration
popd >/dev/null

echo "Running integration test (FalkorDB and ArcadeDB)..."
./harness/bin/integration

echo "Integration test finished. Check output above for any errors."