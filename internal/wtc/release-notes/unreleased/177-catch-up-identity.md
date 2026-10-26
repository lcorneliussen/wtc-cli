## OpenClaw: the card

- **The card's catch-up no longer fails with "Committer identity
  unknown".** It merges with the identity the agent's own commits get: its
  managed GitHub identity's `gitAuthor`, else the agent's name as
  `<name>@localhost`, else OpenClaw's
  ([#177](https://github.com/lcorneliussen/wtc-cli/issues/177)).
- **A catch-up that stops on a conflict goes to the conversation.** The card
  no longer tells you to merge by hand: it asks the agent to catch up in its
  own checkout and resolve the files there. In a sandbox with the gateway's
  git service, `wtc catch-up` stops on the conflict, and
  `wtc catch-up --continue` hands the resolved branch back to the gateway.
- A conflicting file whose name has a space stays whole in the conflict list.
