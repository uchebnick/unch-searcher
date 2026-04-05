#!/usr/bin/env python3

from __future__ import annotations

import argparse
from pathlib import Path


def strip_markdown(text: str) -> str:
    return " ".join(text.replace("`", "").split())


def extract_title(lines: list[str], tag: str) -> str:
    for line in lines:
        if line.startswith("# "):
            return strip_markdown(line[2:].strip())
    return f"unch {tag}"


def extract_summary(lines: list[str]) -> str:
    body = lines[:]
    if body and body[0].startswith("# "):
        body = body[1:]

    paragraph: list[str] = []
    for line in body:
        stripped = line.strip()
        if not stripped:
            if paragraph:
                break
            continue
        if stripped.startswith("## "):
            break
        paragraph.append(stripped)

    return strip_markdown(" ".join(paragraph))


def extract_section_bullets(lines: list[str], heading: str) -> list[str]:
    target = f"## {heading}"
    in_section = False
    bullets: list[str] = []

    for line in lines:
        stripped = line.strip()
        if stripped == target:
            in_section = True
            continue
        if in_section and stripped.startswith("## "):
            break
        if in_section and stripped.startswith("- "):
            bullets.append(strip_markdown(stripped[2:]))

    return bullets


def render_announcement(tag: str, url: str, notes_text: str) -> str:
    lines = notes_text.splitlines()
    title = extract_title(lines, tag)
    summary = extract_summary(lines)
    highlights = extract_section_bullets(lines, "Highlights")
    upgrades = extract_section_bullets(lines, "Upgrade notes")

    rendered: list[str] = [title]

    if summary:
        rendered.extend(["", summary])

    if highlights:
        rendered.extend(["", "Highlights:"])
        rendered.extend(f"- {item}" for item in highlights)

    if upgrades:
        rendered.extend(["", "Upgrade notes:"])
        rendered.extend(f"- {item}" for item in upgrades)

    rendered.extend(["", f"Release: {url}"])
    return "\n".join(rendered)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--tag", required=True)
    parser.add_argument("--url", required=True)
    parser.add_argument("--notes-file")
    args = parser.parse_args()

    notes_text = ""
    if args.notes_file:
        notes_path = Path(args.notes_file)
        if notes_path.exists():
            notes_text = notes_path.read_text(encoding="utf-8")

    if not notes_text:
        notes_text = f"# unch {args.tag}\n"

    print(render_announcement(args.tag, args.url, notes_text))


if __name__ == "__main__":
    main()
