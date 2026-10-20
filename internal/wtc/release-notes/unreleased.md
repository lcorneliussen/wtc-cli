# wtc unreleased

## OpenClaw

- **Workers** for the parallel parts of a task
  ([#106](https://github.com/lcorneliussen/wtc-cli/issues/106)):
  `wtc add-repo <repo> --worker <w> --work "<part>"` spawns hidden sessions
  with worktrees of collection `<task>--<w>`, based on the task's branch, that
  commit in their own checkouts and never publish. `wtc catch-up --from <w>`
  merges a worker's branch into the task's without a fetch, never rebasing,
  and `wtc status` lists each worker with what is left to merge.
