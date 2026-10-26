## OpenClaw: one focus per collection, sub-sessions shown

- **Parallel work is a collection of its own.** A collection has one branch
  and one pull request per repository. The skill and the instructions have
  the agent propose a new conversation with its own collection for parallel
  work or a second pull request, instead of a worker. `wtc add-repo --worker`
  still works but is deprecated
  ([#194](https://github.com/lcorneliussen/wtc-cli/issues/194)).
- **Sub-sessions show under their repository.** A child session with a
  worktree of its own, whether a wtc worker or one OpenClaw or the agent
  spawned, is a sub-row under the repository, compared with the collection's
  checkout instead of the forge: ± its uncommitted files, with a tooltip
  saying which the collection has the same, which differ and which only it
  has; **↑** merges its commits in; **↓** catches it up; **✕** discards it.
  It never has a pull request.
- `wtc catch-up --from <branch>` merges another branch of the repository by
  its name, and `wtc status <name> --sub <worktree>` compares a worktree with
  the collection's.
- On a worker's branch `wtc publish` refuses and `wtc svc` won't start
  services; `wtc catch-up` there merges the collection's branch.
