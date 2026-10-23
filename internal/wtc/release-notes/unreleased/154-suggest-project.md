## OpenClaw: wtc projects

- **The card suggests where to work.** In a conversation without a
  collection, the card reads what the person types, once they pause, or
  their latest message, and suggests a wtc project and its repositories:
  "Work in wtc project wtc with wtc-site?". **Start** asks the conversation
  to start that collection, sending a draft along. In the agent's main
  session it asks for a new conversation for the task instead. The gateway's
  decision model makes the suggestion when one is selected. Until OpenClaw
  releases decision models, the plugin asks OpenAI's Decisions API itself
  when the gateway has `WTC_DECISIONS_OPENAI_API_KEY`. Otherwise the
  message's words are matched against the repositories' names and roles
  ([#154](https://github.com/lcorneliussen/wtc-cli/issues/154)).
- **The `openclaw` CLI no longer reports the wtc plugin failing to
  register.** Without a runtime (CLI metadata, setup), the plugin registers
  nothing
  ([#154](https://github.com/lcorneliussen/wtc-cli/issues/154)).
- **No "wtc Collection" bar across every conversation.** The plugin no
  longer registers a header action, which OpenClaw showed as a bar even in a
  conversation without a collection. The card and the session menu open the
  panel ([#154](https://github.com/lcorneliussen/wtc-cli/issues/154)).
