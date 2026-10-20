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
  ([#100](https://github.com/lcorneliussen/wtc-cli/issues/100)).
