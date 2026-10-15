# wtc under OpenClaw

An [OpenClaw](https://docs.openclaw.ai) gateway runs agents for a team. `wtc`
works in an agent's shell there, with one difference from a terminal or herdr:
it cannot create a worktree the gateway will publish, and it cannot push. The
gateway owns both. So under OpenClaw `wtc` checks and plans, and the agent
carries the plan out with the gateway's tools.

`wtc host` prints `openclaw` when `OPENCLAW_SHELL=exec` is set, which the
gateway's exec tool does, or when `OPENCLAW_STATE_DIR` is: agents on another
runtime (Codex's bash tool) and the Control UI's terminal only inherit the
gateway's environment.

## One conversation, many repositories

One OpenClaw session owns at most one managed worktree, and the gateway
publishes a worktree only for its owner. wtc keeps that, but a task is still
driven from one conversation:

| wtc | OpenClaw |
|---|---|
| the collection (one task) | the conversation the task started in |
| a repository in it | a gateway-managed worktree, held by a **hidden owner** (a subagent session) |
| the collection folder | `$OPENCLAW_STATE_DIR/wtc-collections/<name>`: a link per repository to its worktree, kept by `wtc status`; services run here |
| the branch | `openclaw/<name>`; `<name>` is the collection's `<issue-id>-<slug>` |

The conversation edits, builds and commits in the worktrees through the
folder. The owners only hold the worktrees for publication, which the
Collection panel's **Push + PR** button starts with the gateway's own
`sessions.github.publish`, as the chat's Publish PR button does for a
session's single checkout. A conversation per repository, for swarming, is
an explicit choice: `wtc add-repo --separate`.

The branch prefix is fixed by OpenClaw. The name must be lowercase letters,
digits and hyphens, at most 64 characters.

## What the agent runs

| To | The agent runs | And then |
|---|---|---|
| add a repository | `wtc add-repo <repo> --worktree <name>` | calls `sessions_spawn` with each intent's `args` |
| see the collection | `wtc status [<name>]` | reads it: folder, repositories, pull requests, services |
| check a publication | `wtc publish --title "…"` in `<folder>/<repo>` | the person publishes from the panel, or the owner calls `github_publish` |
| catch up | `wtc catch-up` in `<folder>/<repo>` | publishes again to send the merge |

`<repo>` names a project on the gateway, or a GitHub repository as
`owner/name` or URL. A GitHub repository the gateway has already cloned gets a
hidden owner by its project name. One it has not is added with **Add
repository** in the Collection panel (the gateway's `projects.add`, with the
person's authority); `--separate` instead has the gateway clone it for a
conversation of its own (`projectGitUrl`, which OpenClaw allows only for
visible sessions). Other forges: a gateway admin registers a checkout.

Each of the first two prints a plan and does nothing else:

```json
{
  "host": "openclaw",
  "version": 1,
  "intents": [
    {
      "host": "openclaw", "version": 1,
      "action": "publish", "actor": "repo-session",
      "repo": "api", "forge": "github",
      "tool": "github_publish",
      "args": { "title": "Add the thing" }
    }
  ],
  "pending": ["src/thing.go"],
  "warnings": ["The gateway will commit all 1 pending paths. …"]
}
```

`actor` says which session carries the intent out: `parent`, `repo-session`
or `current`. An intent with a `tool` is a call of that tool with `args`,
unchanged. The repository must be a project on the gateway under the name
given to `wtc add-repo`.

After `github_publish` the gateway commits what is pending as its GitHub
identity, pushes the session's branch and opens or updates a **draft** pull
request, once the turn that asked has ended. People merge.

## What `wtc publish` refuses

The gateway commits every pending file in the worktree and never rewrites a
published branch. `wtc publish` reads the checkout and refuses before the
agent asks for something that would fail or publish the wrong thing:

- outside a gateway-managed worktree (the parent, or a checkout made by hand);
- when history that was published has been rewritten by a rebase, an amend or
  a reset. It knows from the marker the gateway leaves on the commits it
  publishes;
- during an unfinished merge or rebase, or with unresolved conflicts;
- when uncommitted files look like credentials (`.env`, `*.pem`, `id_rsa`, …).
  `--allow-sensitive` is for the file that is meant to be committed;
- when there is nothing to publish.

`wtc catch-up` merges and never rebases, for the same reason, and aborts a
conflicted merge so the worktree is as before.

## Other forges

OpenClaw publishes to GitHub. For a repository whose `origin` is elsewhere the
gateway still creates and owns the worktree, but brokers no publication. The
publish intent then carries a `forge` and no `tool`, and its `args` describe
the push and the pull request:

```json
{ "action": "publish", "actor": "repo-session", "repo": "docs", "forge": "bitbucket",
  "args": { "owner": "example-ws", "repo": "docs", "source": "openclaw/72-add-thing",
            "dest": "main", "title": "Add the thing", "draft": true } }
```

A skill for that forge carries it out, pushing from the session's worktree
with a login the gateway gives the session. Nothing commits for the agent
there: `wtc publish` refuses a worktree with uncommitted work.

## In a sandbox

When an agent's commands run in an OpenClaw sandbox, the shell sees a private
copy of the worktree: the session's branch, no `origin`, no network and no
GitHub token. There:

- `wtc publish` still refuses what is not safe, but cannot check the forge,
  the base branch or earlier publications. It plans `github_publish` and
  leaves the repository to the gateway, which takes it from the session. It
  ignores the skills OpenClaw mirrors into the copy (`.openclaw/sandbox-skills/`).
- The gateway offers `github_publish` in a sandbox only when it is allowed
  there: `tools.sandbox.tools.alsoAllow: ["github_publish"]`. The tool runs on
  the gateway, so this gives the sandbox no credential.
  `tools.github.allowInSandbox` is not needed and would put the token inside.
- `wtc catch-up` cannot fetch. Catch up from the repository's session outside
  the sandbox.
- A repository on another forge cannot be published from a sandbox.

## Credit for the people who asked

The gateway commits as its own GitHub identity and adds a `Co-authored-by`
trailer for each person who prompted the session, provided their gateway
profile has a **verified** GitHub account. Only the sign-in can verify one:
Cloudflare Access with GitHub as the identity provider, or a Tailscale Serve
login made with GitHub. A token, a password, any other identity provider, and
a "My GitHub" connection made for publishing do not. A person who signs in
another way gets no credit unless an administrator links their profile to a
GitHub-verified one.

Repository sessions created with `sessions_spawn`, which is what
`wtc add-repo` asks for, take the parent session's contributors with them, so
people need only talk to the collection's session. This comes from OpenClaw's
documentation; no test here signs in with a verified GitHub account yet.

## The collection in the Control UI

OpenClaw shows a session's changes and pull request for its own checkout only,
so a conversation whose repositories are held by owner sessions shows none of
them. The plugin in [`openclaw-plugin/`](../openclaw-plugin) adds the view of
the task as a whole:

- a card above the message box, where OpenClaw shows a task's progress.
  Collapsed, it shows what is not pushed yet, each open pull request
  coloured by its CI, and how many of the collection's services run; expanded,
  each repository on its branch with **Push + PR** (**Push** once it has a pull
  request) and **Catch up**, and each service on one line with its address and
  its tunnels, as `wtc status` shows them. A tunnel started as a service of its
  own is marked, with **Consolidate** asking the conversation to open it again
  with `wtc svc tunnel`. It appears only on sessions that have a collection. The plugin
  provides it as a composer that wraps the built-in one, selected when the
  plugin loads; **Customize UI → Built-in** switches back.
- a **Collection** panel with a row per repository — branch, commits ahead of
  and behind the base, what the next publication would send, the pull request
  with its CI, and **Catch up** and **Push + PR** buttons — the collection's
  pull requests and services, and **Add repository**.

Both render what `wtc status <name> --json` prints and refresh every 30
seconds while they are open. `node --test openclaw-plugin/test/*.test.mjs`
renders them against a minimal DOM.

```sh
openclaw config set gateway.controlUi.experimental.customPlugins true
openclaw plugins install -l /path/to/openclaw-plugin
```

## What the gateway needs

- `wtc` on the `PATH` of the gateway and of the agents' shells.
- Git 2.42 or newer and the GitHub CLI. OpenClaw 2026.10.2 checks Git
  attributes with `git var GIT_ATTR_GLOBAL` before it publishes; its image
  ships Git 2.39 and no `gh`, and publication fails there with "workspace
  attributes could not be verified".
- A GitHub identity for the agent, connected in the gateway's settings.
- Each repository registered as a project.
