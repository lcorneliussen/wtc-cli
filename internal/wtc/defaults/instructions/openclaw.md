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
  no fetch: the worktrees share the gateway's clone. It never rebases; for a
  conflict, follow the wtc-catch-up skill.
- A worker is merge-only. Its work leaves through your branch: the person
  pushes that with ↑, with the workers' work merged in. `wtc publish` refuses
  on a worker's branch, and `wtc svc` in a worker's collection: services run
  in yours.
- `wtc catch-up` in a worker's checkout merges your branch, its parent, into
  it, not origin's.
- Readers (review, research, a test run) need no worker: they can read your
  checkouts.
- A worker is only for a part that really runs beside yours. Work you would
  do next anyway goes in your own checkout: a worker you then wait for is a
  second copy of the repository for nothing.
- A part that needs its own pull request, its own services or the person's
  attention is no worker: it is a collection of its own, with its own
  conversation. Suggest that the person starts one (ad hoc, or in this wtc
  project).
- When a worker ends, merge it in, or tell the person what is left. The card
  shows each worker as a sub-row under its repository: ± its uncommitted
  files (against yours), **↑** merges its commits into your checkout, **↓**
  catches it up with yours, and **✕** discards it. None should be left
  behind.

## wtc projects

A **wtc project** is a named set of repositories that belong together: a
harness the gateway is configured with, whose `.harness-repos.yml` lists the
repositories, their default branches, and whose `AGENTS.md` holds the
instructions for working in them. A **collection** is one task's worktrees
of the repositories it needs, in one wtc project, or in an ad-hoc one, which
takes any repository. `wtc projects` lists the gateway's projects and their
repositories.

Work on a repository happens in a collection, also when it is only reading
or analyzing. When the person names something to work on ("the site", "the
CLI") and the conversation has no collection, match it with `wtc projects`
and start one with `wtc add-repo`. Never work in another conversation's
collection folder or a clone of your own.

A collection's first `wtc add-repo` adds a worktree of the project's harness
too, and a name like `api` resolves through its `.harness-repos.yml`: that
entry's remote, based on its `default_ref` unless `--base` says otherwise.
Read the harness's `AGENTS.md` right away: it says which other repositories
a task needs.

`wtc add-repo` takes the collection's wtc project when it has one. Otherwise
the registries decide: the project that lists the repositories asked for, by
name, URL or `owner/name`; between several that list some, the ones that list
all; between several still, the gateway's default. When none lists them, the
default or the only project configured. When that does not settle it, it says
so: ask the person which wtc project the task belongs to, then pass
`--project <name>`, or `--project none` for an ad-hoc collection (`--harness`
is the same flag under its earlier name). One collection never mixes
projects; work in another one is another collection.

A repository the project does not list stops `wtc add-repo` with three
choices. Put them to the person and run the one they pick: ad hoc beside the
project's repositories (`--ad-hoc`), registered in the project first (an
entry in `.harness-repos.yml` in the collection's worktree of the harness,
committed there), or a collection of its own in an ad-hoc project
(`--project none` and another name). When another wtc project lists it, a
fourth choice says so: work on that project belongs in a new conversation
for it, which the person starts.

A conversation works in one wtc project, and projects stay apart: never read
or change another project's repositories from this collection, unless the
person added them here ad hoc. All of the gateway's projects can be listed;
that is for choosing one, not for borrowing from it.

The collection's folder says which wtc project it works in:
`$OPENCLAW_STATE_DIR/wtc/<project>/<name>`, or `wtc/ad-hoc/<name>`.

`wtc status <name>` shows the project and the worktree of its harness. Its
`AGENTS.md` and the instructions it points to are the environment's rules,
and they win over general habits. Changes to the harness itself are work like
any other: commit in its worktree, and the person publishes them.

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
| check before publishing | `wtc publish --title "<what changed>" --body-file <file>` in `<folder>/<repo>` | — |
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

**Describe the pull request.** `--title` (one line, at most 256 characters)
and `--body` or `--body-file` (Markdown, at most 8192) are recorded for the
branch in the worktree's git directory, never committed. The card's ↑ opens
the pull request with them, so write the description a reviewer needs: what
changed and why, how it was checked, what is left. Follow the repository's
pull request template when it has one. The gateway takes the text only when
it opens the pull request; later pushes keep it. One you leave out keeps
what was recorded before. Bitbucket's publish takes the title only.

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

## Pull request news

The wtc plugin tells the collection's conversation when one of its pull
requests changes: opened, ready for review, merged or closed, checks failed
or passed, approved, changes requested, conflicting with its base. It comes
as a system event starting with `wtc: pull request news`, usually with a
turn of its own. Update the task's progress with it. For a failure, a review
or a conflict, read it (`gh pr checks <n>`, `gh pr view <n> --comments`) and
tell the person what you will do. Fix in `<folder>/<repo>` and commit, and
they push again. After a merge, run `wtc catch-up` in `<folder>/<repo>` so
the checkout continues from the merge. Don't answer review comments on the
forge yourself.

## Skills

The gateway's agents have the wtc skills for this host, installed with the
plugin by `wtc openclaw install`: **wtc-catch-up** (catching up, merging a
worker, resolving the conflicts), **wtc-pr** (getting a pull request ready
for the person's push) and **wtc-follow** (acting on pull request news). The
card asks for them by name (`/skill wtc-catch-up …`), so a click there and a
request in the chat do the same.

## Catching up

`wtc catch-up` merges the destination branch into the child's worktree. It
never rebases and never pushes. A worktree resting detached at its base, with
no work yet, just moves to the base's new tip; one with commits on no branch
is refused until they are on a branch. A conflict is the agent's to resolve
in its own checkout: the wtc-catch-up skill. Rebasing a published branch
makes it unpublishable.

Never run `git push` yourself, and never `git rebase` or `git commit --amend`
on commits that were published.
