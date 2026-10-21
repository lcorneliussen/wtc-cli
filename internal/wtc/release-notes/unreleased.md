# wtc unreleased

## Status and catch-up

- A squash merge now counts as landed when its merge commit is on the target
  and has the same tree as the branch tip, or the same change as the
  branch's cumulative diff. Before, a PR whose head fact was unavailable stayed
  "merged PR landing not established".
- A status refresh of a collection now returns a clean worktree, still on the
  branch of a PR it sees merged, to the development tip once landing is
  established, and says so in the refresh log. Opt out with
  `WTC_STATUS_RETURN_MERGED=off` or `[status] return_merged = false` in
  `harness/wtc.toml`. Workspace sweeps (`--all`) never move worktrees
  ([#59](https://github.com/lcorneliussen/wtc-cli/issues/59)).
