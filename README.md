# wtc

A CLI for worktree collections. This repository is the beginning of the migration from the reference harness's shell tools to a versioned binary.

## Current commands

- `wtc env [--collection DIR] [--dry-run]` regenerates `.env.collection`, preserving its port base. It leaves `.env.collection.local` intact and writes the collection's mise environment file.
- `wtc doctor` checks the collection registry and local tool availability.
- `wtc commands --json` lists the command surface for agents.
- `wtc eject 'skills/wtc-*'` copies selected embedded defaults into the harness so they can be customized. Existing files are never overwritten.

Run locally with `go run ./cmd/wtc`. Commands that have not migrated remain in the reference harness. Tagged releases build macOS and Linux binaries through GoReleaser. A harness pins an exact release in its checked-in `.wtc-cli-version` file (for example, `0.1.1`). `wtc env` reads that file and writes the tool entry into the generated collection-root `mise.toml`, which every sibling inherits. Then run `mise install` in the collection. No global installation is required.

```toml
[tools]
"github:lcorneliussen/wtc-cli" = "0.1.1"
```

The pin shown here is an example; use a published release version. The shell environment generator in a harness must also preserve the same pin during migration.

`wtc` reads registries using `repos`, `repositories`, or `selected` plus
categorized `non_default` entries. Harness-specific per-repository metadata is
preserved for commands that understand it; basic commands such as `doctor`
do not reject unknown metadata fields.
