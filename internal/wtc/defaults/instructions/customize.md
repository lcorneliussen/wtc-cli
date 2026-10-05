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
`github:lcorneliussen/wtc-cli` to `[mise.tools]`. `wtc env setup --dry-run` previews
`.env.collection` and `mise.toml`; `wtc env setup` regenerates them. Put local values
and credentials in `.env.collection.local`, which the generator preserves.

## Harness hooks

Executable scripts in `harness/hooks/wtc/` run from the collection root. The
CLI sends one JSON object on stdin with `event`, `collection`, `harness`,
`workspace`, and `values`. It also sets `WTC_COLLECTION` and
`WTC_CONFIG_ROOT`. `WTC_COLLECTION` is the collection name; the JSON
`collection` field is its absolute directory. Current events are:

| Action | Before | After |
|---|---|---|
| `wtc env setup` | `env.pre.sh` | `env.post.sh` |
| `wtc pr enlist` | `pr.enlist.pre.sh` | `pr.enlist.post.sh` |
| `wtc pr unlist` | `pr.unlist.pre.sh` | `pr.unlist.post.sh` |
| `wtc registry refresh` | `registry.refresh.pre.sh` | `registry.refresh.post.sh` |
| `wtc mcp render` | `mcp.render.pre.sh` | `mcp.render.post.sh` |
| `wtc secrets link` | `secrets.link.pre.sh` | `secrets.link.post.sh` |
| `wtc skills render` | `skills.render.pre.sh` | `skills.render.post.sh` |
| `wtc new` | `new.pre.sh` | `new.post.sh` |
| `wtc add-repo` | `add-repo.pre.sh` | `add-repo.post.sh` |
| `wtc retire` | `retire.pre.sh` | `retire.post.sh` |

`wtc new` and `wtc add-repo` also run `env.pre.sh` and `env.post.sh`
around their environment generation, before secrets linking and repository
initialization. Environment aliases added by a post-hook are therefore
available to each repository's init hook.

A pre-hook can stop the action by exiting nonzero. A post-hook failure is
reported as a warning; the completed action remains complete. Dry runs and
read-only commands do not run hooks. Hooks should be idempotent and avoid
writing to other collections. For example, a harness may use `env.post.sh`
to regenerate a local tool configuration after the collection env changes.

### Status build facts

`wtc status` can read build results from an executable
`harness/hooks/wtc/status.build.sh`. This is a read-only data provider, called
once for each repository's development branch (`tier: "tip"`) and again for a
distinct `production_ref` (`tier: "prod"`). It is not a lifecycle hook. It
receives JSON on stdin:

```json
{"collection":"demo","repo":"widget","worktree":"/path/to/demo/widget","slug":"example/widget","branch":"main","tier":"tip"}
```

Return one JSON object with `checks` (`SUCCESS`, `FAILURE`, `PENDING`, or
`NONE`), optional `build` and `url`, and optionally the same `branch`:

```json
{"checks":"SUCCESS","build":"123","url":"https://example.invalid/build/123"}
```

An empty response or `null` hides that build cell. Keep the provider
read-only, fast, and safe to call during watched status refreshes. The CLI
stops a call after 15 seconds, rejects invalid JSON or a different branch,
and never infers a passing build when the provider cannot answer. The TUI
shows `T` and `P` cells only when build facts exist; clicking a cell opens its
HTTP(S) URL unless `--no-click` is set. A harness without this provider has
no build columns.

### Browse view

`wtc browse` ships its Neovim collection view inside the CLI release. To
replace that view for one harness, check in
`harness/overlays/browse/wtc-browse.lua`. The command loads the overlay in
place of the bundled view. Keep the file workspace-agnostic: the selected
collection path is available in `vim.g.wtc_browse_root`.

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
`wtc secrets link --repo <name>` rather
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

To replace only one H2 section, put a complete section (starting with its
exact `## Heading` and ending with a newline) in
`harness/overlays/skills/<name>/sections/<short-name>.md`. The heading must
occur exactly once in the base skill, and the patch cannot introduce another
H1 or H2. The base is `harness/skills/<name>/SKILL.md` when present, otherwise
the CLI's embedded skill. A full overlay `SKILL.md` and section patches for
the same skill are mutually exclusive. `wtc skills render --dry-run` validates
patches without writing them; rendering materializes the assembled skill in
the collection's disposable `.wtc/skills/` directory.

Use `wtc skills diff --changes` to review local skill changes against the
installed CLI. It prints the current base SHA-256 for untracked or drifted
overrides. Record that digest in
`harness/overlays/skills/<name>/.wtc-base.sha256` after reviewing a section
patch. If the base later changes, render stops and diff reports `drifted`;
review the patch against the new base and update the digest. A full skill
override can use the same sidecar file for drift reporting. Without a sidecar,
diff reports `untracked`. `--json` provides the same findings for automation.

`wtc eject 'skills/wtc-customize'` or another selected path copies an
embedded default into the harness for editing; eject refuses to overwrite an
existing file. Keep repository-specific agent instructions in that
repository's `AGENTS.md` and hooks in its `.harness/` directory.
