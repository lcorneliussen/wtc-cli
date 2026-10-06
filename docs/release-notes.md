# Release notes and harness upgrades

`wtc release-notes` is available from v0.1.39.

Release notes ship inside the CLI, so people and agents can read them offline
from any directory. The archive includes published versions from 0.1.0 onward.
Historical notes describe the commands and limitations at the time of release;
read the complete upgrade range for later corrections.

```sh
wtc release-notes                  # latest embedded numbered release
wtc release-notes 0.1.38           # one release; v0.1.38 also works
wtc release-notes --since 0.1.35   # newer numbered releases, oldest first
wtc release-notes unreleased      # upcoming changes, explicitly separate
wtc release-notes --list           # available versions
```

`--json` returns the usual envelope with `data` as an array of documents
(`version`, `content`); `--list --json` returns an array of version strings.
An empty upgrade range returns an empty array. `--since` excludes the baseline
and unreleased changes. It cannot be combined with a version or `--list`.

## Upgrade a harness

Read the committed `.wtc-cli-version` and the target release. Run the **target
binary** with `release-notes --since <old-pin>`: an old binary only contains its
own archive and cannot fetch future notes. Download the published target binary
without changing the harness pin first. For a binary that predates this command,
read its [GitHub release notes](https://github.com/lcorneliussen/wtc-cli/releases).

Use the notes to decide which features need local adoption and which contract
changes need tests. Compare local skills and instructions with the target's
defaults, including `wtc skills diff`. Update the CLI pin and compatibility
range together, regenerate the collection environment with `wtc env setup`,
run `mise install`, and re-render skills and MCP configuration. Preserve project
hooks, policy, and overlays; exercise changed behavior in a disposable collection.

The shipped `wtc-harness-upgrade` skill walks through the full procedure. Record
the version range, adoption decisions, and validation in the harness's upgrade
PR. Keep private adoption relationships in private records.

## Author a release

The source of truth is `internal/wtc/release-notes/<version>.md`. These files are
embedded directly; they are also passed to GoReleaser as the release body.

1. Keep upcoming changes in `internal/wtc/release-notes/unreleased.md`.
2. Before tagging, turn the relevant changes into `<version>.md`, headed
   `# wtc <version>` (without `v`). Include features, compatibility changes,
   harness adoption steps, and relevant limitations. Use generic public examples.
3. Clear the released items from `unreleased.md`, retaining its heading and an
   explicit statement about any remaining upcoming changes.
4. Run the Go suite and vet. Inspect the exact notes, diff, and release payload
   for public safety. Commit the numbered notes **before** creating the tag.
5. Follow the repository's normal review and release authorization procedure.

The tag workflow rejects missing or mismatched notes before publishing binaries.
It publishes the same file that is embedded in the tagged binary. Fix future
guidance in source rather than editing only the GitHub body: already downloaded
binaries keep their original notes. Historical files here were imported from
the published release bodies, with their top-level headings normalized.
