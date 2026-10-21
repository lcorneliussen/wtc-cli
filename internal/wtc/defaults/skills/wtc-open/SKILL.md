---
name: wtc-open
description: Open or reshape a worktree collection in herdr — ensure the agent and status panes, add browse or shell panes on demand, trim idle ones, switch wide ↔ narrow, and start an agent only when the agent pane is empty. Use when the user says wtc-open, open the collection in herdr, switch to narrow/wide layout, add a browse or shell pane, trim panes, or fill missing panes after starting an agent first.
---

# Open a collection in herdr

Mechanism: `wtc open`. Canon: `.wtc/instructions/herdr.md`.

A workspace is ergonomics only — worktrees already exist. Opening is
idempotent: reuse the workspace, keep a live agent, fill idle panes, and
only reshape when asked (or when the layout is still partial). A workspace
starts with agent and status only; browse and shell come on demand, because
every pane holds one of the machine's capped pseudo-terminals.

## 1. Run it

From the collection root (or name the collection):

```bash
wtc open                 # this collection; agent + status, auto layout
wtc open --add browse    # add the browse pane (or shell); repeatable
wtc open --full          # agent, status, browse and shell
wtc open --narrow        # stacked tabs; switch if needed
wtc open --wide          # stacked columns; switch if needed
wtc open --list          # pane-by-pane report; change nothing
wtc open --dry-run       # plan only
wtc open --trim          # close idle browse/shell panes (--all: every collection)
```

`WTC_PANES=full` in the control root's `wtc.env` makes `--full` the default.

Bare args after flags are collection names under the workspace root. `--all`
is always something you typed.

### Narrow vs wide

| | Wide | Narrow |
|---|---|---|
| Shape | `[ agent \| browse ]` / `[ shell \| status ]`; agent beside status without tools | tab `main`: agent/status; tab `tools`: browse/shell, once added |
| Heights | shell 20%, status 35% | same ratios, stacked full-width |
| When | default when the session is wide enough | `--narrow`, or `WTC_LAYOUT=narrow` / auto under `WTC_LAYOUT_NARROW_AT` |

`--narrow` / `--wide` **switch** an existing workspace. The agent pane is
preserved; the status pane moves with its running process. Auto never flips a complete wide ↔
narrow on its own.

## 2. Agent-first is normal

Often the human (or you) starts the agent in a bare workspace, then opens:

1. `wtc open` detects a **partial** layout and builds wide or narrow around
   the existing agent pane.
2. If the agent pane already has a live agent, it is left alone.
3. If that pane is empty (or the agent exited), `wtc-open` starts one —
   unless `--no-agent`.

Do not restart a working agent to "apply" a layout. Do not close panes to
force a reshape — pass `--narrow` or `--wide` and let the command move panes.

## 3. Panes on demand

When a step needs a human to look at files, `wtc open --add browse`; for a
spare prompt, `--add shell`. `--trim` closes a shell at its prompt and a
browse Neovim without unsaved buffers — never agent or status, never a
focused pane or a running command. Trim when the user asks, or when
`wtc open` warns that the machine is close to its pty cap.

## 4. What to report

The command prints one line per collection (`layout switched (narrow)`,
`agent live`, `browse started`, …). Prefer that over re-probing. `--list`
is the read-only check.

## 5. Stay out of the way

- Do not `herdr session stop` or close the workspace to change layout.
- Do not type browse/status commands into a pane that is already running them.
- Attach is the human's job: `herdr --session <project>`.

---
Canon: `.wtc/instructions/herdr.md`.
