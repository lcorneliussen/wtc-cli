# Collection-state compatibility for the taba rename

This increment of [phase 0](https://github.com/lcorneliussen/wtc-cli/issues/201)
accepts alternate names for collection-local PR and status state. Check a
release's notes before assuming it includes these readers. No migration or
manual rename is required.

| State | Legacy name | Alternate name |
| --- | --- | --- |
| PR enlistments | `.wtc-prs` | `.taba-prs` |
| Canonical status snapshot | `.wtc-status.json` | `.taba-status.json` |
| Shell-compatible status fallback | `.last-wtc-status.yml` | `.last-taba-status.yml` |

An existing legacy file wins, including an empty file. Only an absent legacy
path permits the alternate name. Invalid or dangling legacy inputs are not
bypassed, and the files are not combined. Listing PRs, reading cached status
and checking cache age do not create or rename files.

Phase 0 continues to write legacy names. When an enlist, unlist or final-merge
recording operation starts with only `.taba-prs`, it reads that registry under
the existing `.wtc-prs.lock`, applies the change, and writes the complete result
to `.wtc-prs`. Existing rows, comments and merge facts carry forward. The
alternate source is left unchanged; subsequent reads select the legacy output.
This is a compatibility write, not a switch to new default names.

A completed status refresh still writes `.wtc-status.json`, `.wtc-status.md`
and `.last-wtc-status.yml`, making its new snapshot available to older readers.
Existing alternate snapshots are left unchanged. JSON snapshots take priority
over the YAML fallback, with legacy-name precedence within each format.

Catch-up has no cached-report reader: its default output remains
`.wtc-catch-up.json` plus `.wtc-catch-up.json.md`. An explicit
`--report .taba-catch-up.json` already works, as does any other report path.
Retirement recognizes both report names, `.taba-status.md`, and the alternate
PR/status files as disposable collection state.

The `.taba/` directory, live runner identity, runtime and watcher locks,
review-mode markers, review bundles and posted-review receipts remain separate
phase-0 work. Skill names, executable packaging and automatic migration belong
to later rename phases.
