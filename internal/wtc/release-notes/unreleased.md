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

## Review

- **`wtc review run` works without harness prompt files.** A private bundle
  used to fail when the harness had no `review/prompts/concern.md` or
  `lead.md`. Each prompt now falls back to the embedded default on its own, so
  a harness can override just one of them
  ([#46](https://github.com/lcorneliussen/wtc-cli/issues/46)).
- **Review someone else's PR without writing to it.** `wtc review start
  <repo>#<n>`, or `wtc new --pr <repo>#<n> --review`, creates a review-only
  collection ([#117](https://github.com/lcorneliussen/wtc-cli/issues/117)):
  - Pushing from the PR worktree is disabled in that worktree's own Git
    config, so neither an agent nor a tool can push by accident: every remote
    gets a dead push URL, other URLs are rewritten to it, and a `pre-push` hook
    refuses the rest. Only `--no-verify` to a URL that no remote had at
    creation gets past the hook. Other worktrees of the repository can still
    push. This turns on `extensions.worktreeConfig` (format version 1) for the
    repository; in a bare owner, `core.bare` moves into the owner's own
    `config.worktree`, as Git requires. A remote with its own
    `remote.<name>.pushurl` cannot be locked, so creation refuses it before
    changing the repository.
  - `wtc review bundle` skips catch-up there and moves the worktree to the PR's
    current head, so running it again after the author pushes starts the next
    round. A commit made in the collection stops it rather than being lost.
    Catch-up leaves the PR worktree alone.
  - `wtc review resolve` and `wtc review ready` refuse, `--user-authorized`
    included: those steps belong to the author.
  - The launch note, the first agent prompt and `wtc status` say review-only;
    status JSON carries `review_only` on the repository row.

## herdr: panes on demand

Every herdr pane holds a pseudo-terminal, and macOS caps those machine-wide
(`kern.tty.ptmx_max`, default 511). Four panes per collection reached that
cap, and after that no `wtc open` or `wtc new` could start
([#118](https://github.com/lcorneliussen/wtc-cli/issues/118)).

- **`wtc open` starts with the agent and status panes.** In the narrow
  layout, status sits under the agent on the `main` tab. In the wide layout
  it sits to the right of the agent. `--full`, or `WTC_PANES=full` in the
  control root's `wtc.env`, keeps opening all four panes.
- **`wtc open --add browse|shell`** adds one tool pane where the layout puts
  it and starts it. The flag can be repeated.
- **`wtc open --trim`** closes idle tool panes: a shell at its prompt, or a
  browse Neovim that has no unsaved buffers. It keeps focused panes, shells
  running a command, and a Neovim that does not answer. It never closes agent
  or status panes. `--all` trims every collection, and `--dry-run` shows what
  would close.
- **`--list`, `--narrow` and `--wide` work on workspaces with fewer panes.**
  A missing browse or shell pane is not reported as damage. Switching layouts
  moves the panes that exist and creates no new tool panes. A partial layout
  names the agent or status pane it is missing.
- **pty budget.** When the machine is close to its pty cap, `wtc open` warns
  and `wtc doctor` reports `pty low`, naming the herdr sessions and pane
  kinds that hold the most. An open that fails with `openpty` now says the
  machine is out of pseudo-terminals.
- **`wtc browse` opens in the terminal it runs in.** From an agent pane it no
  longer sends Neovim to the browse pane or opens a `gh dash` `pr` tab.
  Instead it refuses and points at `wtc open --add browse`. `--here` still
  takes over the current terminal.
