## OpenClaw: the card

- **Workers have a section of their own.** A worker's checkouts no longer
  show as a second row of the task's repository. Per worker and repository,
  the card and the panel show its uncommitted files and the commits the
  task hasn't merged yet. **⇢** merges them into the task's checkout, and
  **✕** discards the worker after asking again
  ([#182](https://github.com/lcorneliussen/wtc-cli/issues/182)).
- The skill keeps workers for parts that really run in parallel, and has
  the agent merge a worker or say what's left when it ends.
