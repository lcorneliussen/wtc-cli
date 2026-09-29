# Shell-to-CLI migration inventory

This checklist tracks the generic reference harness tools. Names are
provisional until the requested naming pass after the command migration.
Shell entry points can remain as thin compatibility shims once a released CLI
command and its contract checks cover their behavior.

| Shell entry point | Provisional CLI | State |
|---|---|---|
| `refresh-env.sh` | `wtc env` | Native in v0.1.3; shell shim and `--all` parity pending |
| `wtc-pr.sh` | `wtc pr path/list/enlist/unlist` | Native in v0.1.3; shell shim pending |
| `refresh-configs.sh` | `wtc registry refresh` | Native in v0.1.3; shell shim pending |
| `link-mcp.sh` | `wtc mcp render` | Native for one collection in v0.1.3; `--all` shim parity pending |
| `link-skills.sh` | `wtc skills render` | Pending |
| `link-secrets.sh` | `wtc secrets link` | Native in v0.1.5; released shims and harness tests in upstream and derivatives |
| `agent-env.sh` | `wtc agent-env` | Native in v0.1.6; released generic shim; configurable collection-local bins in this branch |
| `branch-off.sh` | `wtc new` | Pending |
| `add-repo.sh` | `wtc add-repo` | Pending |
| `retire.sh` | `wtc retire` | Pending |
| `catch-up.sh` | `wtc catch-up` | Pending |
| `wtc-status.sh`, `wtc-status-tui.sh` | `wtc status` | Pending data layer and TUI |
| `wtc-open.sh` | `wtc open` | Pending |
| `wtc-browse.sh` | `wtc browse` | Pending |
| `review-bundle.sh` | `wtc review bundle` | Native public no-catch-up subset in v0.1.4; private snapshots, overlays, catch-up, and prior-round context pending |
| `review-run.sh` | `wtc review run` | Native local runner subset in v0.1.4; integrated posting lifecycle and rich stats pending |
| `review-post.sh` | `wtc review post` | Native summary lifecycle, inline dedup, and local receipt in v0.1.4; forge parity tests pending |
| `review-status.sh` | `wtc review status` | Native GitHub and Bitbucket status and trusted-local check in v0.1.4 |
| `review-resolve.sh` | `wtc review resolve` | Native reply/resolve in v0.1.4; forge parity tests pending |
| `bb-pr-ready.sh` | `wtc review ready` | Native guarded promotion in v0.1.4 |

`wtc-status-legacy.sh`, `wtc-status-legacy-tui.sh`, `lib.sh`, and
`wtc-status-common.sh` are implementation support for those entry points.
The status, PR-facts, and review Python helpers and browse Lua integration also need
native replacements or retirement once their callers move.

The completion gate is behavior, not a command name: run the relevant
black-box harness contracts through both the CLI and retained shims, including
dirty worktrees, hook failures, dry runs, and forge-unavailable states. Finish
with a naming pass over command help, shims, skills, and docs.
