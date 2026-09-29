# wtc

A CLI for worktree collections. This repository is the beginning of the migration from the reference harness's shell tools to a versioned binary.

## Commands

- `wtc env [--collection DIR] [--dry-run]` regenerates `.env.collection`, preserving its port base. It leaves `.env.collection.local` intact and writes the collection's mise environment file.
- `wtc doctor` checks the collection registry and local tool availability.
- `wtc commands --json` lists the command surface for agents.
- `wtc eject 'skills/wtc-*'` copies selected embedded defaults into the harness so they can be customized. Existing files are never overwritten.
- `wtc pr path|list|enlist|unlist` manages the collection-local `.wtc-prs` file used by status and catch-up.
- `wtc registry refresh` regenerates the local bare-owner map and reports registry mismatches.
- `wtc mcp render [--dry-run]` renders the harness MCP registry into Claude, Cursor, and Codex config files for this collection.
- `wtc secrets link [--repo <name>] [--dry-run] [--include-prod]` links gitignored control-root files into checked-out worktrees, preserving displaced local files in collection backups.
- `wtc agent-env` prints shell exports for sibling mise toolchains and configured collection-local bins; `--write` refreshes `.env.toolchain`, `--print-path` prints its bin list, and `--wrap` handles PreToolUse JSON from stdin.
- `wtc customize` prints the versioned customization guide, including harness hooks and application repository init/teardown hooks.
- `wtc review status <repo> [pr-number] [--trusted-local]` reads the newest local review status comment and checks whether it covers the current PR head.
- `wtc review bundle <repo> [pr-number] --public --no-catch-up` builds a public-safe review bundle from current local refs and generic concerns at the base commit, using versioned CLI defaults for a bootstrap review.
- `wtc review run <bundle-dir>` launches separate concern agents, runs a lead agent, and writes a verdict and summary into the bundle.
- `wtc review post <bundle-dir>` posts or updates the review comment, records a local receipt, and adds deduplicated inline findings.
- `wtc review resolve <bundle-dir>` replies to and resolves inline review threads; without filters, it resolves every unresolved thread in the bundle.
- `wtc review ready <pr-number> [--repo NAME]` promotes a draft only after a current, locally posted passing review (or an explicit user override).

Run locally with `go run ./cmd/wtc`. Commands that have not migrated remain in the reference harness. Tagged releases build macOS and Linux binaries through GoReleaser. A harness pins an exact release in its checked-in `.wtc-cli-version` file (for example, `0.1.3`). `wtc env` reads that file and writes the tool entry into the generated collection-root `mise.toml`, which every sibling inherits. Then run `mise install` in the collection. No global installation is required.

```toml
[tools]
"github:lcorneliussen/wtc-cli" = "0.1.3"
```

The pin shown here is an example; use a published release version. The shell environment generator in a harness must also preserve the same pin during migration.

Additional collection-root mise tools belong in the harness's committed `wtc.toml` under `[mise.tools]`. The CLI retains them when regenerating `mise.toml`. The exact CLI pin remains in `.wtc-cli-version` and cannot be overridden there.

The customization guide is embedded in the binary (`wtc customize`), and a harness can eject `skills/wtc-customize` or other defaults for local editing. Executable `harness/hooks/wtc/*.sh` scripts can react to migrated CLI actions; application repositories keep their worktree setup and cleanup in `harness:init` and `harness:teardown` tasks or `.harness/` scripts.

`wtc` reads registries using `repos`, `repositories`, or `selected` plus
categorized `non_default` entries. Harness-specific per-repository metadata is
preserved for commands that understand it; basic commands such as `doctor`
do not reject unknown metadata fields.
