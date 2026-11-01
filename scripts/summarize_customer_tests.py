#!/usr/bin/env python3
"""Summarize actual observations; empty rows never become customer evidence."""
import argparse
import csv
import math
import re
import statistics
from pathlib import Path

TASKS = {
    "T1": "Account setup and purchase entry",
    "T2": "Find and correct a purchase",
    "T3": "Import and review bank history",
    "T4": "Understand spending and transfers",
    "T5": "Compare posted balances",
    "T6": "Back up and recover a workspace",
}
OUTCOMES = {"unaided", "assisted", "failed", "skipped"}


def observations(path):
    rows, seen = [], set()
    with Path(path).open(newline="", encoding="utf-8-sig") as stream:
        reader = csv.DictReader(stream)
        if not {"participant_id", "task_id", "outcome"}.issubset(reader.fieldnames or []):
            raise ValueError("Expected participant_id, task_id, and outcome columns")
        for line, raw in enumerate(reader, 2):
            row = {key: (value or "").strip() for key, value in raw.items() if key}
            person, task = row.get("participant_id", ""), row.get("task_id", "")
            if not re.fullmatch(r"[A-Za-z0-9_-]{1,32}", person) or task not in TASKS:
                raise ValueError(f"Line {line}: use an anonymous participant ID and T1–T6")
            pair = person, task
            if pair in seen:
                raise ValueError(f"Line {line}: duplicate participant/task row")
            seen.add(pair)
            outcome = row.get("outcome", "").lower()
            if not outcome:
                continue
            if outcome not in OUTCOMES:
                raise ValueError(f"Line {line}: outcome must be unaided, assisted, failed, or skipped")
            row["outcome"] = outcome
            duration = row.get("duration_seconds", "")
            if duration:
                value = float(duration)
                if not math.isfinite(value) or value < 0:
                    raise ValueError(f"Line {line}: duration must be a finite nonnegative number")
                row["duration"] = value
            assistance = row.get("assistance_count", "")
            if assistance:
                count = int(assistance)
                if count < 0 or (outcome == "unaided" and count):
                    raise ValueError(f"Line {line}: unaided tasks cannot include assistance")
            rows.append(row)
    return rows


def escape(value):
    return str(value).replace("&", "&amp;").replace("<", "&lt;").replace(
        ">", "&gt;"
    ).replace("|", "\\|").replace("\n", " ")


def report(rows):
    participants = {row["participant_id"] for row in rows}
    lines = ["# Customer test status", "", f"Observed participants: **{len(participants)}**. Target: at least five.", ""]
    if not rows:
        lines += ["No customer sessions have been recorded. Blank template rows are unobserved; there are no customer findings yet.", ""]
    lines += ["| Task | Attempted | Unaided | Assisted | Failed | Skipped | Median measured seconds |", "| --- | ---: | ---: | ---: | ---: | ---: | ---: |"]
    priorities = []
    for task, name in TASKS.items():
        matching = [row for row in rows if row["task_id"] == task]
        counts = {outcome: sum(row["outcome"] == outcome for row in matching) for outcome in OUTCOMES}
        measured = [row["duration"] for row in matching if "duration" in row and row["outcome"] != "skipped"]
        median = f"{statistics.median(measured):g} (n={len(measured)})" if measured else "Not measured"
        attempted = sum(counts[outcome] for outcome in ("unaided", "assisted", "failed"))
        lines.append(f"| {task}: {name} | {attempted} | {counts['unaided']} | {counts['assisted']} | {counts['failed']} | {counts['skipped']} | {median} |")
        if counts["failed"] or counts["assisted"]:
            priorities.append((counts["failed"], counts["assisted"], task, name))
    lines += ["", "## Tasks to investigate", ""]
    if priorities:
        for failed, assisted, task, name in sorted(priorities, reverse=True):
            lines.append(f"- {task}: {name} — {failed} failed, {assisted} assisted. Review the observations before choosing a fix.")
    else:
        lines.append("No failures or assistance recorded. This is not evidence of success when tasks remain unobserved.")
    lines += ["", "## Recorded observations", ""]
    found = False
    for row in rows:
        notes = [f"{key.replace('_', ' ')}: {escape(row[key])}" for key in ("errors", "distraction", "evidence_quote", "suggested_improvement") if row.get(key)]
        if notes:
            found = True
            lines.append(f"- {escape(row['participant_id'])}, {row['task_id']} ({row['outcome']}): " + "; ".join(notes))
    if not found:
        lines.append("No qualitative observations recorded.")
    lines += ["", "This is a small, directional study. Automated scenarios are separate from customer observations. Measure return behavior and compare with the participant's previous method before claiming demand or superiority.", ""]
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("csv")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    try:
        result = report(observations(args.csv))
    except (ValueError, OSError) as error:
        parser.error(str(error))
    if args.output:
        args.output.write_text(result, encoding="utf-8")
    else:
        print(result)


if __name__ == "__main__":
    main()
