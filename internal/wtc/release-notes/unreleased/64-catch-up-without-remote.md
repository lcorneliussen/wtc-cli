## Catch-up

- An owner without any remote no longer drops out of catch-up.
  Its fetch row reads `no-remote`, which does not fail the run, and its
  worktrees are reconciled against the owner's own branches (`main` rather
  than `origin/main`). Status measures them against the same local branch and
  no longer tries to fetch them. When a configured `origin` cannot be reached,
  the fetch row now carries git's own error line instead of the owner path, and
  repo-scoped hooks such as secret linking still run
  ([#64](https://github.com/lcorneliussen/wtc-cli/issues/64)).
