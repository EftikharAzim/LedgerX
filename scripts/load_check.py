#!/usr/bin/env python3
"""Free API load/correctness baseline. Uses synthetic workspaces, never logs tokens.

Creates lasting test records: use only an isolated stack and pass
--confirm-synthetic. Reported user levels are workloads, not a capacity promise.
"""
import argparse
import concurrent.futures
import csv
import io
import json
import math
import os
from pathlib import Path
import platform
import secrets
import subprocess
import threading
import time
from urllib import error, parse, request
import uuid


class Client:
    def __init__(self, base):
        self.base = base
        self.access = ""
        self.account = None
        self.expected = 1_000_000
        self.writes = 0

    def call(self, method, path, body=None, key=None, retry_rate_limit=False):
        data = None if body is None else json.dumps(body).encode()
        headers = {"Content-Type": "application/json"}
        if self.access:
            headers["Authorization"] = "Bearer " + self.access
        if key:
            headers["Idempotency-Key"] = key
        for attempt in range(10 if retry_rate_limit else 1):
            start = time.perf_counter()
            req = request.Request(self.base + path, data=data, headers=headers, method=method)
            try:
                with request.urlopen(req, timeout=20) as response:
                    status, raw = response.status, response.read()
            except error.HTTPError as exc:
                status, raw = exc.code, exc.read()
            except (OSError, TimeoutError):
                status, raw = 0, b""
            elapsed = (time.perf_counter() - start) * 1000
            if status != 429 or not retry_rate_limit or attempt == 9:
                try:
                    value = json.loads(raw)
                except (ValueError, UnicodeError):
                    value = raw
                return status, value, elapsed
            time.sleep(1.1)
        raise RuntimeError("unreachable")

    def expect(self, method, path, body=None):
        status, value, _ = self.call(method, path, body, retry_rate_limit=True)
        if status not in (200, 201, 202, 204):
            raise RuntimeError(f"{method} {path}: HTTP {status}")
        return value


def seed(base, rows, index, run_id):
    client = Client(base)
    tokens = client.expect("POST", "/auth/register", {
        "email": f"load-{run_id}-{index}@example.com", "password": secrets.token_hex(18),
    })
    client.access = tokens["access_token"]
    account = client.expect("POST", "/v1/accounts", {
        "name": "Synthetic everyday", "currency": "USD",
        "opening_balance_minor": client.expected, "opening_balance_at": "2024-01-01T00:00:00Z",
    })
    client.account = account["id"]
    for offset in range(0, rows, 1000):
        text = io.StringIO()
        writer = csv.writer(text)
        writer.writerow(["Date", "Amount", "Note", "Category", "Reference"])
        for row in range(offset, min(rows, offset+1000)):
            writer.writerow(["2026-01-02", "-1.00", f"Synthetic purchase {row}", "Everyday", f"load-{run_id}-{index}-{row}"])
        preview = client.expect("POST", "/v1/imports/preview", {
            "account_id": client.account, "filename": "synthetic-load.csv", "csv": text.getvalue(), "delimiter": "comma",
            "mapping": {"date": 0, "amount": 1, "note": 2, "category": 3, "reference": 4, "amount_mode": "signed", "date_format": "iso", "number_style": "dot", "sign_convention": "income_positive"},
            "starting_balance": {"mode": "existing", "amount_minor": 0, "date": ""},
        })
        plan = preview["preview"]
        if plan["invalid_count"] or plan["duplicate_count"] or plan.get("plan_errors"):
            raise RuntimeError("synthetic seed did not pass import review")
        receipt = client.expect("POST", f'/v1/imports/{preview["id"]}/commit', {"rows": [row["number"] for row in plan["rows"]], "duplicate_approvals": []})
        if receipt["imported_count"] != min(1000, rows-offset):
            raise RuntimeError("synthetic seed import count mismatch")
        client.expected -= receipt["imported_count"] * 100
    return client


def percentile(values, fraction):
    values = sorted(values)
    return round(values[max(0, math.ceil(len(values)*fraction)-1)], 2) if values else None


def user_loop(client, seconds, pause):
    records = []
    deadline = time.monotonic() + seconds
    step = 0
    while time.monotonic() < deadline:
        choice = step % 5
        if choice == 0:
            method, path, label = "GET", "/v1/accounts", "accounts"
        elif choice == 1:
            method, path, label = "GET", "/v1/spending?type=all&currency=USD", "spending"
        elif choice == 2:
            method, path, label = "GET", "/v1/activity?currency=USD&limit=25", "activity"
        elif choice == 3:
            method, path, label = "GET", f"/v1/accounts/{client.account}/balance", "balance"
        else:
            body = {"account_id": client.account, "amount_minor": -125, "currency": "USD", "occurred_at": "2026-01-03T12:00:00Z", "note": "Synthetic load purchase"}
            key = str(uuid.uuid4())
            status, first, ms = client.call("POST", "/v1/transactions", body, key)
            records.append(("write", status, ms))
            if status in (200, 201):
                client.expected -= 125
                client.writes += 1
                status, repeated, ms = client.call("POST", "/v1/transactions", body, key)
                records.append(("write_retry", status, ms))
                if status in (200, 201) and repeated.get("id") != first.get("id"):
                    raise RuntimeError("idempotent retry changed transaction identity")
            step += 1
            time.sleep(pause)
            continue
        status, _, ms = client.call(method, path)
        records.append((label, status, ms))
        step += 1
        time.sleep(pause)
    return records


def export_history(client, minimum_rows, maximum_rows):
    export = client.expect("POST", "/v1/exports?type=all")
    for _ in range(120):
        status = client.expect("GET", f'/v1/exports/{export["id"]}/status')
        if status["status"] == "done":
            data = client.expect("GET", f'/v1/exports/{export["id"]}/download')
            # CSV has a header and includes the explicit opening-balance record.
            count = sum(1 for _ in csv.reader(io.StringIO(data.decode()))) - 1
            if not minimum_rows <= count <= maximum_rows():
                raise RuntimeError("export record count does not match the ledger")
            return count
        if status["status"] == "error":
            raise RuntimeError("background export failed")
        time.sleep(0.5)
    raise RuntimeError("background export did not complete")


def metrics_snapshot(base, token):
    if not token:
        return {"available": False}
    req = request.Request(base + "/metrics", headers={"Authorization": "Bearer " + token})
    with request.urlopen(req, timeout=10) as response:
        text = response.read().decode()
    selected = {}
    for line in text.splitlines():
        if line.startswith(("db_pool_", "process_resident_memory_bytes ", "go_goroutines ", "ledgerx_queue_")):
            key, value = line.rsplit(" ", 1)
            selected[key] = float(value)
    return selected


def docker_snapshot(project):
    if not project:
        return None
    result = subprocess.run(["docker", "ps", "--filter", "label=com.docker.compose.project="+project, "--format", "{{.ID}}"], check=True, text=True, capture_output=True)
    ids = result.stdout.split()
    if not ids:
        raise RuntimeError("no containers matched the requested project")
    result = subprocess.run(["docker", "stats", "--no-stream", "--format", "{{json .}}", *ids], check=True, text=True, capture_output=True)
    return [json.loads(line) for line in result.stdout.splitlines()]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--confirm-synthetic", action="store_true")
    parser.add_argument("--users", type=int, default=25)
    parser.add_argument("--rows", type=int, default=1000)
    parser.add_argument("--levels", default="1,5,10,25")
    parser.add_argument("--seconds", type=int, default=15)
    parser.add_argument("--pause", type=float, default=0.4)
    parser.add_argument("--max-p95-ms", type=float, default=500, help="Initial engineering latency budget; not an industry benchmark")
    parser.add_argument("--metrics-env-file", help="Local app configuration; only METRICS_TOKEN is read and never logged")
    parser.add_argument("--docker-project", help="Optional local container resource snapshots")
    parser.add_argument("--output", default="tmp/load-check.json")
    args = parser.parse_args()
    base = args.base_url.rstrip("/")
    url = parse.urlsplit(base)
    if not args.confirm_synthetic or url.scheme not in ("http", "https") or not url.netloc or url.username or url.password:
        parser.error("provide a valid app URL without credentials and confirm that it is an isolated synthetic stack")
    levels = [int(level) for level in args.levels.split(",")]
    if not (1 <= args.users <= 100 and 0 <= args.rows <= 50000 and 5 <= args.seconds <= 300 and args.pause >= 0.25 and args.max_p95_ms > 0 and levels and all(1 <= level <= args.users for level in levels)):
        parser.error("use 1–100 users, 0–50,000 rows, 5–300 seconds, pause >= 0.25, and valid user levels")
    metrics_token = os.environ.get("METRICS_TOKEN", "")
    if args.metrics_env_file:
        for line in Path(args.metrics_env_file).read_text().splitlines():
            if line.startswith("METRICS_TOKEN="):
                metrics_token = line.partition("=")[2].strip()
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    result = {
        "status": "running", "synthetic": True,
        "configuration": {"users": args.users, "history_per_user": args.rows, "levels": levels, "seconds_per_level": args.seconds, "pause_seconds": args.pause, "p95_budget_ms": args.max_p95_ms},
        "host": {"platform": platform.platform(), "cpu_count": os.cpu_count(), "python": platform.python_version(), "cgroup_cpu_max": Path('/sys/fs/cgroup/cpu.max').read_text().strip() if Path('/sys/fs/cgroup/cpu.max').exists() else None, "cgroup_memory_max": Path('/sys/fs/cgroup/memory.max').read_text().strip() if Path('/sys/fs/cgroup/memory.max').exists() else None},
        "levels": [], "limitations": ["API baseline; excludes real-device rendering and hosted network latency", "Resource snapshots are samples, not peak measurements", "Passing levels are not a supported-user promise"],
    }
    clients = []
    sampler_stop = threading.Event()
    try:
        run_id = uuid.uuid4().hex[:12]
        for index in range(args.users):
            clients.append(seed(base, args.rows, index, run_id))
            print(f"Seeded synthetic workspace {index+1}/{args.users} ({args.rows} history rows)", flush=True)
        for users in levels:
            metrics = []
            def sample_metrics():
                while not sampler_stop.is_set():
                    try:
                        metrics.append(metrics_snapshot(base, metrics_token))
                    except (OSError, ValueError):
                        metrics.append({"available": False})
                    sampler_stop.wait(1)
            sampler_stop.clear()
            sampler = threading.Thread(target=sample_metrics, daemon=True)
            sampler.start()
            before = docker_snapshot(args.docker_project)
            start = time.monotonic()
            with concurrent.futures.ThreadPoolExecutor(max_workers=users+1) as executor:
                work = [executor.submit(user_loop, client, args.seconds, args.pause) for client in clients[:users]]
                first = clients[0]
                background = executor.submit(export_history, first, args.rows + 1 + first.writes, lambda: args.rows + 1 + first.writes)
                records = [record for future in work for record in future.result()]
                export_count = background.result()
            elapsed = time.monotonic() - start
            sampler_stop.set(); sampler.join(timeout=12)
            rate_limits = sum(status == 429 for _, status, _ in records)
            unexpected = sum(not 200 <= status < 300 and status != 429 for _, status, _ in records)
            timings = [ms for _, status, ms in records if 200 <= status < 300]
            entry = {
                "simultaneous_users": users, "elapsed_seconds": round(elapsed, 2), "requests": len(records),
                "successful_requests_per_second": round(len(timings)/elapsed, 2),
                "p50_ms": percentile(timings, 0.5), "p95_ms": percentile(timings, 0.95),
                "rate_limited_requests": rate_limits, "unexpected_errors": unexpected,
                "background_export_rows": export_count, "metrics_samples": metrics,
                "resources_before": before, "resources_after": docker_snapshot(args.docker_project),
            }
            entry["endpoints"] = {}
            for label in sorted({label for label, _, _ in records}):
                latencies = [ms for name, status, ms in records if name == label and 200 <= status < 300]
                entry["endpoints"][label] = {"successful_requests": len(latencies), "p50_ms": percentile(latencies, 0.5), "p95_ms": percentile(latencies, 0.95)}
            result["levels"].append(entry)
            print(json.dumps({key: value for key, value in entry.items() if key not in ("metrics_samples", "resources_before", "resources_after")}), flush=True)
            if unexpected or rate_limits:
                result["workload_failure"] = "workload had errors or throttling; inspect the report before increasing load"
                break
            if not timings or entry["p95_ms"] > args.max_p95_ms:
                result["workload_failure"] = "workload exceeded the selected p95 latency budget"
                break
        for client in clients:
            balance = client.expect("GET", f"/v1/accounts/{client.account}/balance")
            if balance["balance_minor"] != client.expected:
                raise RuntimeError("balance mismatch after concurrent writes and retries")
            spending = client.expect("GET", "/v1/spending?type=all&currency=USD")
            if spending["spending_minor"] != 1_000_000 - client.expected:
                raise RuntimeError("spending totals disagree with the ledger")
        result["verified_workspaces"] = len(clients)
        result["status"] = "failed" if result.get("workload_failure") else "passed"
    except Exception as exc:
        result["status"] = "failed"
        # Do not serialize request bodies, response bodies, or auth credentials.
        result["failure"] = str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__
    finally:
        sampler_stop.set()
        output.write_text(json.dumps(result, indent=2) + "\n")
    print(f'Result: {result["status"]}. Report: {output}', flush=True)
    return 0 if result["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
