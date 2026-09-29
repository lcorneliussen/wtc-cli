# Customize a worktree collection

Run `wtc customize` to read this guide from the installed CLI release. Keep
collection-specific choices in the harness repository and application setup in
each application repository. Generated collection-root files are disposable.

## Harness configuration

The harness checks in `.harness-repos.yml`, `.wtc-cli-version`, and `wtc.toml`.
The registry names repos, remotes, default refs, and optional port offsets.
`wtc.toml` declares CLI compatibility and extra collection-root mise tools.
For example, the tool entries can be:

```toml
[mise.tools]
bun = "1.2.3"
"npm:example-cli" = "4.5.6"
```

For collection-local commands outside mise, list existing directories relative
to the collection root. `wtc agent-env` puts these paths before sibling mise
bins, skips absent directories, and refreshes `.env.toolchain` when `wtc.toml`
changes:

```toml
[agent_env]
prepend_paths = ["tools/bin"]
```

The committed `.wtc-cli-version` supplies the exact `wtc` version. Do not add
`github:lcorneliussen/wtc-cli` to `[mise.tools]`. `wtc env --dry-run` previews
`.env.collection` and `mise.toml`; `wtc env` regenerates them. Put local values
and credentials in `.env.collection.local`, which the generator preserves.

## Harness hooks

Executable scripts in `harness/hooks/wtc/` run from the collection root. The
CLI sends one JSON object on stdin with `event`, `collection`, `harness`,
`workspace`, and `values`. It also sets `WTC_COLLECTION` and
`WTC_CONFIG_ROOT`. `WTC_COLLECTION` is the collection name; the JSON
`collection` field is its absolute directory. Current events are:

| Action | Before | After |
|---|---|---|
| `wtc env` | `env.pre.sh` | `env.post.sh` |
| `wtc pr enlist` | `pr.enlist.pre.sh` | `pr.enlist.post.sh` |
| `wtc pr unlist` | `pr.unlist.pre.sh` | `pr.unlist.post.sh` |
| `wtc registry refresh` | `registry.refresh.pre.sh` | `registry.refresh.post.sh` |
| `wtc mcp render` | `mcp.render.pre.sh` | `mcp.render.post.sh` |
| `wtc secrets link` | `secrets.link.pre.sh` | `secrets.link.post.sh` |
| `wtc skills render` | `skills.render.pre.sh` | `skills.render.post.sh` |
| `wtc new` | `new.pre.sh` | `new.post.sh` |
| `wtc add-repo` | `add-repo.pre.sh` | `add-repo.post.sh` |

A pre-hook can stop the action by exiting nonzero. A post-hook failure is
reported as a warning; the completed action remains complete. Dry runs and
read-only commands do not run hooks. Hooks should be idempotent and avoid
writing to other collections. For example, a harness may use `env.post.sh`
to regenerate a local tool configuration after the collection env changes.

## Hooks in an application repository

The existing collection lifecycle runs an `init` hook when it creates or adds
a repository worktree, and a `teardown` hook before removing it. Each
application repository can check in either a `mise.toml` task named
`harness:init` / `harness:teardown`, or executable `.harness/init.sh` /
`.harness/teardown.sh` files. The mise task takes precedence when mise is
available. Hooks run with cwd set to that repository worktree. They can read
the collection env through mise, or source `../.env.collection` directly.

```toml
[tasks."harness:init"]
run = "./.harness/init.sh"

[tasks."harness:teardown"]
run = "./.harness/teardown.sh"
```

`wtc add-repo` links the new repository's gitignored control-root files before
running init, then refreshes skills and MCP configuration. The hook can use
those files immediately. Make init safe to repeat: a repo can be added after
collection creation, and catch-up may repeat setup. Check whether a sibling
exists before using it.
Keep teardown limited to resources owned by that worktree. The harness warns
and continues if a repository hook fails, so the hook should report incomplete
setup clearly on stderr. To link control-root files, call
`wtc secrets link --repo <name>` or the harness's compatibility shim rather
than making links independently. The command refuses a target without a git
ignore rule and backs up an existing regular file before replacing it.

Production-capable files stay out of worktrees unless `--include-prod` is
explicit. Declare their paths relative to each repository in its registry
entry's `prod_paths`, or use `[secrets] prod_paths` in `wtc.toml` with
`repo/path` entries:

```toml
[secrets]
prod_paths = ["api/.env.production"]
```

## Skills and instructions

The CLI embeds generic `wtc-*` skills and instructions. `wtc skills render`
materializes embedded skills in the disposable collection, links them into
`.claude/skills` and `.agents/skills`, and wires `AGENTS.md`, agent hooks,
`.envrc`, and `.env.toolchain`. `--seed-scope` creates `WTC-SCOPE.md` from the
harness template only when it is absent. `--dry-run` previews changes, and
`--all` explicitly applies them to every collection in the workspace.

For a skill with local wording, check in `harness/skills/<name>/SKILL.md`;
it replaces the embedded default. A file at
`harness/overlays/skills/<name>/SKILL.md` takes precedence over both. New
skills in either directory are linked as well. A real directory already at an
agent client's skill destination is a local override and is left alone. The
renderer prunes only stale symlinks it owns.

`wtc eject 'skills/wtc-customize'` or another selected path copies an
embedded default into the harness for editing; eject refuses to overwrite an
existing file. Keep repository-specific agent instructions in that
repository's `AGENTS.md` and hooks in its `.harness/` directory.
