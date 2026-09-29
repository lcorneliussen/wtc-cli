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

Make init safe to repeat: a repo can be added after collection creation, and
catch-up may repeat setup. Check whether a sibling exists before using it.
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

The CLI embeds generic `wtc-*` skills and instructions. A harness can run
`wtc eject 'skills/wtc-customize'` or another selected path to copy a default
into its own repository for editing. `eject` refuses to overwrite an existing
file. Keep repository-specific agent instructions in that repository's
`AGENTS.md` and hooks in its `.harness/` directory.
