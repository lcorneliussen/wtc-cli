# wtc unreleased

These changes are on main and are not part of v0.1.38. Read the numbered notes
for published releases; this document is a preparation guide for the next release.

## Features

- `wtc harness init` scaffolds six project-owned files. Generic collection
  instructions come from the CLI, and `wtc docs` exposes them outside a collection.
- The public README has collection, environment, secrets, and harness recordings
  using real commands against synthetic repositories.
- Herdr layouts repair narrow and wide pane arrangements consistently.
- `wtc release-notes` reads the embedded release archive offline. Use
  `wtc release-notes --since 0.1.35` to read newer releases in upgrade order,
  or `wtc release-notes unreleased` to see this preparation guide.
- Release publishing requires a committed version-specific note and uses that
  same Markdown for the GitHub release body.

## Harness upgrade

Existing harnesses can keep their configuration and hooks. The small scaffold
is optional: it does not replace local policy or copy over an existing harness.
Review local instruction and skill overrides before moving duplicated generic
guidance to CLI defaults. Read `wtc docs`, inspect `wtc skills diff`, and retain
project-specific sections in overlays or harness-owned instructions.

For each CLI upgrade, run the **target binary** with
`release-notes --since <old-pin>` before changing the harness pin. Older installed
binaries cannot describe future releases. Review the changed commands, hooks,
and compatibility notes; update the pin and compatibility range together, then
regenerate the collection environment, install its tools, and render guidance.
Test the harness's affected hooks and workflows in a disposable collection.
