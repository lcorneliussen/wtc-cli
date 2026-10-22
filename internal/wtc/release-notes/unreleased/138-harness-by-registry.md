## OpenClaw: harnesses

- **The registry picks the harness.** Asking for a repository that a configured harness lists, by name, URL or `owner/name`, starts the collection in that harness, with a worktree of the harness and of that repository; `defaultHarness` decides only between harnesses that list it, or when none does ([#138](https://github.com/lcorneliussen/wtc-cli/issues/138)).
- **A repository the harness does not list is the person's choice.** `wtc add-repo` stops and the agent asks: ad hoc beside the harness (`--ad-hoc`), registered in the harness first, or a collection of its own (`--harness none`) ([#138](https://github.com/lcorneliussen/wtc-cli/issues/138)).
- **The folder says which harness.** A collection's folder is `$OPENCLAW_STATE_DIR/wtc/<harness>/<name>`, or `wtc/ad-hoc/<name>` without one; folders under `wtc-collections/` keep working. A harness can no longer be called `ad-hoc` ([#138](https://github.com/lcorneliussen/wtc-cli/issues/138)).
