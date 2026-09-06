#!/usr/bin/env python3
"""Plan and apply an exact commit directly to the repository's current main branch."""

from __future__ import annotations

import hashlib
import json
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


PROJECT = Path(__file__).resolve().parent.parent
REQUEST = PROJECT / "tmp" / "release-request.json"
PLAN = PROJECT / "tmp" / "release-plan.json"


def run(arguments: list[str], *, check: bool = True) -> subprocess.CompletedProcess[str]:
    result = subprocess.run(
        arguments,
        cwd=PROJECT,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if check and result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or f"command failed: {arguments[0]}")
    return result


def now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def load_request() -> dict[str, object]:
    value = json.loads(REQUEST.read_text(encoding="utf-8"))
    if value.get("branch") not in {"main", "master"}:
        raise ValueError("branch must be main or master")
    if not str(value.get("message") or "").strip():
        raise ValueError("commit message is required")
    paths = value.get("paths")
    if not isinstance(paths, list) or not paths or any(not isinstance(path, str) or not path for path in paths):
        raise ValueError("paths must be a non-empty string list")
    if len(set(paths)) != len(paths):
        raise ValueError("paths must not contain duplicates")
    return value


def status_paths() -> tuple[list[str], list[str]]:
    lines = run(["git", "status", "--porcelain=v1", "--untracked-files=all"]).stdout.splitlines()
    staged: list[str] = []
    paths: list[str] = []
    for line in lines:
        if len(line) < 4:
            continue
        state, path = line[:2], line[3:]
        if " -> " in path:
            path = path.split(" -> ", 1)[1]
        paths.append(path)
        if state[0] not in {" ", "?"}:
            staged.append(path)
    return sorted(paths), sorted(staged)


def fingerprints(paths: list[str]) -> dict[str, str]:
    result: dict[str, str] = {}
    for relative in paths:
        path = PROJECT / relative
        if not path.is_file():
            raise RuntimeError(f"release path is missing or not a file: {relative}")
        result[relative] = hashlib.sha256(path.read_bytes()).hexdigest()
    return result


def write_json_atomic(path: Path, value: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    staged = path.with_name(path.name + ".new")
    staged.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    staged.replace(path)


def validate_workspace(request: dict[str, object]) -> tuple[list[str], dict[str, str]]:
    branch = run(["git", "branch", "--show-current"]).stdout.strip()
    if branch != request["branch"]:
        raise RuntimeError(f"expected branch {request['branch']}, found {branch}")
    paths, staged = status_paths()
    expected = sorted(request["paths"])
    if staged:
        raise RuntimeError(f"refusing pre-staged changes: {', '.join(staged)}")
    if paths != expected:
        missing = sorted(set(expected) - set(paths))
        unexpected = sorted(set(paths) - set(expected))
        raise RuntimeError(f"release scope mismatch; missing={missing}, unexpected={unexpected}")
    check = run(["git", "diff", "--check"], check=False)
    if check.returncode != 0:
        raise RuntimeError(check.stdout.strip() or check.stderr.strip() or "git diff --check failed")
    return paths, fingerprints(paths)


def plan() -> None:
    request = load_request()
    paths, hashes = validate_workspace(request)
    value = {
        "version": 1,
        "created_at": now(),
        "branch": request["branch"],
        "message": request["message"],
        "paths": paths,
        "sha256": hashes,
        "remote": run(["git", "remote", "get-url", "origin"]).stdout.strip(),
    }
    write_json_atomic(PLAN, value)
    print(json.dumps(value, indent=2))
    print(f"PLAN={PLAN}")


def apply() -> None:
    request = load_request()
    value = json.loads(PLAN.read_text(encoding="utf-8"))
    paths, hashes = validate_workspace(request)
    if value.get("version") != 1 or value.get("branch") != request["branch"] or value.get("message") != request["message"]:
        raise RuntimeError("release plan is missing or stale")
    if value.get("paths") != paths or value.get("sha256") != hashes:
        raise RuntimeError("release files changed after planning; plan again")

    run(["git", "add", "--", *paths])
    staged = sorted(run(["git", "diff", "--cached", "--name-only"]).stdout.splitlines())
    if staged != paths:
        raise RuntimeError(f"staged scope mismatch: {staged}")
    check = run(["git", "diff", "--cached", "--check"], check=False)
    if check.returncode != 0:
        raise RuntimeError(check.stdout.strip() or check.stderr.strip() or "staged diff check failed")
    run(["git", "commit", "-m", str(request["message"])])
    run(["git", "push", "origin", str(request["branch"])])
    commit = run(["git", "rev-parse", "HEAD"]).stdout.strip()
    print(f"COMMIT={commit}")
    print(f"PUSHED=origin/{request['branch']}")


def notify(message: str) -> None:
    executable = shutil.which("notify-send")
    if executable:
        subprocess.run([executable, "TeleAntiSpam2Bot release", message], check=False)


def main() -> int:
    if len(sys.argv) != 2 or sys.argv[1] not in {"plan", "apply"}:
        raise ValueError("usage: release_changes.py {plan|apply}")
    if sys.argv[1] == "plan":
        plan()
        notify("Commit/push plan is ready")
    else:
        apply()
        notify("Changes committed and pushed")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        notify(f"Release failed: {exc}")
        raise SystemExit(1)
