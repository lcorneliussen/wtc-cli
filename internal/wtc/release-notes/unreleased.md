# wtc unreleased

## OpenClaw

- A gateway can be configured with harnesses, under
  `plugins.entries.wtc.config.harnesses` (and optionally `defaultHarness`).
  A collection works in one: its first `wtc add-repo` adds a managed worktree
  of the harness, and repository names resolve through the harness's
  `.harness-repos.yml`, with their default branches. When it is not clear
  which harness a collection belongs to, `wtc add-repo` tells the agent to
  ask, and takes `--harness <name>` or `--harness none`. `wtc status` shows
  the collection's harness and its worktree, and `wtc doctor` checks the
  config and that each harness is a project on the gateway
  ([#100](https://github.com/lcorneliussen/wtc-cli/issues/100)). When
  the gateway's config cannot be read, `add-repo` stops with the error
  rather than going on without a harness.
- A folder's own `.wtc/config` now decides that it is an ad-hoc collection,
  whatever its repositories are called, so a gateway's collection folder
  that links a repository called `harness` stays what it is. `add-repo`
  under OpenClaw checks the worktree name before it records anything.

## Catch-up

- An owner without an `origin` remote no longer drops out of catch-up.
  Its fetch row reads `no-remote`, which does not fail the run, and its
  worktrees are reconciled against the owner's own branches (`main` rather
  than `origin/main`). Status measures them against the same local branch and
  no longer tries to fetch them. When a configured `origin` cannot be reached,
  the fetch row now carries git's own error line instead of the owner path, and
  repo-scoped hooks such as secret linking still run
  ([#64](https://github.com/lcorneliussen/wtc-cli/issues/64)).
