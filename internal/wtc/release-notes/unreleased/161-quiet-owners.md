## OpenClaw: collections

- **Adding repositories no longer ends in "Couldn't finish this reply".** A
  repository's owner session only holds its worktree, so `wtc add-repo` now
  spawns it with `expectsCompletionMessage: false`. Its start no longer comes
  back to the collection's conversation as an extra turn, which could be
  empty after the agent had already answered and was then shown as a failed
  reply. Workers still report back
  ([#161](https://github.com/lcorneliussen/wtc-cli/issues/161)).
