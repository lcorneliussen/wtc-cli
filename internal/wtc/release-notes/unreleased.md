# wtc unreleased

## Features

- Under an OpenClaw gateway `wtc` plans instead of acting: `wtc host` reports
  the host, `wtc add-repo` prints `sessions_spawn` intents, the new
  `wtc publish` checks a session's managed worktree and prints the intent that
  publishes it, and `wtc catch-up` merges without rebasing or pushing. See
  `docs/openclaw.md`. Behaviour in a terminal and under herdr is unchanged.
- `wtc checkouts` prints the status of checkouts as JSON, and
  `openclaw-plugin/` is an OpenClaw plugin that shows a collection's
  repositories, pending changes and pull requests in the Control UI.

Read `wtc release-notes` for the latest numbered release, or
`wtc release-notes --since <old-pin>` for an upgrade range.
