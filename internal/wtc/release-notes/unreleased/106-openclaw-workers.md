## OpenClaw: workers

- **Workers take the parallel parts of a task**
  ([#106](https://github.com/lcorneliussen/wtc-cli/issues/106)).
  `wtc add-repo <repo> --worker <w> --work "<part>"` spawns a hidden session
  per repository in a collection of its own, `<task>--<w>`, on branch
  `openclaw/<task>--<w>`. A worker starts from the task's branch where the
  task has one, and is measured and caught up against the task's base. An
  explicit `--base` may name a branch that exists only on the gateway, such as
  another worker's. Workers commit in their own checkouts only, never publish,
  and report what they did; OpenClaw tells the coordinating session when one
  finishes. A worker uses the task's harness for names and branches, without
  a harness worktree of its own.
- **`wtc catch-up --from <w>`** merges a worker's branch into the task's. The
  gateway's clone is shared, so nothing is fetched. It never rebases, and a
  conflict is aborted for the coordinator to merge by hand.
- **`wtc status`** lists the task's direct workers, each with how many of its
  commits are still to merge.
