# wtc

A versioned CLI for worktree collections. The reference harness keeps shell
entry points as compatibility shims; their normal paths use the pinned binary.

## Commands

- `wtc env [--collection DIR] [--dry-run]` regenerates `.env.collection`, preserving its port base. It leaves `.env.collection.local` intact, writes the collection's mise environment file, and trusts the generated mise config when mise is installed.
- `wtc doctor` checks the collection registry and local tool availability.
- `wtc commands --json` lists the command surface for agents.
- `wtc eject 'skills/wtc-*'` copies selected embedded defaults into the harness so they can be customized. Existing files are never overwritten.
- `wtc pr path|list|enlist|unlist` manages the collection-local `.wtc-prs` file used by status and catch-up.
- `wtc registry refresh` regenerates the local bare-owner map and reports registry mismatches.
- `wtc mcp render [--dry-run]` renders the harness MCP registry into Claude, Cursor, and Codex config files for this collection.
- `wtc secrets link [--repo <name>] [--dry-run] [--include-prod]` links gitignored control-root files into checked-out worktrees, preserving displaced local files in collection backups.
- `wtc agent-env` prints shell exports for sibling mise toolchains and configured collection-local bins; `--write` refreshes `.env.toolchain`, `--print-path` prints its bin list, and `--wrap` handles PreToolUse JSON from stdin.
- `wtc skills render [--seed-scope] [--dry-run]` exposes embedded skills and harness overrides to agent clients, wires the collection entry point and hooks, and refreshes agent shell files. `--all` explicitly renders every collection in the workspace.
- `wtc skills diff [--changes] [--json]` reports skill overrides, H2 section patches, and drift from a recorded base digest.
- `wtc customize` prints the versioned customization guide, including harness hooks and application repository init/teardown hooks.
- `wtc new [slug] [repo ...]` creates a collection from a slug, issue, tracker key, or GitHub PR head, then prepares its environment and runs repository init hooks.
- `wtc open [collection ...]` opens or repairs herdr workspaces with agent, browse, shell, and status panes. Use `--list` or `--dry-run` to inspect them, `--wide` or `--narrow` to set the layout, and `--all` only when intentionally opening every collection. `wtc new --open` uses this native command.
- `wtc status` reports collection status. In an interactive terminal it logs collector steps and elapsed time on stderr; `--silent` hides that progress. `--json`, `--md`, and piped output stay free of progress messages. `wtc status --tui` shows the current step while refreshing; click “refreshing” or press `l` to inspect its refresh log. Active PR details use a 90-second forge cache, while merged PR details can be reused for 24 hours; setting `WTC_FORGE_CACHE_AGE` overrides both ages.
- `wtc add-repo <repo> [repo ...]` adds detached worktrees to the current collection, prepares secrets and generated files, and runs each new repository's init hook. `--collection NAME` explicitly selects another collection.
- `wtc retire <collection>` checks for dirty or unpushed work, runs repository teardown hooks, removes the collection's worktrees and generated files, and leaves remote branches intact. Run it from a different collection; use `--force` only after checking that local work is disposable.
- `wtc review status <repo> [pr-number] [--trusted-local]` reads the newest local review status comment and checks whether it covers the current PR head.
- `wtc review bundle <repo> [pr-number]` checks out the PR branch and catches up the current collection when a PR is known, then builds a private review bundle with concern overlays, related PR patches, and registry-defined repository snapshots. Branch-only reviews use the current local refs. `--no-catch-up` skips the PR catch-up; `--public` excludes local overlays and other repositories. Public rounds keep prior context from public bundles only. Inspect the exact bundle before giving it to an external reviewer or posting its summary.
- `wtc review run <bundle-dir>` launches separate concern agents, runs a lead agent, and writes a verdict and summary into the bundle. `--post` posts a progress comment before the run and updates it with the result; inspect public review output before posting.
- `wtc review post <bundle-dir>` posts or updates the review comment, records a local receipt, and adds deduplicated inline findings.
- `wtc review resolve <bundle-dir>` replies to and resolves inline review threads; without filters, it resolves every unresolved thread in the bundle.
- `wtc review ready <pr-number> [--repo NAME]` promotes a draft only after a current, locally posted passing review (or an explicit user override).
- `wtc browse [collection]` opens the bundled Neovim collection view. From an agent pane it uses that collection's browse pane; `--here` opens it in the current terminal. A harness can replace the view through `overlays/browse/wtc-browse.lua`.

Run locally with `go run ./cmd/wtc`. Tagged releases build macOS and Linux
binaries through GoReleaser. A harness pins an exact release in its checked-in
`.wtc-cli-version` file (for example, `0.1.25`). `wtc env` reads that file and
writes the tool entry into the generated collection-root `mise.toml`, which
every sibling inherits. Then run `mise install` in the collection. No global
installation is required.

```toml
[tools]
"github:lcorneliussen/wtc-cli" = "0.1.25"
```

The pin shown here is an example; use a published release version. Retained
shell entry points select the target collection's pin and keep a bootstrap
fallback for older installations.

Additional collection-root mise tools belong in the harness's committed `wtc.toml` under `[mise.tools]`. The CLI retains them when regenerating `mise.toml`. The exact CLI pin remains in `.wtc-cli-version` and cannot be overridden there.

The customization guide is embedded in the binary (`wtc customize`), and a harness can eject `skills/wtc-customize` or other defaults for local editing. Executable `harness/hooks/wtc/*.sh` scripts can react to migrated CLI actions; application repositories keep their worktree setup and cleanup in `harness:init` and `harness:teardown` tasks or `.harness/` scripts.

`wtc` reads registries using `repos`, `repositories`, or `selected` plus
categorized `non_default` entries. Harness-specific per-repository metadata is
preserved for commands that understand it; basic commands such as `doctor`
do not reject unknown metadata fields.
