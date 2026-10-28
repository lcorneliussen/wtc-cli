# CLI version in status

The live status header starts with the version of the running CLI, including
before the first refresh completes and while a refresh fails or runs offline.
It comes from the process's build version, independently of the harness pin,
installed binaries and cached snapshot metadata. No version probes or network
requests run on refresh.

Stable versions link to that version's GitHub release notes using a native
terminal hyperlink (modifier-click in supporting terminals). `--no-click`
disables the TUI's ordinary mouse targets; native terminal hyperlinks remain.
`NO_COLOR` preserves links, while `TERM=dumb` renders plain text. Development,
prerelease and custom builds show their build label without inventing a release
URL. A missing version appears as `unknown`.

Local JSON snapshots add the optional `cli_version` field without changing
schema 1. Markdown and one-shot tables show the same snapshot producer version.
Reading `--cached` preserves the producer's version; older snapshots without
the field show an unknown producer. The live TUI always shows its own running
version, even when it initially displays rows from an older snapshot.

This slice covers version visibility for #92 and the local snapshot observation
for #133. Dev source freshness, replaced-running-binary detection and ignored
dev-build diagnostics remain open under #92. Existing gateway version and
plugin mismatch displays are unchanged.
