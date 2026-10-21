---
name: wtc-browse
description: Open LazyVim on a worktree collection with one vim tab per sibling so git integrations work. Use when the user wants to look around a collection, browse its files, or open the wtc in nvim. From a terminal it opens in this window; an agent inside herdr uses wtc open --add browse for the workspace's browse pane. For a text snapshot of branches and PRs, use wtc-status instead.
---

# Browse a wtc in neovim

One LazyVim, one vim tab per sibling (`:tcd`). Git plugins then see a real
repo. The multi-repo map is `wtc status`, not this buffer. If you need to
*say* what is in flight, run `wtc status`.

## 1. Confirm you are in a collection

A `harness/` next to the siblings, or `harness/AGENTS.md` one level up. The
collection root is the directory holding `harness/`.

## 2. Open it

From an agent pane, open the workspace's browse pane:

```bash
wtc open --add browse                # this collection
wtc open <collection> --add browse   # a named one under the workspace root
```

It adds the pane if the workspace has none and starts nvim in it — never in
the agent itself.

From a terminal (including the herdr `shell` pane), `wtc browse` opens nvim
**in this window**:

```bash
wtc browse                  # this collection
wtc browse <collection>     # a named one under the workspace root
```

Run from an agent pane, `wtc browse` refuses and points at
`wtc open --add browse`. Do not pass `--here` from an agent pane unless the
user asked to take it over.

## 3. Leave it alone

nvim uses the alternate screen. Do not `herdr pane read` it to answer a
question, and do not scrape the TUI. The human reads it; you keep working
in your own pane. Status-pane clicks talk to this nvim over its listen
socket. gt / gT (or the tabline) switches siblings.

---
Canon: `.wtc/instructions/herdr.md`.
