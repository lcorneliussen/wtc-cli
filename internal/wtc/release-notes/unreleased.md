# wtc unreleased

## Added

- `wtc status` follows builds without a harness hook. The test tier is each repository's `default_ref` branch, and the prod tier is `production_ref` (or `release_ref`). GitHub builds come from the branch head's check rollup, and Bitbucket builds from the newest branch pipeline, using the `bb` login or `BB_USERNAME`/`BB_API_TOKEN`.
- Merged PRs are followed again: the PR table shows the tier still waiting for a passing build that contains the merge (`→ test`, `● prod`, `✗ test`, `✓ built`). The follow starts at the tier that builds the PR's base branch; PRs merged into other branches are not followed. The merge commit and base are recorded in `.wtc-prs` as a separate `# merged-commit` row.
- Repository rows show a build as `○` when their HEAD is not in it.
- `status.build.sh` may return `sha`, the commit a build ran on, and `{"use_default":true}` to hand one request to the built-in provider. Requests now include `forge`.

## Fixed

- Long conflicting PR numbers fit narrow TUI rows while retaining the conflict marker and aligned click target, including linked numbers at the 40-column boundary.
- Review posting skips GitHub inline findings outside the PR diff and position-related HTTP 422 rejections, recording `skipped: not in diff` while keeping the finding in the summary ([#82](https://github.com/lcorneliussen/wtc-cli/issues/82)). Authentication, network and other validation failures still leave the review pending, with forge diagnostics retained for retry.
- PR merge conflicts are explicit in `wtc status`: repository rows show `✗ conflict` in the PR cell and retain the upstream count with a conflict marker (for example, `✗3`). Compact views keep the warning visible even when sync columns are hidden. GitHub's `mergeable` field detects conflicts even when its merge-state label says draft; passing checks and a clean local checkout do not hide the conflict.
- The TUI and terminal tables show TEST and PROD columns only for tiers with build facts, as documented ([#58](https://github.com/lcorneliussen/wtc-cli/issues/58)).
- A failing or invalid build provider marks only its cell as an error; it no longer fails the whole status refresh.
- The provider is called once per repository and tier, at most four at a time, instead of once per worktree row in sequence.

## Compatibility

- Harnesses whose `production_ref` equals `default_ref` now get one `prod` tier for that branch instead of one `tip` tier. A hook that keys on `tier` should answer `prod` for it.
- Harnesses with a `status.build.sh` keep it as the only provider. Return `{"use_default":true}` to opt individual repositories or tiers into the built-in one.
- A repository whose forge reports no checks or pipelines shows no build, as before. To turn the built-in provider off for a harness, add a `status.build.sh` that prints nothing.

## Ad-hoc collections

- `wtc init` makes an empty folder a collection without a harness repository.
  Its registry, `.wtc/config/repos.yml`, is local and tracked nowhere;
  `.wtc/config/` also takes `wtc.toml`, hooks and overrides.
- In an ad-hoc collection, `wtc add-repo` accepts a clone URL or a path to an
  existing checkout. The first add asks for the development branch and PR
  target (`--target` answers it). Clones are shared per user under
  `$XDG_DATA_HOME/wtc/bare`, and added repositories are remembered by name for
  later ad-hoc collections.
- `wtc retire .` retires an ad-hoc collection in place when Herdr is not in use.
- `wtc new <slug> [repo ...]` from an ad-hoc collection creates a sibling
  ad-hoc folder. Named repositories keep their registration (owner, tip,
  ports); `--tracker` and `--branch` work as usual, while `--issue` and
  `--pr` need a harness registry.
- In ad-hoc collections, every `--all` sweep, `add-repo --collection`, and
  `open`, `status`, `browse`, `catch-up` or `retire` of another collection
  are refused for now. Commands given an explicit `--collection <path>` act
  on that path as if run from it.

Harness-backed workspaces are unchanged and need no harness upgrade, with one
exception: `wtc retire` now also removes the generated `.wtc-catch-up.json`
report and its `.md` companion, which used to stay behind as leftovers.
