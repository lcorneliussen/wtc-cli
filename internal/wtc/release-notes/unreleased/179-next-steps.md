## Status: the next step

- **`wtc status --json` names each repository's next step.** A `next` field
  per repository says what should happen now, read from its state: resolve
  a merge in progress, catch up (predicting with `git merge-tree` whether
  merging the base would conflict), merge a conflicting pull request, fix
  failed checks, address requested changes, push, or merge an approved
  pull request. Each step says who does it (`by`: `wtc`, `agent`, or
  `person` for merging a pull request), who must agree first (`approve`:
  `person` for the gateway's push, or `agent` for a clean merge into a branch
  or one whose outcome is unknown), and what carries it out: a skill, a wtc command, or
  the gateway's own action. Its `key` changes only when the facts do. The
  same field is in the OpenClaw gateway's status
  ([#179](https://github.com/lcorneliussen/wtc-cli/issues/179)).
