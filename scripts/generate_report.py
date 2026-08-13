import json
import os
import re

RESULTS_FILE = "results/results.json"
README_FILE = "README.md"
START_MARKER = "<!-- BENCHMARK_RESULTS_START -->"
END_MARKER = "<!-- BENCHMARK_RESULTS_END -->"

def generate_markdown_table(data):
    lines = [
        "| Workload | p50 Latency (ms) | p95 Latency (ms) | p50 (µs) | p95 (µs) |",
        "| :--- | :--- | :--- | :--- | :--- |"
    ]
    for q, metrics in data.items():
        p50_us = metrics.get("p50_us", 0)
        p95_us = metrics.get("p95_us", 0)
        p50_ms = p50_us / 1000.0
        p95_ms = p95_us / 1000.0
        lines.append(f"| `{q}` | **{p50_ms:.2f} ms** | **{p95_ms:.2f} ms** | {p50_us:,} µs | {p95_us:,} µs |")
    return "\n".join(lines)

def main():
    if not os.path.exists(RESULTS_FILE):
        print(f"Skipping report generation: {RESULTS_FILE} not found.")
        return

    with open(RESULTS_FILE, "r") as f:
        data = json.load(f)

    table_md = generate_markdown_table(data)
    block = f"{START_MARKER}\n{table_md}\n{END_MARKER}"

    if not os.path.exists(README_FILE):
        readme_content = f"# CognoDB Benchmark Report\n\n## Latest Execution Results\n\n{block}\n"
    else:
        with open(README_FILE, "r") as f:
            readme_content = f.read()

        # If comment markers exist in README.md, replace between them
        if START_MARKER in readme_content and END_MARKER in readme_content:
            pattern = re.compile(f"{re.escape(START_MARKER)}.*?{re.escape(END_MARKER)}", re.DOTALL)
            readme_content = pattern.sub(block, readme_content)
        else:
            # Otherwise, append a new section at the bottom
            readme_content = readme_content.rstrip() + f"\n\n## Latest Benchmark Results\n\n{block}\n"

    with open(README_FILE, "w") as f:
        f.write(readme_content)

    print(f"Successfully updated {README_FILE} with benchmark results!")

if __name__ == "__main__":
    main()