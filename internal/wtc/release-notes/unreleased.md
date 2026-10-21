# wtc unreleased

## Catch-up

- An owner without any remote no longer drops out of catch-up.
  Its fetch row reads `no-remote`, which does not fail the run, and its
  worktrees are reconciled against the owner's own branches (`main` rather
  than `origin/main`). Status measures them against the same local branch and
  no longer tries to fetch them. When a configured `origin` cannot be reached,
  the fetch row now carries git's own error line instead of the owner path, and
  repo-scoped hooks such as secret linking still run
  ([#64](https://github.com/lcorneliussen/wtc-cli/issues/64)).

## Retire

- When `wtc retire` cannot delete a managed collection folder because
  unexpected files remain, it writes a `.wtc-retired` marker (JSON: `retired_at`,
  `cli_version`, `blocking_paths`). Workspace sweeps (`catch-up --all`,
  `env`, `skills`, `mcp` and `open` sweeps) skip a marked folder, commands
  run inside it say it holds a retired collection's leftovers, and status and
  catch-up reports are no longer written into it. `wtc status --all` lists it
  under "Retired leftovers". Running `wtc retire <collection>` on a marked
  folder finishes the job once the listed files are gone
  ([#56](https://github.com/lcorneliussen/wtc-cli/issues/56)).

## Status and catch-up

- A squash merge now counts as landed when its merge commit is on the target
  and has the same tree as the branch tip, or the same change as the
  branch's cumulative diff. Before, a PR whose head fact was unavailable stayed
  "merged PR landing not established".
- A status refresh of a collection now returns a clean worktree, still on the
  branch of a PR it sees merged, to the development tip once landing is
  established, and says so in the refresh log. Opt out with
  `WTC_STATUS_RETURN_MERGED=off` or `[status] return_merged = false` in
  `harness/wtc.toml`. Workspace sweeps (`--all`) never move worktrees
  ([#59](https://github.com/lcorneliussen/wtc-cli/issues/59)).
- Catch-up's PR-state lookups through `gh` or `bb` now give up after 30
  seconds, as status lookups do, and leave the branch untouched instead of
  waiting on a stalled forge CLI.
