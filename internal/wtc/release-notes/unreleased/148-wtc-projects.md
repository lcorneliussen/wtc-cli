## OpenClaw: wtc projects

- **A harness is a wtc project.** The skill, the instructions, the docs,
  `wtc add-repo`'s questions and `wtc status` now call a configured
  harness a wtc project: a named set of repositories, whose collections are
  one task's worktrees of them. The panel names a collection's project, or
  "ad-hoc wtc project". The config keys and the folders stay as they are
  ([#148](https://github.com/lcorneliussen/wtc-cli/issues/148)).
- **`wtc projects`** lists the gateway's wtc projects with their
  repositories and roles. `wtc add-repo --project` is `--harness` under
  its new name, and the old one still works
  ([#148](https://github.com/lcorneliussen/wtc-cli/issues/148)).
- **Every task starts a collection.** The agent starts one whenever the
  person wants to work on a repository, also only to read or analyze it. It
  matches what they named with `wtc projects`, and adds the other
  repositories the project's AGENTS.md names for the task
  ([#148](https://github.com/lcorneliussen/wtc-cli/issues/148)).
- **Another project's repository suggests its own conversation.** When the
  collection's wtc project doesn't list a repository and another configured
  project does, `wtc add-repo` adds a fourth choice: work on it in a new
  conversation for that project. Projects stay apart: a collection never
  reads from another project's repositories unless they were added ad hoc
  ([#148](https://github.com/lcorneliussen/wtc-cli/issues/148)).
