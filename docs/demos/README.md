# Recorded WTC demos

These terminal recordings run real commands against fresh synthetic Git repos.
The candidate adds a small harness scaffold; v0.1.38 does not contain it. No
service runner, forge account, desktop session, or agent is needed.

## Small harness scaffold

Create six project-owned files without initializing Git, contacting a remote,
installing tools, or running hooks. The harness owns its registry and pins;
standard collection instructions come from the CLI.

![Scaffold a minimal harness](harness.gif)

[Transcript](harness.txt) · [Asciicast](harness.cast)

## One task, two repositories

Create `add-search` with API and web worktrees, inspect its status, and see a
local frontend edit appear in the repo row.

![Create and inspect a collection](collections.gif)

[Transcript](collections.txt) · [Asciicast](collections.cast)

## Reproduce

Build the candidate, then supply Python 3 and Git. The optional
[agg](https://github.com/asciinema/agg) 1.9.0 executable converts casts to GIFs:

```sh
go build -o ./wtc ./cmd/wtc
python3 docs/demos/record.py --wtc ./wtc --agg /path/to/agg
```

Omitting `--agg` records only casts and transcripts. The script uses fresh local
fixture repos and an explicit environment, and removes its temporary workspace.
Normal bootstrap clones your harness from its forge remote.

The v0.1.38 pin is fixture metadata only: every command uses the supplied
candidate binary. No pin is installed or changed in your projects. Captures
replace only their own temporary sandbox path with `~/wtc-demo`; transcripts
also normalize terminal whitespace. Inspect all captures before publishing.
