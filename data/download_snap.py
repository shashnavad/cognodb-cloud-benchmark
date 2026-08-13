"""
Download and normalize a SNAP-style edge list into batched JSON payloads.

Behavior:
- Accepts either a `--source-url` to download a remote file or `--source-path`
  to read a local file.
- Detects gzipped inputs and CSV/TSV delimiter automatically.
- Produces:
  - data/nodes.jsonl          (one JSON node per line)
  - data/relationships.jsonl  (one JSON rel per line)
  - data/batches/nodes_batch_{i}.json  (arrays of nodes)
  - data/batches/rels_batch_{i}.json   (arrays of rels)
  - data/dataset_stats.json  (counts)

Default assumptions are conservative: input is an edge list with two columns
(src,dst). Node attributes are synthesized deterministically so loads are
reproducible.

Usage examples:
  python3 data/download_snap.py --source-url https://example.org/musae-github_edges.csv.gz
  python3 data/download_snap.py --source-path /path/to/edges.csv --batch-size 5000
"""

from __future__ import annotations

import argparse
import csv
import gzip
import json
import os
import hashlib
from pathlib import Path
from typing import Iterable, Tuple

try:
	import requests
except Exception:  # pragma: no cover - graceful fallback
	requests = None


def ensure_dir(p: Path) -> None:
	p.mkdir(parents=True, exist_ok=True)


def open_maybe_gz(path: Path):
	if str(path).endswith(".gz"):
		return gzip.open(path, "rt", encoding="utf-8")
	return open(path, "r", encoding="utf-8")


def download_file(url: str, out: Path, chunk_size: int = 8192) -> None:
	if requests is None:
		raise RuntimeError("requests is required to download files. Install with `pip install requests`.")
	with requests.get(url, stream=True, timeout=60) as r:
		r.raise_for_status()
		with open(out, "wb") as f:
			for chunk in r.iter_content(chunk_size=chunk_size):
				if chunk:
					f.write(chunk)


def detect_delimiter(sample: str) -> str:
	# Use csv.Sniffer to detect delimiter; fallback to comma
	try:
		dialect = csv.Sniffer().sniff(sample)
		return dialect.delimiter
	except Exception:
		return ","


def parse_edge_lines(lines: Iterable[str]) -> Iterable[Tuple[str, str]]:
	# Yield (src, dst) from a sequence of text lines. Skip comments and empty.
	buf = []
	for i, ln in enumerate(lines):
		if i < 20:
			buf.append(ln)
		if ln.strip() == "" or ln.lstrip().startswith("#"):
			continue
	sample = "\n".join(buf)
	delim = detect_delimiter(sample)
	reader = csv.reader(lines, delimiter=delim)
	for row in reader:
		if not row:
			continue
		if row[0].startswith("#"):
			continue
		if len(row) >= 2:
			yield row[0].strip(), row[1].strip()


def synth_node_attrs(node_id: str) -> dict:
	# Deterministic synthetic attribute: public_repos derived from hash.
	h = hashlib.sha256(node_id.encode("utf-8")).digest()
	public_repos = h[0] % 100
	return {"id": node_id, "location": "", "public_repos": int(public_repos)}


def write_jsonl(path: Path, objs: Iterable[dict]) -> None:
	with open(path, "w", encoding="utf-8") as f:
		for o in objs:
			f.write(json.dumps(o, separators=(",", ":")) + "\n")


def chunked_iterable(iterable, size):
	buf = []
	for it in iterable:
		buf.append(it)
		if len(buf) >= size:
			yield buf
			buf = []
	if buf:
		yield buf


def main() -> None:
	p = argparse.ArgumentParser()
	p.add_argument("--source-url", help="URL to download the edge list (optional)")
	p.add_argument("--source-path", help="Local path to an edge list file (optional)")
	p.add_argument("--out-dir", default="data", help="Output directory")
	p.add_argument("--batch-size", type=int, default=5000, help="Batch size for nodes/relationships")
	p.add_argument("--force", action="store_true", help="Overwrite existing outputs if present")
	args = p.parse_args()

	out_dir = Path(args.out_dir)
	raw_dir = out_dir / "raw"
	batches_dir = out_dir / "batches"
	ensure_dir(out_dir)
	ensure_dir(raw_dir)
	ensure_dir(batches_dir)

	# Determine source path
	if not args.source_path and not args.source_url:
		raise SystemExit("Provide either --source-url or --source-path")

	if args.source_url:
		fname = Path(args.source_url.split("/")[-1])
		if not fname.suffix:
			# default to .csv
			fname = fname.with_suffix('.csv')
		dest = raw_dir / fname
		if dest.exists() and not args.force:
			print(f"Using cached file {dest}")
		else:
			print(f"Downloading {args.source_url} -> {dest}")
			download_file(args.source_url, dest)
		source_path = dest
	else:
		source_path = Path(args.source_path)
		if not source_path.exists():
			raise SystemExit(f"source path not found: {source_path}")

	# Parse edges and build node set
	print(f"Parsing edges from {source_path}")
	edges = []
	nodes_set = set()
	with open_maybe_gz(source_path) as fh:
		# We'll feed the file twice; read lines into memory for simplicity (SNAP fits easily)
		lines = [l for l in fh]
	for src, dst in parse_edge_lines(lines):
		edges.append({"from": src, "to": dst, "type": "MUTUAL_FOLLOW"})
		nodes_set.add(src)
		nodes_set.add(dst)

	print(f"Found {len(nodes_set)} unique nodes and {len(edges)} relationships")

	# Synthesize nodes
	nodes = [synth_node_attrs(n) for n in sorted(nodes_set)]

	# Write full JSONL outputs
	nodes_jsonl = out_dir / "nodes.jsonl"
	rels_jsonl = out_dir / "relationships.jsonl"
	if nodes_jsonl.exists() and not args.force:
		print(f"{nodes_jsonl} exists; skipping (use --force to overwrite)")
	else:
		write_jsonl(nodes_jsonl, nodes)

	if rels_jsonl.exists() and not args.force:
		print(f"{rels_jsonl} exists; skipping (use --force to overwrite)")
	else:
		write_jsonl(rels_jsonl, edges)

	# Create batches
	batch_size = int(args.batch_size)
	for i, chunk in enumerate(chunked_iterable(nodes, batch_size), start=1):
		outp = batches_dir / f"nodes_batch_{i:04d}.json"
		with open(outp, "w", encoding="utf-8") as f:
			json.dump(chunk, f, separators=(",", ":"))

	for i, chunk in enumerate(chunked_iterable(edges, batch_size), start=1):
		outp = batches_dir / f"rels_batch_{i:04d}.json"
		with open(outp, "w", encoding="utf-8") as f:
			json.dump(chunk, f, separators=(",", ":"))

	stats = {"nodes": len(nodes), "relationships": len(edges)}
	with open(out_dir / "dataset_stats.json", "w", encoding="utf-8") as f:
		json.dump(stats, f, indent=2)

	print(f"Wrote batches to {batches_dir}; stats: {stats}")


if __name__ == "__main__":
	main()

