# wtc unreleased

## Added

- `wtc dev install` builds the wtc-cli checkout in a collection into `.wtc/dev/wtc`. From this release on, any `wtc` started inside that collection hands over to the dev build while it is the same version or newer, so panes, agents and hooks run the code under development without PATH changes. `wtc dev status` shows which build is in effect, and `wtc dev uninstall` removes it ([#79](https://github.com/lcorneliussen/wtc-cli/issues/79)).

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
  with their tunnels on one line, started, stopped and restarted one by one or
  all at once, tunnels opened and closed with `wtc svc tunnel`), laid out as
  `wtc status`'s tables and wrapping each row into two lines when narrow, and
  a panel with the same tables, the pull requests and adding repositories from
  the gateway, GitHub or the person's Bitbucket workspaces. Pushing from them
  goes through the gateway with the person's own GitHub connection, after they
  confirm; the card says how to connect one. `wtc checkouts` prints
  the status of checkouts as JSON for it. A new tunnel can also be the
  gateway's own OpenClaw portal, offered last as "needs ingress" when the
  gateway has no portal ingress.
- `wtc svc tunnel <service> tailscale --public` now says that Tailscale
  tunnels are tailnet-only and names cloudflare, ngrok and devtunnel for a
  public link.
