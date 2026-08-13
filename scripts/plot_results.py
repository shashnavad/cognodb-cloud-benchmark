import json
import os
import sys
import matplotlib.pyplot as plt

RESULTS_PATH = "results/results.json"
OUTPUT_DIR = "docs/img"


def load_results():
    if not os.path.exists(RESULTS_PATH):
        print(f"[plot_results] Error: Results file not found at {RESULTS_PATH}")
        sys.exit(1)

    with open(RESULTS_PATH, "r") as f:
        return json.load(f)


def plot_latency_by_workload(data):
    workloads = [
        k for k in data.keys() if isinstance(data[k], dict) and "p50_us" in data[k]
    ]

    if not workloads:
        print("[plot_results] No workload data found to plot.")
        return

    p50_ms = [data[w]["p50_us"] / 1000.0 for w in workloads]
    p95_ms = [data[w]["p95_us"] / 1000.0 for w in workloads]

    x = range(len(workloads))
    width = 0.35

    fig, ax = plt.subplots(figsize=(10, 6))

    bars1 = ax.bar(
        [i - width / 2 for i in x],
        p50_ms,
        width,
        label="p50 Latency (ms)",
        color="#2b5c8f",
    )
    bars2 = ax.bar(
        [i + width / 2 for i in x],
        p95_ms,
        width,
        label="p95 Latency (ms)",
        color="#d95f02",
    )

    ax.set_ylabel("Latency (ms)", fontsize=12)
    ax.set_title("CognoDB Cloud Benchmark — Query Latency Profile", fontsize=14, pad=15)
    ax.set_xticks(list(x))
    ax.set_xticklabels(workloads, rotation=15, ha="right", fontsize=10)
    ax.legend(fontsize=11)
    ax.grid(axis="y", linestyle="--", alpha=0.5)

    # Label values on top of bars
    for bar in bars1:
        height = bar.get_height()
        ax.annotate(
            f"{height:.1f}",
            xy=(bar.get_x() + bar.get_width() / 2, height),
            xytext=(0, 3),
            textcoords="offset points",
            ha="center",
            va="bottom",
            fontsize=8,
        )

    for bar in bars2:
        height = bar.get_height()
        ax.annotate(
            f"{height:.1f}",
            xy=(bar.get_x() + bar.get_width() / 2, height),
            xytext=(0, 3),
            textcoords="offset points",
            ha="center",
            va="bottom",
            fontsize=8,
        )

    plt.tight_layout()
    out_path = os.path.join(OUTPUT_DIR, "latency_by_workload.png")
    plt.savefig(out_path, dpi=300)
    plt.close()
    print(f"[plot_results] Generated chart -> {out_path}")


def plot_hop_depth_latency(data):
    traversals = ["traversal_1", "traversal_2", "traversal_3"]
    if not all(t in data for t in traversals):
        return

    hops = [1, 2, 3]
    p50_ms = [data[t]["p50_us"] / 1000.0 for t in traversals]
    p95_ms = [data[t]["p95_us"] / 1000.0 for t in traversals]

    fig, ax = plt.subplots(figsize=(8, 5))

    ax.plot(
        hops,
        p50_ms,
        marker="o",
        linewidth=2,
        color="#2b5c8f",
        label="p50 Latency (ms)",
    )
    ax.plot(
        hops,
        p95_ms,
        marker="s",
        linewidth=2,
        linestyle="--",
        color="#d95f02",
        label="p95 Latency (ms)",
    )

    ax.set_xlabel("Hop Depth", fontsize=12)
    ax.set_ylabel("Latency (ms)", fontsize=12)
    ax.set_title("Graph Traversal Latency Scaling by Hop Depth", fontsize=14, pad=15)
    ax.set_xticks(hops)
    ax.legend(fontsize=11)
    ax.grid(True, linestyle="--", alpha=0.5)

    plt.tight_layout()
    out_path = os.path.join(OUTPUT_DIR, "latency_by_hop_depth.png")
    plt.savefig(out_path, dpi=300)
    plt.close()
    print(f"[plot_results] Generated chart -> {out_path}")


def main():
    os.makedirs(OUTPUT_DIR, exist_ok=True)
    data = load_results()
    plot_latency_by_workload(data)
    plot_hop_depth_latency(data)


if __name__ == "__main__":
    main()