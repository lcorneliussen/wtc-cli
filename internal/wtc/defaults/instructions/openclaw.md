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
separate conversation per repository is for swarming, and only when the person
asks for one (`--separate`, or a spawn of your own with `visible: true`).

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
| publish | the person presses **Publish** for the repository in the Collection panel; or `sessions_send` the owner: "Run wtc publish --title '…' and carry out its plan." | — |
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

On GitHub the gateway then commits what is pending, pushes the branch and
opens or updates a **draft** pull request after your turn ends. The result
arrives in the session without starting a new turn. People merge.

On another forge the plan's intent has a `forge` and no `tool`: the gateway
brokers nothing there. Commit first, then hand the plan to that forge's
skill, which pushes from the worktree. Never force-push.

## Catching up

`wtc catch-up` merges the destination branch into the child's worktree. It
never rebases and never pushes. A conflicted merge is aborted and reported:
merge by hand, resolve, commit. Rebasing a published branch makes it
unpublishable.

Never run `git push` yourself in a GitHub worktree, and never `git rebase` or
`git commit --amend` on commits that were published.
