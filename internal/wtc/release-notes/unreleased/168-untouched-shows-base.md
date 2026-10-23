## OpenClaw: the card

- **A repository with nothing on its branch shows its base.** A gateway
  worktree is on its own `openclaw/<name>` branch from the start. Until
  something is on it (a commit, a change, a push or a pull request), the card
  shows its base (`main`) instead, as a detached worktree is shown in a local
  collection. The branch's name is in the tooltip
  ([#168](https://github.com/lcorneliussen/wtc-cli/issues/168)).
- **Catching up a branch with nothing on it says so.** When a gateway
  worktree's branch has no commits of its own, `wtc catch-up` fast-forwards
  it, and now reports it at `origin/<base>` again with nothing to publish,
  instead of "merged origin/main into openclaw/<name>; publish to send the
  merge" ([#168](https://github.com/lcorneliussen/wtc-cli/issues/168)).
