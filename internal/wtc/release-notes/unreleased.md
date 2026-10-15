# wtc unreleased

## OpenClaw

- Under an OpenClaw gateway `wtc` plans instead of acting: `wtc host` reports
  the host, `wtc add-repo` prints `sessions_spawn` intents, the new
  `wtc publish` checks a session's managed worktree and prints the intent that
  publishes it, and `wtc catch-up` merges without rebasing or pushing. See
  `docs/openclaw.md`. Behaviour in a terminal and under herdr is unchanged.
- A collection is driven from one conversation. Each repository's worktree is
  held by a hidden owner session, and `wtc status` keeps one folder per
  collection with a link to each worktree, where `wtc svc` runs its services.
  Under the gateway it fetches each clone's base branch at most every five
  minutes, so "behind" is current. `wtc init` and `wtc new` refuse there.
- `wtc add-repo` takes GitHub and Bitbucket URLs. The gateway clones GitHub
  repositories itself; a Bitbucket one has to be added in the Control UI first.
- `openclaw-plugin/` is an OpenClaw plugin for the Control UI: a collection
  card above the message box (each repository's branch, what is not pushed,
  pull requests with CI, conflict and review state, what is behind, services
  with their tunnels on one line), a panel with Push + PR and Catch up per
  repository, and adding repositories from the
  gateway, GitHub or the person's Bitbucket workspaces. `wtc checkouts` prints
  the status of checkouts as JSON for it.
