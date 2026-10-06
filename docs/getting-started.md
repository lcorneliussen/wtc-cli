# Getting started

WTC needs Git and a harness repository describing the repositories in your
workspace. Mise manages tool versions and environment; it is optional. Herdr,
Neovim, and an agent CLI are optional interfaces.

## Install WTC

Install a [published release](https://github.com/lcorneliussen/wtc-cli/releases).
With mise:

```sh
mise use -g github:lcorneliussen/wtc-cli@0.1.39
```

The workspace root is an ordinary folder. Each collection contains a worktree
of your harness and the application repos needed for its task.

## Minimal harness

`wtc harness init` creates a small harness whose generic instructions come
from the CLI. Use **v0.1.39 or newer**: v0.1.38 does not include this command
or the complete embedded instruction fallback.

```sh
wtc harness init agent-harness \
  --remote git@github.com:example-org/agent-harness.git \
  --cli-version 0.1.39
```

The command requires an explicit version, creates six files, and refuses to
overwrite existing files. It does not initialize Git, publish, install tools,
or run hooks. Use `--dry-run` to preview the filenames.

Commit and publish the harness to your own remote. Then follow the
[versioned bootstrap guide](../internal/wtc/defaults/bootstrap.md) to create
its bare owner and first collection. Add application repositories to its
registry and use `wtc add-repo` or `wtc new` to bring in worktrees.

The recording uses local synthetic remotes to stay reproducible and offline;
normal workspace bootstrap clones the harness from your forge.

## Existing populated harness

Your own adaptation of the optional
[reference harness](https://github.com/lcorneliussen/wtc-boilerplate) remains
supported. Follow the [populated-harness bootstrap](released-bootstrap.md)
for its first bare owner and collection. That path also works with v0.1.38.
Use the [upgrade notes](release-notes.md) when changing an existing harness pin.

## Add a repository without changing ordinary development

The registry records its name, Git remote, development ref, and optional port
offset. `wtc add-repo api` brings it into the current collection. Setup hooks
are optional; introduce them only for collection-specific preparation.

Keep `bin/dev`, mise tasks, npm scripts, and other local commands usable outside
WTC. Make collection-specific setup optional so developers retain the same
local workflow.
