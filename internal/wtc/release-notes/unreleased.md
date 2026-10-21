# wtc unreleased

## Review

- **`wtc review run` works without harness prompt files.** A private bundle
  used to fail when the harness had no `review/prompts/concern.md` or
  `lead.md`. Each prompt now falls back to the embedded default on its own, so
  a harness can override just one of them
  ([#46](https://github.com/lcorneliussen/wtc-cli/issues/46)).
