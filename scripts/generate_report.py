import json
import os
import re

RESULTS_FILE = "results/results.json"
README_FILE = "README.md"
START_MARKER = "<!-- BENCHMARK_RESULTS_START -->"
END_MARKER = "<!-- BENCHMARK_RESULTS_END -->"

def generate_markdown_table(data):
    # Filter out top-level invalid keys (e.g. orphan "point", "aggregation" null maps)
    platforms = [k for k, v in data.items() if isinstance(v, dict) and any(isinstance(sub, dict) for sub in v.values())]
    workloads = ["point", "traversal_1", "traversal_2", "traversal_3", "aggregation"]

    headers = ["Workload"] + [f"{p.upper()} p50 (ms)" for p in platforms] + [f"{p.upper()} p95 (ms)" for p in platforms]
    lines = [
        "| " + " | ".join(headers) + " |",
        "|" + "|".join([":---"] * len(headers)) + "|"
    ]

    for w in workloads:
        row = [f"`{w}`"]
        # Add p50s
        for p in platforms:
            val = data[p].get(w, {}).get("p50_us", 0) / 1000.0 if isinstance(data[p].get(w), dict) else 0
            row.append(f"**{val:.2f} ms**" if val > 0 else "N/A")
        # Add p95s
        for p in platforms:
            val = data[p].get(w, {}).get("p95_us", 0) / 1000.0 if isinstance(data[p].get(w), dict) else 0
            row.append(f"**{val:.2f} ms**" if val > 0 else "N/A")

        lines.append("| " + " | ".join(row) + " |")

    return "\n".join(lines)

def main():
    if not os.path.exists(RESULTS_FILE):
        return

    with open(RESULTS_FILE, "r") as f:
        data = json.load(f)

    table_md = generate_markdown_table(data)
    block = f"{START_MARKER}\n{table_md}\n{END_MARKER}"

    if os.path.exists(README_FILE):
        with open(README_FILE, "r") as f:
            content = f.read()

        if START_MARKER in content and END_MARKER in content:
            pattern = re.compile(f"{re.escape(START_MARKER)}.*?{re.escape(END_MARKER)}", re.DOTALL)
            content = pattern.sub(block, content)
            with open(README_FILE, "w") as f:
                f.write(content)
            print("Successfully updated README.md with comparative matrix!")

if __name__ == "__main__":
    main()