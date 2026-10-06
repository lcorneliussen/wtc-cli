# wtc — one workspace per task

A worktree collection puts the repositories needed for one task in a single
folder. Start an agent or editor there and it sees the API, frontend, and
shared library at the right revisions. Start another task alongside it with
its own worktrees and ports.

WTC manages the workspace around your projects: Git worktrees, environment,
agent guidance, PR status, and cleanup. Projects keep their usual build and
run commands.

![Two repositories, one task](docs/demos/collections.gif)

[Watch the recordings or reproduce them](docs/demos/README.md).

## Everyday use

From an existing collection:

```sh
wtc new fix-login api web --no-open
cd ../fix-login
wtc status --no-watch
wtc add-repo shared
# Make changes, commit, and open PRs in each repository.
wtc retire .
```

Worktrees start detached at their development tip. Create a branch before
committing. Retirement checks for local work before removing worktrees;
remote branches remain available.

Use an ordinary editor and terminal, or `wtc open` for a herdr workspace with
agent, browse, shell, and status panes. `/wtc-start` is the agent orientation
procedure; service startup uses `wtc up` in the runtime candidate.

## Install and set up

Install a [published release](https://github.com/lcorneliussen/wtc-cli/releases)
for macOS or Linux. With mise, for example:

```sh
mise use -g github:lcorneliussen/wtc-cli@0.1.38
```

An existing harness pins its CLI in `.wtc-cli-version`. `wtc env setup`
generates collection-local mise configuration from that pin and the harness's
`[mise.tools]`; run `mise install` in the collection. Your project toolchains
can continue to use their own mise files.

For a first workspace, read [getting started](docs/getting-started.md).
The [boilerplate](https://github.com/lcorneliussen/wtc-boilerplate) remains an
optional reference harness. The smaller, config-only harness and `wtc harness
init` are being tested on this branch; v0.1.38 does not contain them.

## Who owns what?

| Location | Responsibility |
| --- | --- |
| `wtc-cli` | Collection operations, standard instructions and skills, bootstrap tooling |
| Your harness repo | Repository registry, CLI pin, tool configuration, project policy and hooks |
| Your application repos | Normal development commands; optional WTC adapters |
| Each collection | Generated environment and agent files, local overrides, runtime logs |

A harness is versioned project configuration. It does not need to be a fork
of a large template. Read the [harness design](docs/harness-design.md) for the
migration boundary and the role of the boilerplate.

## Services and resources — candidate

The opt-in runtime uses [dekit](https://github.com/pvolok/dekit) 0.10 to run
services independently of agents. WTC associates each service and tunnel with
its owning repo, exposes status and persistent logs, and calls project-owned
provision/teardown hooks. `down` stops processes and preserves resources;
retirement tears down collection-owned resources before removing worktrees.

![Service lifecycle and grouped endpoints](docs/demos/runtime.gif)

See the [onboarding guide](internal/wtc/defaults/instructions/runtime.md),
[standalone example](examples/runtime/README.md), and
[cleanup recording](docs/demos/README.md#cleanup-failure).
The recorded tunnel is a loopback relay. Real cross-machine tunnels and cloud
resources use your existing commands and provider-specific hooks.

## Find the details

- [Command reference](docs/commands.md)
- [Configuration and hooks](internal/wtc/defaults/instructions/customize.md)
- [Workspace geometry](internal/wtc/defaults/instructions/worktree-workspace.md)
- [Environment and ports](internal/wtc/defaults/instructions/hooks-and-env.md)
- [Secrets](internal/wtc/defaults/instructions/secrets.md)
- [Agent instructions and skills](internal/wtc/defaults/instructions/skills.md)
- [Development and validation](docs/development.md)

The installed customization guide is available with `wtc customize`.
The candidate also exposes all embedded guidance through `wtc docs`.
