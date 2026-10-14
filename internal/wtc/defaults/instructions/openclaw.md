# Under OpenClaw

`wtc host` prints `openclaw` when a command runs under an OpenClaw gateway: in
an agent's shell, whatever tool runs it, or in the Control UI's terminal. There the rules for creating
worktrees and publishing change, because the gateway owns both:

- a command in the agent's shell holds no git credential and cannot reach the
  gateway;
- the gateway only publishes a worktree it created itself, for the session
  that owns it;
- one session owns one repository. A collection is the session the task
  started in (the **parent**) plus one **child session per repository**, each
  with a managed worktree on branch `openclaw/<name>`. `<name>` is the
  collection's `<issue-id>-<slug>`: lowercase letters, digits and hyphens.

`<repo>` is the name of a project registered on the gateway, or a GitHub
repository, as `owner/name` or its URL: the gateway clones that itself, with
its own GitHub login. A repository on another forge has to be registered as a
project by a gateway admin first.

A collection is not a folder here: it is the gateway's managed worktrees of
one name, one per repository. Never `wtc init` or `wtc new` a folder
collection under OpenClaw (wtc refuses): the gateway can neither see nor
publish it. `wtc status` lists the collections with each repository's branch,
pending changes and pull request.

So `wtc` does not act there. It checks, then prints a **plan**: JSON on
stdout with the `intents` for you to carry out, and one line per intent on
stderr saying how. An intent with a `tool` means: call that tool with the
intent's `args`, unchanged, in the session `actor` names. Do not edit the
args, and do not do the work some other way.

| You want to | Run | Then |
|---|---|---|
| add a repository | `wtc add-repo <repo> --worktree <name>` in the parent | call `sessions_spawn` once per intent |
| publish work | `wtc publish --title "<what changed>"` in the repository's child | call `github_publish`; off GitHub, hand the plan to the forge's skill |
| take in upstream changes | `wtc catch-up` in the child | publish again to send the merge |
| see the collection | `wtc status [<name>]`, anywhere | — |

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
