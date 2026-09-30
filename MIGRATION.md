# Shell-to-CLI migration inventory

This checklist tracks the generic reference harness tools. Names are
provisional until the requested naming pass after the command migration.
Shell entry points can remain as thin compatibility shims once a released CLI
command and its contract checks cover their behavior.

| Shell entry point | Provisional CLI | State |
|---|---|---|
| `refresh-env.sh` | `wtc env` | Native in v0.1.3; target-aware shim in v0.1.9; native `--all` and safe shell sweep released in v0.1.14 |
| `wtc-pr.sh` | `wtc pr path/list/enlist/unlist` | Native in v0.1.3; released target-aware shell shim and binary contract tests |
| `refresh-configs.sh` | `wtc registry refresh` | Native in v0.1.3; generic shell shim released in v0.1.6 |
| `link-mcp.sh` | `wtc mcp render` | Native for one collection in v0.1.3; native `--all` and target-aware shell shim released in v0.1.14 |
| `link-skills.sh` | `wtc skills render` | Native in v0.1.8; released target-aware shell shim and contract tests |
| Skill section patches and overlay drift | `wtc skills diff` | H2 section patches and base-digest drift review implemented; release and published-binary contracts pending |
| `link-secrets.sh` | `wtc secrets link` | Native in v0.1.5; released shims and harness tests in upstream and derivatives |
| `agent-env.sh` | `wtc agent-env` | Native in v0.1.6; generic shim released; configurable collection-local bins in v0.1.7 |
| `branch-off.sh` | `wtc new` | Native in v0.1.10; reference shim released with matching-pin dispatch and bootstrap fallback |
| `add-repo.sh` | `wtc add-repo` | Native in v0.1.11; target-aware shim and released-binary contract tests |
| `retire.sh` | `wtc retire` | Native in v0.1.12; released target-aware shell shim and binary contract tests |
| `catch-up.sh` | `wtc catch-up` | Native in v0.1.15; matching-pin reference shim and released-binary contract tests |
| `wtc-status.sh`, `wtc-status-tui.sh` | `wtc status` | Native one-shot and TUI released in v0.1.16; reference shims, published-binary contracts, and watch lifecycle ported |
| `wtc-open.sh` | `wtc open` | Native command released in v0.1.21; matching-pin shim, bootstrap guidance, and published-binary contracts merged in the reference harness and in-scope ports |
| `wtc-browse.sh` | `wtc browse` | Native launch, bundled Neovim view, and herdr routing released in v0.1.17; reference matching-pin shim and released-binary contracts merged |
| `review-bundle.sh` | `wtc review bundle` | Native public and private bundles, branch checkout, collection catch-up, concern overlays, related patches, registry snapshots, prior summaries, and GitHub/Bitbucket reply context released in v0.1.23 with a matching-pin reference shim and published-binary contract tests |
| `review-run.sh` | `wtc review run` | Native local runner, per-agent usage, and integrated posting released in v0.1.24; matching-pin reference shim and published-binary contracts merged |
| `review-post.sh` | `wtc review post` | Native summary lifecycle and inline dedup; matching-pin reference shim and synthetic published-binary GitHub contract merged; Bitbucket forge parity pending |
| `review-status.sh` | `wtc review status` | Native GitHub/Bitbucket status and trusted-local check; matching-pin reference shim and published-binary contracts merged |
| `review-resolve.sh` | `wtc review resolve` | Native reply/resolve; matching-pin reference shim and synthetic published-binary GitHub contract merged; Bitbucket forge parity pending |
| `bb-pr-ready.sh` | `wtc review ready` | Native guarded promotion; matching-pin reference shim and published-binary contract merged |

`wtc-status-legacy.sh`, `wtc-status-legacy-tui.sh`, `lib.sh`, and
`wtc-status-common.sh` are implementation support for those entry points.
The status, PR-facts, and review Python helpers and browse Lua integration
remain in shell fallback paths. The native commands have replacements for
their normal dispatch; retire a helper only when its retained shim no longer
needs it.

The completion gate is behavior, not a command name: run the relevant
black-box harness contracts through both the CLI and retained shims, including
dirty worktrees, hook failures, dry runs, and forge-unavailable states. Finish
with a naming pass over command help, shims, skills, and docs.
