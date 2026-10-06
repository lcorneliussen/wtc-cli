# A small harness, backed by the CLI

WTC should supply the common mechanics and guidance. A project should own the
configuration that makes its workspace different. Keeping copies of every
standard skill in every harness makes upgrades harder and hides which files
are deliberate policy overrides.

## The default from v0.1.39

A new harness starts with six files:

```text
.harness-repos.yml     repository names, remotes, and development refs
.wtc-cli-version      exact released CLI version
wtc.toml              compatibility and project configuration
.gitignore            local/generated harness files
AGENTS.md             policy for editing this harness
README.md             project setup notes
```

Add tool pins, hooks, secret path declarations, and policy only when the
project needs them. The CLI supplies the standard collection entry point,
instructions, and skills. Rendering puts effective instructions under
`.wtc/instructions/`, links project overrides from `harness/instructions/`, and
exposes skills to agent clients. `wtc docs` reads the binary's defaults; it does
not replace project policy.

Use `wtc eject <path>` to take ownership of a default. An ejected file becomes
a tracked override and stops automatically following that CLI default.
Existing harnesses with authored entry points and skills keep those overrides.

A harness remains a Git repo because its registry and policy need review,
history, and reproducible revisions beside the code. The CLI scaffolds that
repo; it does not own your repository list or publish the repo for you.

## What happens to the boilerplate?

Keep it as an optional reference during the transition. Make the CLI the
canonical source for generic guidance, examples, and demos. New projects can use the
minimal scaffold without a boilerplate fork.

For an existing harness:

1. Keep its current CLI pin and authored files while evaluating the CLI defaults.
2. Compare standard files with the embedded defaults using `wtc skills diff`
   and the source documents. Review project policy and hooks separately.
3. After installing a compatible release, remove only overrides that are truly
   redundant, in a normal project PR. Re-render and check agent instructions,
   environment, setup, and cleanup before adopting it.

No automatic deletion of project policy is part of this change. Archiving the
boilerplate can be a later decision if maintaining the reference no longer
helps. Its existence need not be a dependency of WTC itself.

## Application compatibility

Keep repository development commands usable outside WTC. Optional setup and
teardown hooks prepare worktree-specific files without replacing normal local
commands or toolchains.

The small scaffold and complete embedded instruction fallback are available
from v0.1.39. The earlier v0.1.38 bootstrap uses a populated harness.
