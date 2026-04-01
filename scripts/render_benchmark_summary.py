#!/usr/bin/env python3

from __future__ import annotations

import json
import sys
from pathlib import Path


def format_duration_ms(value: float) -> str:
    if value >= 1000:
        return f"{value / 1000:.2f}s"
    return f"{value:.2f}ms"


def safe_top_hit(query: dict) -> str:
    runs = query.get("runs") or []
    if not runs:
        return "No hits"
    hits = runs[0].get("hits") or []
    if not hits:
        return "No hits"
    hit = hits[0]
    return f"`{hit['path']}:{hit['line']}`"


def render_summary(report: dict) -> str:
    lines: list[str] = []
    env = report["environment"]
    timing = report["timing"]
    metrics = report["metrics"]

    lines.append("## Benchmark")
    lines.append("")
    lines.append(f"- Tool: `{report['tool']}` (`{env['tool_version']}`)")
    lines.append(f"- Suite: `{report['suite']['name']}`")
    lines.append(f"- Suite revision: `{report['suite_revision']}`")
    lines.append(f"- Environment: `{env['os']}/{env['arch']}` • `{env.get('cpu_info') or 'unknown CPU'}` • `{env['num_cpu']} cores`")
    lines.append(f"- Cold index mean: `{format_duration_ms(timing['cold_index_mean_ms'])}`")
    lines.append(f"- Warm index mean: `{format_duration_ms(timing['warm_index_mean_ms'])}`")
    lines.append(f"- Warm search mean: `{format_duration_ms(timing['warm_search_mean_ms'])}`")
    lines.append(
        f"- Quality: `{metrics['quality_score']}/100` "
        f"(top1=`{metrics['top1']:.3f}` top3=`{metrics['top3']:.3f}` mrr=`{metrics['mrr']:.3f}`)"
    )
    lines.append("")
    lines.append("### Repositories")
    lines.append("")
    lines.append("| Repo | Language | Cold index | Warm index | Warm search | Score |")
    lines.append("| --- | --- | --- | --- | --- | --- |")
    for repo in report["repositories"]:
        repo_timing = repo["timing"]
        repo_metrics = repo["metrics"]
        lines.append(
            f"| `{repo['id']}` | `{repo['language']}` | "
            f"`{format_duration_ms(repo_timing['cold_index_mean_ms'])}` | "
            f"`{format_duration_ms(repo_timing['warm_index_mean_ms'])}` | "
            f"`{format_duration_ms(repo_timing['warm_search_mean_ms'])}` | "
            f"`{repo_metrics['quality_score']}` |"
        )

    misses = []
    for repo in report["repositories"]:
        for query in repo["queries"]:
            query_metrics = query["metrics"]
            if query_metrics["top1_success"]:
                continue
            misses.append((repo, query))

    lines.append("")
    lines.append("### Ranking Misses")
    lines.append("")
    if not misses:
        lines.append("All benchmark queries returned an expected exact hit at rank 1.")
    else:
        for repo, query in misses:
            query_metrics = query["metrics"]
            lines.append(
                f"- `{repo['id']}` / `{query['id']}`: expected "
                f"`{', '.join(query['expected_hits'])}`, got {safe_top_hit(query)} "
                f"(top3=`{'yes' if query_metrics['top3_success'] else 'no'}` rr=`{query_metrics['rr']:.3f}`)"
            )

    lines.append("")
    lines.append("### Artifact")
    lines.append("")
    lines.append("Machine-readable report uploaded as `benchmark-report`.")
    lines.append("")

    return "\n".join(lines)


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: render_benchmark_summary.py <report.json>", file=sys.stderr)
        return 2

    report_path = Path(sys.argv[1])
    report = json.loads(report_path.read_text())
    sys.stdout.write(render_summary(report) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
