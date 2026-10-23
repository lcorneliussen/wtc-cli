## OpenClaw: pull requests

- **After a merge, the card offers the catch-up.** A merged pull request whose
  merge the checkout doesn't have yet shows ↓, which fetches the base and
  merges it in. `wtc status` reports this as the pull request's `mergedIn`
  ([#144](https://github.com/lcorneliussen/wtc-cli/issues/144)).
- **The conversation acts on pull request news.** The event now asks the
  agent to bring the task's progress up to date, to catch up after a merge,
  and to tell the person only what they need to act on
  ([#144](https://github.com/lcorneliussen/wtc-cli/issues/144)).
