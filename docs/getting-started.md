# Getting started

WTC needs Git and a harness repository describing the repositories in your
workspace. Mise manages tool versions and environment; it is optional. Herdr,
Neovim, and an agent CLI are optional interfaces.

## With a released CLI

Use a [published release](https://github.com/lcorneliussen/wtc-cli/releases) and
a populated harness, such as your own adaptation of the optional
[reference harness](https://github.com/lcorneliussen/wtc-boilerplate).
Follow its [released bootstrap](released-bootstrap.md)
for the first bare owner and collection. This path works with v0.1.38.

The workspace root is an ordinary folder. Each collection contains a worktree
of your harness and the application repos needed for its task.

## Minimal harness — candidate

`wtc harness init` and the complete embedded instruction fallback are new on
this branch. Do not use a v0.1.38 pin for the minimal path: that release does
not render all of its instruction dependencies. Test with the candidate binary;
wait for a release containing this path before adopting it with mise.

```sh
wtc harness init agent-harness \
  --remote git@github.com:example-org/agent-harness.git \
  --cli-version <release-containing-the-minimal-harness>
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

## Add a repository without changing ordinary development

The registry records its name, Git remote, development ref, and optional port
offset. `wtc add-repo api` brings it into the current collection. Setup hooks
are optional; introduce them only for collection-specific preparation.

Keep `bin/dev`, mise tasks, npm scripts, and other local commands usable outside
WTC. For the runtime candidate, add a small task fragment that wraps the existing
foreground launch command. Start with the [onboarding guide](../internal/wtc/defaults/instructions/runtime.md)
and [synthetic example](../examples/runtime/README.md). Test standalone startup,
parallel collections, logs after shutdown, and teardown failure before adopting
it for real resources.
