# Recorded WTC demos

These recordings run the candidate CLI against fresh synthetic Git repositories.
They are terminal captures of actual commands and output, rather than a desktop
recording. The services run in a detached dekit runner; no agent process hosts
them. Runtime and minimal-harness features are not included in v0.1.38.

## One task, two repositories

Create `add-search` with API and web worktrees, inspect its status, and see a
local frontend edit appear in the repo row.

![Create and inspect a collection](collections.gif)

[Transcript](collections.txt) · [Asciicast](collections.cast)

## Services, grouped endpoints, and logs

Start an existing build-and-run command and its loopback relay as one group.
Read their status and logs, restart with the cached build, then stop processes.
The synthetic owned data folder and append-only logs remain after `down`.

![Service lifecycle](runtime.gif)

[Transcript](runtime.txt) · [Asciicast](runtime.cast)

The relay runs on the same machine. It demonstrates grouping and dependencies;
it does not demonstrate cross-machine access or a cloud provider. Replace it
with the project's existing foreground tunnel command for real onboarding.

## Cleanup failure

A deliberate resource teardown failure blocks retirement and preserves the
worktrees. Correct the override and retry: owned resources and the collection
are removed, while a synthetic shared resource marker remains.

![Retirement failure and retry](cleanup.gif)

[Transcript](cleanup.txt) · [Asciicast](cleanup.cast)

## Reproduce

Build the candidate, then supply Python 3, Git, and dekit 0.10.0. The optional
[agg](https://github.com/asciinema/agg) 1.9.0 executable converts casts to GIFs:

```sh
go build -o ./wtc ./cmd/wtc
python3 docs/demos/record.py \
  --wtc ./wtc --dekit /path/to/dekit --agg /path/to/agg
```

Run from the CLI repository. Omitting `--agg` records only the transcripts and
casts. The script creates its own temporary repos and owners, uses an explicit
environment, launches only loopback services, and cleans up its fixture. Its
local remotes are for an offline demo; ordinary bootstrap uses forge remotes.

The fixture's v0.1.38 pin is metadata only. All commands use the candidate
binary supplied with `--wtc`; no pin is installed or changed in your projects.
The recordings replace only their own temporary sandbox path with `~/wtc-demo`.
No command results are substituted. Inspect transcripts and animations before
publishing new captures.
