# wtc under OpenClaw

An [OpenClaw](https://docs.openclaw.ai) gateway runs agents for a team. `wtc`
works in an agent's shell there, with one difference from a terminal or herdr:
it cannot create a worktree the gateway will publish, and it cannot push. The
gateway owns both. So under OpenClaw `wtc` checks and plans, and the agent
carries the plan out with the gateway's tools.

`wtc host` prints `openclaw` when `OPENCLAW_SHELL=exec` is set, which the
gateway does for every command an agent runs.

## A collection is a session and its children

One OpenClaw session owns one repository. A collection therefore maps onto:

| wtc | OpenClaw |
|---|---|
| the collection (one task) | the session the task started in — the **parent** |
| a repository in it | a **child session** owning a gateway-managed worktree |
| the branch | `openclaw/<name>`; `<name>` is the collection's `<issue-id>-<slug>` |

The branch prefix is fixed by OpenClaw. The name must be lowercase letters,
digits and hyphens, at most 64 characters.

## What the agent runs

| To | The agent runs | And then |
|---|---|---|
| add a repository | `wtc add-repo <repo> --worktree <name>` in the parent | calls `sessions_spawn` with each intent's `args` |
| publish | `wtc publish --title "…"` in the repository's child | calls `github_publish` with the intent's `args` |
| catch up | `wtc catch-up` in the child | publishes again to send the merge |

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

OpenClaw shows a parent session with its children nested under it, and each
child's changes on its own. It has no view of the task as a whole. The plugin
in [`openclaw-plugin/`](../openclaw-plugin) adds one: a **Collection** panel,
on the parent and on every child, with a row per repository — branch, commits
ahead of and behind the base, what the next publication would send, and the
pull request — and a **Catch up** button that runs `wtc catch-up` in that
child's worktree. It renders what `wtc checkouts` prints.

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
