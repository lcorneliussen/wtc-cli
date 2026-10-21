## Release notes

- **Unreleased notes are one file per change.** Each change adds
  `internal/wtc/release-notes/unreleased/<issue>-<slug>.md` instead of editing
  a shared `unreleased.md`, so branches in flight no longer conflict over
  release notes. `wtc release-notes unreleased` (and `--list`, `--json`) shows
  the fragments assembled into one document, as before. `go run
  ./tools/release-notes cut <version>` writes the numbered notes from the
  fragments and deletes them, and the release workflow fails while fragments
  remain ([#90](https://github.com/lcorneliussen/wtc-cli/issues/90)).
