## OpenClaw: the card

- **A repository with nothing on its branch shows its base.** A gateway
  worktree is on its own `openclaw/<name>` branch from the start. Until
  something is on it (a commit, a change, a push or a pull request), the card
  shows its base (`main`) instead, as a detached worktree is shown in a local
  collection. The branch's name is in the tooltip
  ([#168](https://github.com/lcorneliussen/wtc-cli/issues/168)).
