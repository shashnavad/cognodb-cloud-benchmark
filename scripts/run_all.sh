#!/usr/bin/env bash
set -e

echo "=========================================================="
echo " 🚀 CognoDB Cloud Benchmark Suite — End-to-End Execution  "
echo "=========================================================="

# 1. Load Environment Variables
if [ -f .env ]; then
  echo "--> [1/5] Loading environment variables from .env..."
  export $(grep -v '^#' .env | xargs)
fi

# 2. Boot Local Containers
echo "--> [2/5] Booting local database containers..."
docker compose up -d
echo "    Waiting 15 seconds for local containers to settle..."
sleep 15

# 3. Prepare SNAP Dataset
echo "--> [3/5] Checking and preparing SNAP dataset..."
python3 data/download_snap.py

# 4. Run Go Benchmark Engine across active targets
echo "--> [4/5] Running Go Benchmark Engine across targets..."

TARGETS=("memgraph" "falkordb" "arcadedb")

# Include CognoDB if environment variables exist
if [ -n "$BOLT_COGNODB_URI" ]; then
  TARGETS+=("cognodb")
fi

# Include Neo4j if environment variables exist
if [ -n "$NEO4J_URI" ]; then
  TARGETS+=("neo4j")
fi

cd harness
for target in "${TARGETS[@]}"; do
  echo "----------------------------------------------------------"
  echo " Running benchmark for target: $target"
  echo "----------------------------------------------------------"
  go run . --target="$target" --ingest --measure --iterations=100 || echo "Warning: $target failed to run."
done
cd ..

# 5. Generate Visual Charts & Update Report
echo "--> [5/5] Generating visual charts and updating README report..."
python3 scripts/plot_results.py
python3 scripts/generate_report.py

echo "=========================================================="
echo " 🎉 Benchmark suite finished! Check results in README.md  "
echo "=========================================================="