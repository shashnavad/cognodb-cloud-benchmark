import argparse
import json
import os
import io
import zipfile
import requests

DEFAULT_SNAP_URL = "https://snap.stanford.edu/data/git_web_ml.zip"
RAW_DIR = "data/raw"
BATCH_DIR = "data/batches"
NODES_FILE = "data/nodes.jsonl"

def download_and_extract_snap(url, output_dir):
    os.makedirs(output_dir, exist_ok=True)
    target_csv = os.path.join(output_dir, "musae_git_edges.csv")
    
    if os.path.exists(target_csv):
        print(f"Dataset already exists at {target_csv}, skipping download.")
        return target_csv

    print(f"Downloading dataset from {url}...")
    r = requests.get(url, stream=True)
    r.raise_for_status()

    print("Extracting zip archive...")
    with zipfile.ZipFile(io.BytesIO(r.content)) as z:
        for file_info in z.infolist():
            if file_info.filename.endswith("musae_git_edges.csv"):
                file_info.filename = "musae_git_edges.csv"
                z.extract(file_info, output_dir)
                print(f"Extracted -> {target_csv}")
                return target_csv

    raise FileNotFoundError("musae_git_edges.csv not found inside zip archive")

def prepare_batches(csv_path, batch_size=5000):
    os.makedirs(BATCH_DIR, exist_ok=True)
    os.makedirs(os.path.dirname(NODES_FILE), exist_ok=True)

    nodes = set()
    edges = []

    print(f"Parsing edges from {csv_path}...")
    with open(csv_path, "r") as f:
        _ = f.readline()  # Skip header
        for line in f:
            parts = line.strip().split(",")
            if len(parts) >= 2:
                u, v = parts[0].strip(), parts[1].strip()
                nodes.add(u)
                nodes.add(v)
                edges.append((u, v))

    print(f"Total Nodes: {len(nodes)}, Total Edges: {len(edges)}")

    # Write nodes.jsonl for query sampling
    with open(NODES_FILE, "w") as f:
        for node_id in nodes:
            f.write(json.dumps({"id": node_id}) + "\n")

    # Generate node batches as raw arrays
    node_list = list(nodes)
    node_batch_idx = 0
    for i in range(0, len(node_list), batch_size):
        batch = [{"id": nid, "label": "User"} for nid in node_list[i:i + batch_size]]
        batch_path = os.path.join(BATCH_DIR, f"nodes_batch_{node_batch_idx:03d}.json")
        with open(batch_path, "w") as f:
            json.dump(batch, f)  # Raw array output
        node_batch_idx += 1

    # Generate edge batches as raw arrays
    rel_batch_idx = 0
    for i in range(0, len(edges), batch_size):
        batch = [{"from": u, "to": v, "type": "MUTUAL_FOLLOW"} for u, v in edges[i:i + batch_size]]
        batch_path = os.path.join(BATCH_DIR, f"rels_batch_{rel_batch_idx:03d}.json")
        with open(batch_path, "w") as f:
            json.dump(batch, f)  # Raw array output
        rel_batch_idx += 1

    print(f"Saved {node_batch_idx} node batches and {rel_batch_idx} relationship batches into {BATCH_DIR}")

def main():
    parser = argparse.ArgumentParser(description="Download & prepare SNAP dataset")
    parser.add_argument("--source-url", default=DEFAULT_SNAP_URL, help="SNAP dataset URL")
    args = parser.parse_args()

    csv_path = download_and_extract_snap(args.source_url, RAW_DIR)
    prepare_batches(csv_path)

if __name__ == "__main__":
    main()