## OpenClaw: the card

- **The card's catch-up no longer fails with "Committer identity
  unknown".** It merges with the identity the agent's own commits get: its
  managed GitHub identity's `gitAuthor`, else the agent's name as
  `<name>@localhost`, else OpenClaw's
  ([#177](https://github.com/lcorneliussen/wtc-cli/issues/177)).
- **A catch-up that stops on a conflict goes to the conversation.** The card
  no longer tells you to merge by hand: it asks the conversation to merge the
  base in, resolve the conflicting files and commit, as ✗ on a conflicting
  pull request does.
