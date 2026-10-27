#!/usr/bin/env python3
"""A file-backed runner: it never executes a task or a provider command."""
import json
import pathlib
import re
import sys

if sys.argv[1:] == ["--version"]:
    print("dekit 0.10.0")
    sys.exit(0)
root = pathlib.Path(sys.argv[2])
args = sys.argv[4:]
command = " ".join(args)
with (root / "calls").open("a") as log:
    log.write(command + "\n")
if (root / "fail-action").exists() and (root / "fail-action").read_text() == command:
    sys.exit("synthetic runner failure")
state_path = root / "runner-state.json"
state = json.loads(state_path.read_text()) if state_path.exists() else {"running": False, "tasks": {}}
tasks = {t["path"]: t for t in json.loads((root / "manifest.json").read_text())["tasks"]}
for path in tasks:
    state["tasks"].setdefault(path, "idle")

def start(path):
    for dep in tasks[path].get("deps", []):
        start(dep)
    state["tasks"][path] = "ready"
    state["running"] = True

if args == ["runner", "status"]:
    print(json.dumps({"status": "running" if state["running"] else "absent"}))
elif args == ["ls"]:
    for path, count in list(state.get("pending", {}).items()):
        state["pending"][path] = count - 1
        if count <= 1:
            state["tasks"][path] = "idle"
            del state["pending"][path]
        elif (root / "stop-query-fails").exists():
            state["tasks"][path] = "unknown"
    print(json.dumps({"tasks": [{"path": p, "state": s} for p, s in state["tasks"].items()]}))
elif args == ["down"] or args == ["runner", "stop"]:
    state["running"] = False
    state["tasks"] = {p: "idle" for p in tasks}
elif args[0] == "veto":
    stopped = {args[1]}
    if args[1].endswith("/**"):
        stopped = {p for p in tasks if p.startswith(args[1][:-2])}
    while True:
        dependents = {p for p, t in tasks.items() if any(d in stopped for d in t.get("deps", []))}
        if dependents <= stopped:
            break
        stopped |= dependents
    for path in stopped:
        state["tasks"][path] = "idle"
        if (root / "slow-stop").exists():
            state["tasks"][path] = "stopping"
            state.setdefault("pending", {})[path] = 3
elif args[0] in ["start", "restart"]:
    start(args[1])
elif args == ["up"]:
    # Read autostart from the generated native config, independently of intent.
    path = None
    for line in (root / "dekit.yaml").read_text().splitlines():
        match = re.fullmatch(r"    ([^ ]+):", line)
        if match:
            path = match.group(1)
        if line == "        autostart: true" and path in tasks:
            start(path)
    state["running"] = True
elif args[0] == "rm":
    state["tasks"].pop(args[1], None)
elif args == ["reload", "--yes"]:
    if state.get("pending"):
        sys.exit("configuration reloaded before stop confirmation")
else:
    sys.exit("unexpected fixture command: " + command)
state_path.write_text(json.dumps(state))
