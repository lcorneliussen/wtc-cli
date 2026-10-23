## OpenClaw: wtc projects

- **The card suggests where to work.** In a conversation without a
  collection, the card reads the person's latest message and suggests a wtc
  project and its repositories: "Work in wtc project wtc with wtc-site?".
  **Start** asks the conversation to start that collection. The gateway's
  decision model makes the suggestion when one is selected; otherwise the
  message's words are matched against the repositories' names and roles
  ([#154](https://github.com/lcorneliussen/wtc-cli/issues/154)).
- **The `openclaw` CLI no longer reports the wtc plugin failing to
  register.** Without a runtime (CLI metadata, setup), the plugin registers
  nothing
  ([#154](https://github.com/lcorneliussen/wtc-cli/issues/154)).
