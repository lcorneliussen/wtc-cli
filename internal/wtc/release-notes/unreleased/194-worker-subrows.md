## OpenClaw: workers

- **A worker is a sub-row of its repository.** The card and the panel show
  it under the repository it works on, with the same columns, measured
  against the task's branch instead of the forge: ± its uncommitted files,
  with a tooltip saying which the task has the same, which differ and which
  only the worker has; **↑** merges its commits into the task; **↓** catches
  it up with the task ([#194](https://github.com/lcorneliussen/wtc-cli/issues/194)).
- **Workers are merge-only.** `wtc publish` refuses on a worker's branch and
  `wtc svc` in a worker's collection: its work reaches the forge through the
  task. `wtc catch-up` in a worker's checkout merges the task's branch.
- The skill says that a part needing its own pull request or services is a
  collection of its own, not a worker.
