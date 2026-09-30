# CLI command map and compatibility

This map tracks the generic reference harness entry points. The naming audit
kept the released CLI names: collection actions are top-level verbs, while
related operations use nouns (`skills render/diff`, `review bundle/run/post/
status/resolve/ready`, `pr path/list/enlist/unlist`, `registry refresh`,
`mcp render`, and `secrets link`). The old script names remain compatibility
entry points. No command rename is needed for the final transition.

Skill names keep the `wtc-` prefix for agent discovery. Mechanism-backed
skills use the matching action name (`wtc-new`, `wtc-open`, `wtc-catch-up`,
`wtc-browse`, `wtc-status`, `wtc-add-repo`, and `wtc-retire`). Procedure-only
skills such as `wtc-start`, `wtc-follow`, `wtc-pr`, and `wtc-local-review` keep
their task names; they are not claims that a same-named CLI command exists.

| Shell entry point | CLI | State |
|---|---|---|
| `refresh-env.sh` | `wtc env` | Native in v0.1.3; target-aware shim in v0.1.9; native `--all` and safe shell sweep released in v0.1.14 |
| `wtc-pr.sh` | `wtc pr path/list/enlist/unlist` | Native in v0.1.3; released target-aware shell shim and binary contract tests |
| `refresh-configs.sh` | `wtc registry refresh` | Native in v0.1.3; generic shell shim released in v0.1.6 |
| `link-mcp.sh` | `wtc mcp render` | Native for one collection in v0.1.3; native `--all` and target-aware shell shim released in v0.1.14 |
| `link-skills.sh` | `wtc skills render` | Native in v0.1.8; released target-aware shell shim and contract tests |
| Skill section patches and overlay drift | `wtc skills diff` | H2 section patches and base-digest drift review released in v0.1.25; published-binary contracts cover content drift, renamed headings, refusal, and recovery |
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
| `review-post.sh` | `wtc review post` | Native summary lifecycle and inline dedup; matching-pin reference shim and synthetic published-binary GitHub contract merged; Bitbucket CLI and API paths covered by synthetic Go forge tests |
| `review-status.sh` | `wtc review status` | Native GitHub/Bitbucket status and trusted-local check; matching-pin reference shim and published-binary contracts merged; synthetic Bitbucket trust/head test added |
| `review-resolve.sh` | `wtc review resolve` | Native reply/resolve; matching-pin reference shim and synthetic published-binary GitHub contract merged; Bitbucket reply/resolve covered by synthetic Go forge tests |
| `bb-pr-ready.sh` | `wtc review ready` | Native guarded promotion; matching-pin reference shim and published-binary contract merged |

`wtc-status-legacy.sh`, `wtc-status-legacy-tui.sh`, `lib.sh`, and
`wtc-status-common.sh` are implementation support for those entry points.
The status, PR-facts, and review Python helpers and browse Lua integration
remain in shell fallback paths. The native commands have replacements for
their normal dispatch; retire a helper only when its retained shim no longer
needs it.

Completion is about behavior: run the relevant black-box harness contracts
through the CLI and retained shims, including dirty worktrees, hook failures,
dry runs, and forge-unavailable states. The naming pass found the released
command names consistent across CLI help and embedded skills; keep the script
spellings as compatibility names in the reference harness.
