# wtc unreleased

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
