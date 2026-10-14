# wtc unreleased

## Added

- `wtc dev install` builds the wtc-cli checkout in a collection into `.wtc/dev/wtc`. From this release on, any `wtc` started inside that collection hands over to the dev build while it is the same version or newer, so panes, agents and hooks run the code under development without PATH changes. `wtc dev status` shows which build is in effect, and `wtc dev uninstall` removes it ([#79](https://github.com/lcorneliussen/wtc-cli/issues/79)).
- `wtc status` follows builds without a harness hook. The test tier is each repository's `default_ref` branch, and the prod tier is `production_ref` (or `release_ref`). GitHub builds come from the branch head's check rollup, and Bitbucket builds from the newest branch pipeline, using the `bb` login or `BB_USERNAME`/`BB_API_TOKEN`.
- Merged PRs are followed again: the PR table shows the tier still waiting for a passing build that contains the merge (`→ test`, `● prod`, `✗ test`, `✓ built`). The follow starts at the tier that builds the PR's base branch; PRs merged into other branches are not followed. The merge commit and base are recorded in `.wtc-prs` as a separate `# merged-commit` row.
- Repository rows show a build as `○` when their HEAD is not in it.
- `status.build.sh` may return `sha`, the commit a build ran on, and `{"use_default":true}` to hand one request to the built-in provider. Requests now include `forge`.

## Fixed

- The TUI and terminal tables show TEST and PROD columns only for tiers with build facts, as documented ([#58](https://github.com/lcorneliussen/wtc-cli/issues/58)).
- A failing or invalid build provider marks only its cell as an error; it no longer fails the whole status refresh.
- The provider is called once per repository and tier, at most four at a time, instead of once per worktree row in sequence.

## Compatibility

- Harnesses whose `production_ref` equals `default_ref` now get one `prod` tier for that branch instead of one `tip` tier. A hook that keys on `tier` should answer `prod` for it.
- Harnesses with a `status.build.sh` keep it as the only provider. Return `{"use_default":true}` to opt individual repositories or tiers into the built-in one.
- A repository whose forge reports no checks or pipelines shows no build, as before. To turn the built-in provider off for a harness, add a `status.build.sh` that prints nothing.
