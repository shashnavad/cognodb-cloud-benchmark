import json
import os
import sys
import matplotlib.pyplot as plt
import numpy as np

RESULTS_PATH = "results/results.json"
OUTPUT_DIR = "docs/img"

def load_results():
    if not os.path.exists(RESULTS_PATH):
        print(f"[plot_results] Error: Results file not found at {RESULTS_PATH}")
        sys.exit(1)
    with open(RESULTS_PATH, "r") as f:
        data = json.load(f)
    # Sanitize top-level keys
    return {k: v for k, v in data.items() if isinstance(v, dict) and any(isinstance(sub, dict) for sub in v.values())}

def plot_latency_comparison(data):
    platforms = list(data.keys())
    workloads = ["point", "traversal_1", "traversal_2", "traversal_3", "aggregation"]
    
    x = np.arange(len(workloads))
    width = 0.8 / max(len(platforms), 1)

    fig, ax = plt.subplots(figsize=(12, 6))

    for idx, platform in enumerate(platforms):
        p50_vals = []
        for w in workloads:
            w_data = data[platform].get(w)
            val = w_data.get("p50_us", 0) / 1000.0 if isinstance(w_data, dict) else 0
            p50_vals.append(val)
            
        offset = x + (idx * width) - (len(platforms) * width / 2) + (width / 2)
        ax.bar(offset, p50_vals, width, label=f"{platform.upper()} (p50)")

    ax.set_ylabel("Latency (ms)", fontsize=12)
    ax.set_title("Multi-Platform Graph Workload Comparison (p50)", fontsize=14, pad=15)
    ax.set_xticks(x)
    ax.set_xticklabels(workloads, rotation=15, ha="right", fontsize=10)
    ax.legend(fontsize=10)
    ax.grid(axis="y", linestyle="--", alpha=0.5)

    plt.tight_layout()
    out_path = os.path.join(OUTPUT_DIR, "latency_by_workload.png")
    plt.savefig(out_path, dpi=300)
    plt.close()
    print(f"[plot_results] Generated multi-platform chart -> {out_path}")

def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    data = load_results()
    plot_latency_comparison(data)

if __name__ == "__main__":
    main()