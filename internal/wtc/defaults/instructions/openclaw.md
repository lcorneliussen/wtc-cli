# Under OpenClaw

`wtc host` prints `openclaw` when a command runs under an OpenClaw gateway: in
an agent's shell, whatever tool runs it, or in the Control UI's terminal. There
the gateway owns worktrees and publishing:

- a command in the agent's shell holds no git credential;
- the gateway publishes only a worktree it created, for the session that owns
  it, and each session owns at most one.

## One conversation, many repositories

A task is driven from **one conversation**: the one it started in. For each
repository, `wtc add-repo` has the gateway create a managed worktree on branch
`openclaw/<name>`, held by a **hidden owner** session that does nothing on its
own. `<name>` is the collection's `<issue-id>-<slug>`: lowercase letters,
digits and hyphens. You work in all the worktrees yourself, from the
**collection folder**: one link per repository to its worktree.

    wtc status <name>          # the folder, each repository, its PR, services
    cd <folder>/<repo>         # edit, build, test, commit here

`<repo>` is a project on the gateway, or a GitHub `owner/name` or URL that the
gateway has cloned. If it has not, `wtc add-repo` says so: ask the person to
add it with **Add repository** in the Collection panel, then add it again. A
separate conversation per repository is only for when the person asks for one
(`--separate`, or a spawn of your own with `visible: true`). Parallel parts of
the task go to workers (below).

## Workers: parts of a task in parallel

When parts of a task can go on in parallel, give each part a **worker**: a
session with checkouts of its own, never yours. Two writers in one checkout
fight over the index and build each other's half-made state. A worker gets
the collection `<name>--<worker>`, on branch `openclaw/<name>--<worker>`,
based on your branch where you have one, so it builds on your groundwork.

    wtc add-repo api --worker paging --work "Add paging to the list endpoint, with tests."
    # call sessions_spawn once per intent; each worker reports back when it ends
    wtc status <name>                      # workers, and what is left to merge
    cd <folder>/api && wtc catch-up --from paging   # merge its branch into yours

- One `--worker` per part, with every repository that part needs. Its
  sessions work, build, test and commit in their own worktrees, and are told
  never to publish.
- `wtc catch-up --from <worker>` merges the worker's branch into yours with
  no fetch: the worktrees share the gateway's clone. It never rebases; a
  conflict is aborted for you to merge by hand.
- Publishing stays the task's: the person pushes your branch with ↑, with
  the workers' work merged in. Workers' branches are not published.
- Readers (review, research, a test run) need no worker: they can read your
  checkouts.

## The collection's harness

A gateway can be configured with harnesses: repositories with a registry of
the repositories that belong together, their default branches, and the
instructions for working in them. A collection works in at most one. Its
first `wtc add-repo` adds a worktree of the harness too, and a name like
`api` resolves through the harness's `.harness-repos.yml`: that entry's
remote, based on its `default_ref` unless `--base` says otherwise.

`wtc add-repo` takes the collection's harness when it has one. Otherwise the
registries decide: the harness that lists the repositories asked for, by
name, URL or `owner/name`; between several, the gateway's default. When none
lists them, the default or the only harness configured. When that does not
settle it, it says so: ask the person which harness the task belongs to, then
pass `--harness <name>`, or `--harness none` for a collection without one.
One collection never mixes harnesses; work in another one is another
collection.

A repository the harness does not list stops `wtc add-repo` with three
choices. Put them to the person and run the one they pick: ad hoc beside the
harness (`--ad-hoc`), registered in the harness first (an entry in
`.harness-repos.yml` in the collection's worktree of the harness, committed
there), or a collection of its own (`--harness none` and another name).

The collection's folder says which harness it works in:
`$OPENCLAW_STATE_DIR/wtc/<harness>/<name>`, or `wtc/ad-hoc/<name>` without
one.

`wtc status <name>` shows the harness and its worktree. Read its `AGENTS.md`
and the instructions it points to before working in the collection: they are
the environment's rules, and they win over general habits. Changes to the
harness itself are work like any other: commit in its worktree, and the
person publishes them.

Never `wtc init` or `wtc new` under OpenClaw (wtc refuses): the gateway can
neither see nor publish a folder collection of its own.

`wtc` does not act on the gateway's behalf. It checks, then prints a **plan**:
JSON on stdout with the `intents` for you to carry out, and one line per
intent on stderr saying how. An intent with a `tool` means: call that tool
with the intent's `args`, unchanged, in the session `actor` names. Do not edit
the args, and do not do the work some other way.

| You want to | Run | Then |
|---|---|---|
| add a repository | `wtc add-repo <repo> --worktree <name>` | call `sessions_spawn` once per intent |
| see the collection | `wtc status [<name>]` | — |
| check before publishing | `wtc publish --title "<what changed>"` in `<folder>/<repo>` | — |
| publish | nothing: tell the person it is ready. They push it with **↑** for the repository in the collection card, as themselves, after confirming | — |
| take in upstream changes | `wtc catch-up` in `<folder>/<repo>` | publish again to send the merge |
| run a service | `wtc svc add <name> --port-env <VAR> -- <cmd>` in the folder | `wtc svc` shows it; the panel too |

## Publishing safely

`wtc publish` changes nothing. It refuses when publishing would go wrong or
publish the wrong thing:

- **outside a managed worktree**: you are in the parent, or in a checkout the
  gateway did not create. Publish from the repository's child session.
- **history that was published has been rewritten** (rebase, amend, reset).
  The gateway never rewrites a published branch. Put the changes back on top
  of the published commit, or continue in a new child session.
- **a merge or rebase is unfinished**, or conflicts are unresolved.
- **files that look like credentials are uncommitted** (`.env`, `*.pem`,
  `id_rsa`, …). On GitHub the gateway commits *every* pending file in the
  worktree, so read the plan's `pending` list before you call the tool:
  anything that is not part of the work has to be removed or ignored first.

**Pushing is the person's.** The collection card's ↑ has the gateway push
the repository with their own GitHub or Bitbucket account, after they
confirm. On GitHub the gateway commits what is pending first, so the
`pending` list matters there too; on Bitbucket only what is committed is
pushed, so commit first. Either way it opens or updates a **draft** pull
request. People merge.

Only when the person explicitly asks *you* to publish, carry out the plan
yourself, in the repository's session (`sessions_send` its owner: "Run wtc
publish --title '…' and carry out its plan."). On GitHub that is the
`github_publish` tool: the gateway commits, pushes and opens the draft pull
request as its own GitHub identity after your turn ends, and the result
arrives in the session without a new turn. On another forge the intent has a
`forge` and no `tool`: commit first, then hand the plan to that forge's
skill. If the skill says the gateway keeps logins out of your commands, you
cannot push there: the person pushes from the card. Never force-push.

## Catching up

`wtc catch-up` merges the destination branch into the child's worktree. It
never rebases and never pushes. A conflicted merge is aborted and reported:
merge by hand, resolve, commit. Rebasing a published branch makes it
unpublishable.

Never run `git push` yourself, and never `git rebase` or `git commit --amend`
on commits that were published.
