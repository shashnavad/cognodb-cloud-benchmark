#!/usr/bin/env bash
set -euo pipefail

# Always execute relative to the repository root directory
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

echo "=========================================================="
echo " 🚀 CognoDB Cloud Benchmark Suite — End-to-End Execution  "
echo "=========================================================="

# 1. Load Environment Variables
if [ -f .env ]; then
  echo "--> [1/5] Loading environment variables from .env..."
  set -a
  source .env
  set +a
else
  echo "⚠️  [1/5] Warning: .env file not found."
  echo "    Ensure credentials (BOLT_COGNODB_URI, etc.) are exported in your environment."
fi

# 2. Spin Up Local Target Containers
echo "--> [2/5] Booting local database containers (Memgraph, FalkorDB) with resource caps..."
docker compose up -d

echo "    Waiting 5 seconds for local containers to settle..."
sleep 5

# 3. Dataset ETL
echo "--> [3/5] Checking and preparing SNAP dataset..."
if [ ! -f data/raw/git_web_ml/musae_git_edges.csv ]; then
  echo "    Downloading SNAP git_web_ml.zip..."
  mkdir -p data/raw
  curl -L -s -o data/raw/git_web_ml.zip https://snap.stanford.edu/data/git_web_ml.zip
  unzip -q -o data/raw/git_web_ml.zip -d data/raw/
fi

python3 data/download_snap.py --source-path data/raw/git_web_ml/musae_git_edges.csv

# 4. Execute Benchmark Engine
echo "--> [4/5] Running Go Benchmark Engine across all targets..."
(cd harness && go run . --ingest --measure)

# 5. Generate Plots & Markdown Reports
echo "--> [5/5] Generating visual charts and updating README report..."
python3 scripts/plot_results.py
python3 scripts/generate_report.py

echo "=========================================================="
echo " 🎉 Benchmark suite finished! Check results in README.md  "
echo "=========================================================="