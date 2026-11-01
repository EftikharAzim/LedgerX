#!/usr/bin/env python3
"""Check container replacement and full server recovery on two fresh test stacks.

Requires Docker only (plus Python for this check). Creates and removes its own
synthetic volumes; refuses any project name that already has containers/volumes.
"""
import argparse
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import uuid

from load_check import Client

ROOT = Path(__file__).resolve().parents[1]


def command(args, env, **kwargs):
    return subprocess.run(args, cwd=ROOT, env=env, check=True, **kwargs)


def compose(env, *args):
    return ["docker", "compose", "--env-file", env["LEDGERX_APP_ENV"], "-f", "deploy/compose.app.yml", *(["-f", env["LEDGERX_COMPOSE_OVERRIDE"]] if env.get("LEDGERX_COMPOSE_OVERRIDE") else []), *args]


def ready(base):
    client = Client(base)
    for _ in range(60):
        if client.call("GET", "/readyz")[0] == 200:
            return
        time.sleep(0.5)
    raise RuntimeError("packaged app did not become ready")


def completed_export(client, export_id):
    for _ in range(60):
        if client.expect("GET", f"/v1/exports/{export_id}/status")["status"] == "done":
            return client.expect("GET", f"/v1/exports/{export_id}/download")
        time.sleep(0.5)
    raise RuntimeError("export did not finish")


def financial_balance(client, account_id):
    # The API's as_of is the read time, so it should change between requests.
    value = client.expect("GET", f"/v1/accounts/{account_id}/balance")
    return value["account_id"], value["balance_minor"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=8095)
    parser.add_argument("--output", default="tmp/package-check.json")
    args = parser.parse_args()
    if not 1024 <= args.port <= 65534:
        parser.error("choose an unprivileged port and leave the next port free")
    (ROOT / "tmp").mkdir(exist_ok=True)
    project = "ledgerx-test-package-" + uuid.uuid4().hex[:8]
    source = dict(os.environ, COMPOSE_PROJECT_NAME=project, LEDGERX_PORT=str(args.port), LEDGERX_APP_ENV=str(ROOT / "tmp" / (project+".env")), API_RATE_LIMIT_RPS="5")
    target = dict(source, COMPOSE_PROJECT_NAME=project+"-restore", LEDGERX_PORT=str(args.port+1), LEDGERX_APP_ENV=str(ROOT / "tmp" / (project+"-restore.env")))
    result = {"status": "running", "checks": []}
    owned = []
    try:
        for env in (source, target):
            for kind in ("ps", "volume"):
                cmd = ["docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+env["COMPOSE_PROJECT_NAME"]] if kind == "ps" else ["docker", "volume", "ls", "-q", "--filter", "label=com.docker.compose.project="+env["COMPOSE_PROJECT_NAME"]]
                if command(cmd, env, capture_output=True, text=True).stdout.strip():
                    raise RuntimeError("refusing to reuse an existing test project")
        owned.append(source)
        command(["./scripts/app.sh", "start"], source)
        base = f"http://127.0.0.1:{args.port}"
        ready(base)
        client = Client(base)
        status, page, _ = client.call("GET", "/accounts/123")
        if status != 200 or not isinstance(page, bytes) or b'<div id="root">' not in page:
            raise RuntimeError("deep UI route did not return the built application")
        email, password = f"package-{uuid.uuid4().hex}@example.com", secrets.token_hex(18)
        pair = client.expect("POST", "/auth/register", {"email": email, "password": password})
        client.access = pair["access_token"]
        account = client.expect("POST", "/v1/accounts", {"name": "Package check", "currency": "USD", "opening_balance_minor": 50000, "opening_balance_at": "2024-01-01T00:00:00Z"})
        client.expect("POST", "/v1/transactions", {"account_id": account["id"], "amount_minor": -450, "currency": "USD", "occurred_at": "2026-01-02T12:00:00Z", "note": "Synthetic package record"})
        export = client.expect("POST", "/v1/exports?type=all")
        original_csv = completed_export(client, export["id"])
        original_balance = financial_balance(client, account["id"])
        if original_balance != (account["id"], 49550):
            raise RuntimeError("initial financial balance is incorrect")
        command(compose(source, "up", "-d", "--no-deps", "--force-recreate", "--wait", "app"), source)
        ready(base)
        if financial_balance(client, account["id"]) != original_balance or completed_export(client, export["id"]) != original_csv:
            raise RuntimeError("container replacement lost records or an existing report")
        result["checks"].append("container replacement preserves financial records, sessions, and previous report downloads")
        before = set((ROOT / "tmp/server-backups").glob("*"))
        command(["./scripts/app.sh", "backup"], source)
        backups = set((ROOT / "tmp/server-backups").glob("*")) - before
        if len(backups) != 1:
            raise RuntimeError("backup directory was not uniquely identified")
        backup = backups.pop()
        if not (backup / "COMPLETE").is_file():
            raise RuntimeError("backup did not complete")
        ready(base)
        owned.append(target)
        command(["./scripts/app.sh", "restore", str(backup)], target)
        restored = Client(f"http://127.0.0.1:{args.port+1}")
        ready(restored.base)
        restored.access = restored.expect("POST", "/auth/login", {"email": email, "password": password})["access_token"]
        if financial_balance(restored, account["id"]) != original_balance or completed_export(restored, export["id"]) != original_csv:
            raise RuntimeError("server restore changed records or report contents")
        if restored.call("POST", "/auth/refresh", {"refresh_token": pair["refresh_token"]})[0] != 401:
            raise RuntimeError("server restore resurrected a refresh session")
        result["checks"].append("database, Redis, and export-volume backup restores into a fresh stack with old sessions revoked")
        refused = subprocess.run(["./scripts/app.sh", "restore", str(backup)], cwd=ROOT, env=target, capture_output=True, text=True)
        if refused.returncode == 0 or "destination database must be completely empty" not in refused.stderr:
            raise RuntimeError("restore did not refuse an occupied destination")
        if financial_balance(restored, account["id"]) != original_balance:
            raise RuntimeError("refused restore changed financial records")
        result["checks"].append("restore refuses a nonempty destination without altering the ledger")
        result["status"] = "passed"
    except Exception as exc:
        result["status"] = "failed"
        result["failure"] = str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__
    finally:
        for env in reversed(owned):
            if Path(env["LEDGERX_APP_ENV"]).exists():
                subprocess.run(compose(env, "down", "--volumes", "--remove-orphans"), cwd=ROOT, env=env, check=False)
        output = ROOT / args.output
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result))
    return 0 if result["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
