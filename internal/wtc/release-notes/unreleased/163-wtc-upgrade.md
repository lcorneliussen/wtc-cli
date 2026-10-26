## Harness upgrades

- **`wtc upgrade` moves a harness to a newer wtc release.** Run in a
  collection of the harness, it resolves the latest release, or the one given
  with `--version`, and bumps `.wtc-cli-version` and the `wtc.toml`
  compatibility range together. It then refreshes the collection's mise
  config, installs the release, and prints that release's own notes since the
  old pin, ready to decide what to adapt or adopt. It commits nothing.
  `--dry-run` writes nothing and reads the published release notes
  ([#163](https://github.com/lcorneliussen/wtc-cli/issues/163)).
- **`wtc upgrade --rollout` rolls a merged upgrade out.** It catches every
  collection's harness up to the merged tip and runs `mise install` there,
  leaving collections with a dirty harness, a conflicting branch, or a busy
  agent to their owners and listing them
  ([#163](https://github.com/lcorneliussen/wtc-cli/issues/163)).

## Catch-up

- **`--idle-only` and `--install-tools`.** `wtc catch-up --idle-only` leaves
  a collection whose herdr agent is working or blocked to that agent, as
  `needs-owner`. `--install-tools` runs `mise install` after a harness's
  environment refresh
  ([#163](https://github.com/lcorneliussen/wtc-cli/issues/163)).
- The `wtc-catch-up` skill no longer says that only new collections get a
  regenerated `.env.collection`; `wtc env setup` refreshes existing ones.
