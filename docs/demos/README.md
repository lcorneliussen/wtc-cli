# Recorded WTC demos

These terminal recordings run real commands against fresh synthetic Git repos.
The candidate adds a small harness scaffold; v0.1.38 does not contain it. No
service runner, forge account, desktop session, or agent is needed.

## One task, two repositories

Run `wtc new add-search api web`, then open the live view with `wtc status`.
Quit with `q`, make a frontend edit, and refresh with `r`.
No flags are needed to choose the interactive view in a terminal.

![Create and inspect a collection](collections.gif)

[Transcript](collections.txt) · [Asciicast](collections.cast)

## Environment and secret inventories

Browse `wtc env list` and `wtc secrets list` with their default TUIs. Move the
selection to inspect names, scopes, and link states. Environment values and
credential contents are never shown; the fixture files contain synthetic data.

![Browse environment and secret link inventories](inventories.gif)

[Transcript](inventories.txt) · [Asciicast](inventories.cast)

## Small harness scaffold — preview

Create six project-owned files without initializing Git, contacting a remote,
installing tools, or running hooks. The harness owns its registry and pins;
standard collection instructions come from the CLI. The two flags shown here
are required: the remote to record and the exact CLI version to pin.

![Scaffold a minimal harness](harness.gif)

[Transcript](harness.txt) · [Asciicast](harness.cast)

## Reproduce

Build the candidate, then supply Python 3, Git, and `pyte` 0.8.2 for capturing
readable TUI screen transcripts. The optional
[agg](https://github.com/asciinema/agg) 1.9.0 executable converts casts to GIFs:

```sh
go build -o ./wtc ./cmd/wtc
python3 -m venv /tmp/wtc-demo-venv
/tmp/wtc-demo-venv/bin/pip install pyte==0.8.2
/tmp/wtc-demo-venv/bin/python docs/demos/record.py --wtc ./wtc --agg /path/to/agg
```

Omitting `--agg` records only casts and transcripts. The script uses fresh local
fixture repos and an explicit environment, and removes its temporary workspace.
Normal bootstrap clones your harness from its forge remote.

GIFs use the Nord palette, 20px monospace text, and an 88 × 16 terminal.
Commands are typed at a readable pace; output and TUI updates are captured
live from a real pseudoterminal. The last TUI stays visible at the end of the
recording; the script sends `q` and waits for the process to exit afterward.
TUI transcripts contain the actual screen just before quitting, with repeated
blank rows collapsed, rather than raw cursor-control sequences.

The v0.1.38 pin is fixture metadata only: every command uses the supplied
candidate binary. No pin is installed or changed in your projects. Captures
replace only their own temporary sandbox path with `~/wtc-demo`; transcripts
also normalize terminal whitespace. Inspect all captures before publishing.
