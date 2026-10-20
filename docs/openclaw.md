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
collection card's ↑ starts with the gateway's own `sessions.github.publish`,
as the chat's Publish PR button does for a session's single checkout. A
conversation per repository, for swarming, is an explicit choice:
`wtc add-repo --separate`.

The branch prefix is fixed by OpenClaw. The name must be lowercase letters,
digits and hyphens, at most 64 characters.

## What the agent runs

| To | The agent runs | And then |
|---|---|---|
| add a repository | `wtc add-repo <repo> --worktree <name>` | calls `sessions_spawn` with each intent's `args` |
| see the collection | `wtc status [<name>]` | reads it: folder, repositories, pull requests, services |
| check a publication | `wtc publish --title "…"` in `<folder>/<repo>` | tells the person it is ready: they push it with ↑ in the card. Only when they ask the agent to publish does the owner call `github_publish` |
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

- a card above the message box, where OpenClaw shows a task's progress. It
  is `wtc status`'s tables: every fact has its column, always in the same
  place, and `·` where there is none. **Repo Branch ± ↑ ↓ PR C M R** is a row
  per repository: files changed, commits to push, commits behind the base,
  the pull request, its CI, whether it merges and its review. **Service Repo
  State URL** is a row per service, then a column per way in (an OpenClaw
  portal or a tunnel provider) and a **+** that opens a new one. Collapsed, the
  card shows the same facts summed, in the same order: **± ↑ ↓ PR svc**.

  What can be done about a fact is done on it, so actions have their place
  too: the ↑ count pushes, opening a draft pull request when there is none,
  the ↓ count catches
  up, a ✗ under **M** asks the conversation to resolve the conflict, and a
  service's ▶, ■ or ↻ starts, stops or restarts it. The same marks in the
  header of the services act on all of them, and collapsed, ↑, ↓ and the
  services' mark act on the whole collection.

  An open tunnel shows the first part of its address, links to it and closes
  with ✕; a closed one is a ▶ to open it again. **+** offers, in order, the
  gateway's own OpenClaw portal, a public Cloudflare or ngrok link for when a
  portal does not work, a dev tunnel for the person's account, and Tailscale
  when the plugin's `teamTailnet` setting says the gateway's tailnet is the
  team's rather than a person's; a Tailscale tunnel is never public, a public
  link is Cloudflare's or ngrok's. Without portal ingress on the gateway
  (`gateway.portals.ingress`, or Tailscale run by the gateway), a portal
  answers at the gateway's own address, so it is offered last, marked "needs
  ingress", and an open one is marked ⚠ when the browser cannot reach its
  address. A provider whose client is not on the gateway's PATH says "install
  first", and the conversation is asked to install it. A service's own address
  is on the gateway, often inside a container, so it shows as its port only. A
  tunnel started as a service of its own is marked, with **Consolidate** asking
  the conversation to open it again with `wtc svc tunnel`. The card appears
  only on sessions that have a collection. The plugin provides it as a
  composer that wraps the built-in one, selected when the plugin loads;
  **Customize UI → Built-in** switches back.

  Pushing is the person's, never the agent's. The gateway publishes the
  repository's worktree with the person's own GitHub connection
  (**Settings → Profile → GitHub connections → For me**), which only the
  gateway holds, and only once they confirm: the first click on ↑ asks the
  gateway, which then waits, and ↑ becomes **confirm ↑** naming the account.
  The gateway's shared GitHub account, often the `gh` login its agents use
  too, is never used for it; without a personal connection ↑ says where to
  connect one. A Bitbucket repository is pushed the same way by the
  [openclaw-bitbucket](https://github.com/lcorneliussen/openclaw-bitbucket)
  plugin (0.3.0 or later), with the person's own Bitbucket login: it pushes
  what is committed, so uncommitted files stay behind, and opens a draft pull
  request into the repository's base. Its `execLogin: false` keeps the login
  out of the agents' commands, so that this is the only way to push. Without
  the plugin, ↑ shows what is waiting without a button; the card never asks
  the conversation to push. **Resolve** asks it to merge and commit, not to
  push.
- a **Collection** panel beside the conversation (⤢ on the card) with the
  collection's name, branch and folder, the same repository and service
  tables with the same actions, wtc status's pull request table (**PR Repo
  State C M R Title**), and **Add repository**.

Narrow, as beside a conversation, in a narrow window or on a phone (under
520 pixels), every row wraps into two lines: first what it is (the repository
and its branch, the service and its repository), then its facts, still in
their columns under the header, which wraps the same way.

Both render what `wtc status <name> --json` prints and refresh every 30
seconds while they are open. `node --test openclaw-plugin/test/*.test.mjs`
renders them against a minimal DOM.

## Install

On the gateway's host, as the user the gateway runs as. For a gateway built
from an image by CI/CD, see [Gateway image and CI/CD](#gateway-image-and-cicd)
instead. An agent asked to set wtc up starts with `wtc docs setup.md`.

1. **`wtc`** on the `PATH` of the gateway and of the agents' shells: a
   [release](https://github.com/lcorneliussen/wtc-cli/releases)'s
   `wtc_<version>_linux_<arch>.tar.gz` unpacked into `/usr/local/bin`, or
   `mise use -g github:lcorneliussen/wtc-cli@<version>`.
2. **Git 2.42 or newer and the GitHub CLI.** OpenClaw 2026.10.2 checks Git
   attributes with `git var GIT_ATTR_GLOBAL` before it publishes; its image
   ships Git 2.39 and no `gh`, and publication fails there with "workspace
   attributes could not be verified".
3. **The plugin**, which brings the collection card, the Collection panel and
   the agent's `wtc` skill:

   ```sh
   wtc openclaw install
   ```

   It is embedded in `wtc`, so it is always the plugin of that version. It
   turns on custom Control UI plugins
   (`gateway.controlUi.experimental.customPlugins`) and runs
   `openclaw plugins install … --force`, then `openclaw plugins reload wtc`
   so that a running gateway uses it without a restart. For a gateway on
   another host, `wtc openclaw install --dir ./wtc-plugin` writes the plugin
   for `openclaw plugins install ./wtc-plugin --force` there.
4. **Bitbucket**, if repositories live there:

   ```sh
   openclaw plugins install git:github.com/lcorneliussen/openclaw-bitbucket@v0.3.0 --accept-capabilities
   openclaw config set plugins.entries.bitbucket.config.execLogin false
   ```

   `execLogin false` keeps people's Bitbucket logins out of the agents'
   commands, so that pushing goes only through the card.
5. **Each person** connects their own accounts, which the gateway pushes
   with: GitHub under **Settings → Profile → GitHub connections → For me**,
   Bitbucket on `/plugins/bitbucket/connect` opened in its own tab. The card
   says so the first time it needs one.
6. **Repositories** the gateway clones itself: **Add repository** in the
   Collection panel, or the gateway's own project settings.
7. **Each agent's `AGENTS.md`** says that the gateway works in collections.
   The plugin's `wtc` skill only comes up when an agent already thinks of
   wtc; `AGENTS.md` is what it reads in every conversation. Append the
   section this `wtc` ships, to the `AGENTS.md` in each agent's workspace
   (`openclaw agents list` shows where):

   ```sh
   wtc docs openclaw-AGENTS.md >> <workspace>/AGENTS.md
   ```

To check: `wtc doctor --openclaw` on the gateway host checks Git, `gh`, the
plugin and its version, the Control UI setting and each agent's `AGENTS.md`.
From an agent's command, plain `wtc doctor` does the same. A
conversation asked to "start the collection 1-try-wtc with <owner/repo>" then
gets a card above its message box.

## Gateway image and CI/CD

A gateway that a pipeline builds keeps wtc in its image and its setup in
configuration, so that nothing is installed by hand and an upgrade is a
version bump.

In the image, with the versions pinned as build arguments:

```dockerfile
ARG WTC_VERSION=<version>
ARG OPENCLAW_BITBUCKET_TAG=<tag>
RUN curl -fsSLO "https://github.com/lcorneliussen/wtc-cli/releases/download/v${WTC_VERSION}/wtc_${WTC_VERSION}_linux_$(dpkg --print-architecture).tar.gz" \
 && curl -fsSL "https://github.com/lcorneliussen/wtc-cli/releases/download/v${WTC_VERSION}/checksums.txt" | grep "linux_$(dpkg --print-architecture)" | sha256sum -c - \
 && tar -xzf wtc_*.tar.gz -C /usr/local/bin wtc && rm wtc_*.tar.gz \
 && wtc openclaw install --dir /opt/wtc-plugin
# Only with Bitbucket repositories:
RUN git clone --depth 1 --branch "${OPENCLAW_BITBUCKET_TAG}" https://github.com/lcorneliussen/openclaw-bitbucket.git /opt/bitbucket-plugin
```

The image also needs Git 2.42 or newer and `gh` (step 2 above). `wtc
openclaw install --dir` writes the plugin embedded in that `wtc`, so plugin
and binary cannot drift apart.

In the gateway's configuration, applied by the pipeline (or by
`openclaw config set` in a configure step):

```json
{
  "gateway": { "controlUi": { "experimental": { "customPlugins": true } } },
  "plugins": {
    "load": { "paths": ["/opt/wtc-plugin", "/opt/bitbucket-plugin"] },
    "entries": { "bitbucket": { "config": { "execLogin": false } } }
  }
}
```

Plugins on a load path are loaded from the image as they are, so there is no
install step and nothing stale survives a new image. Keep the agents'
`AGENTS.md` in the same repository as the rest of the gateway's setup, with
the section from `wtc docs openclaw-AGENTS.md`.

To upgrade, bump `WTC_VERSION` (and the Bitbucket tag when its release notes
call for it) and let the pipeline build and roll out the image. People's
connected accounts live in the gateway's state, not in the image, and stay.
As a smoke test after the rollout, run `wtc doctor --openclaw --json` in the
gateway's container: it exits non-zero when a required check fails.

## Upgrade

In place, on the gateway's host: no new gateway, no restart, and the
configuration and people's connected accounts stay as they are. A gateway
built by CI/CD upgrades with a new image instead (above).

1. **`wtc`**: replace the binary the way it was installed, with the new
   release.
2. **Its plugin**: `wtc openclaw install` again. It replaces the plugin with
   the one embedded in the new `wtc` and reloads it in the running gateway.
   Reload open Control UI tabs to get the new card. Until this step, the
   Collection panel says that the plugin and the gateway's `wtc` differ.
3. **openclaw-bitbucket**, when its release notes call for it: install the new
   tag over the old one and reload it.

   ```sh
   openclaw plugins install git:github.com/lcorneliussen/openclaw-bitbucket@<tag> --force --accept-capabilities
   openclaw plugins reload bitbucket
   ```

`openclaw plugins update` does not upgrade the wtc plugin: it skips plugins
installed from a path, which is how `wtc openclaw install` installs.
