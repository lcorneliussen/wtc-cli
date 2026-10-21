# wtc unreleased

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
