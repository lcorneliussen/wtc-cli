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
| the collection folder | `$OPENCLAW_STATE_DIR/wtc/<harness>/<name>`, or `wtc/ad-hoc/<name>` without a harness (`wtc-collections/<name>` for one made before): a link per repository to its worktree, kept by `wtc status`; services run here |
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
| check a publication | `wtc publish --title "…" --body-file <file>` in `<folder>/<repo>` | tells the person it is ready: they push it with ↑ in the card, which opens the pull request with that title and description. Only when they ask the agent to publish does the owner call `github_publish` |
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

## The pull request's title and description

`wtc publish` records `--title` and `--body` (or `--body-file`, `-` for
stdin) for the branch, in the worktree's git directory, so nothing is
committed. `wtc status --json` reports them per repository as `proposal`, and
the card's ↑ hands them to `sessions.github.publish` and shows the title
before the push. The gateway's limits apply: one line of at most 256
characters, a description of at most 8192. The gateway uses them only when it
opens the pull request. Without a proposal, it opens as "Publish
openclaw/<name>". Bitbucket's publish takes the title only.

## Pull request news in the conversation

The card's ↑ runs outside the agent's turn, and a merge or a review happens
on the forge. The plugin compares each look at a collection's pull requests
with the one before and tells the collection's conversation what changed. It
queues one system event (`wtc: pull request news for this collection`, a line
per change) and asks for a turn, so the agent updates its progress without
waiting for the person. It reports a pull request that was opened, became
ready or went back to draft, was merged, closed or reopened, failed or passed
its checks, was approved, got a request for changes, or conflicts with its
base.

The event also says what to do with the news: bring the task's progress up
to date, catch up after a merge, and tell the person only what they need to
act on. On the card, a merged pull request whose merge the checkout doesn't
have yet offers ↓: `wtc status` reports `mergedIn` for it, since a base that
was never fetched doesn't count as behind.

It looks whenever a card loads, and every `pullRequestPollMinutes` (default 5)
at collections with an open pull request. What it saw is kept in
`$OPENCLAW_STATE_DIR/plugins/wtc/pull-requests.json`, so a restart doesn't
repeat news. `pullRequestEvents` is `wake` (the default), `queue` (told on
the conversation's next turn) or `off`. Pull requests come from `gh`, so this
covers GitHub.

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
- Without the gateway's git service (below), `wtc catch-up` cannot fetch.
  Catch up from the repository's session outside the sandbox.
- A repository on another forge cannot be published from a sandbox.

### The gateway's git service

The private copy is one commit deep, and the commits made in it stay there:
only its files reach the managed worktree. The plugin can serve the gateway's
clones to the sandboxes, read-only, so that an agent there reads history and
catches up with Git's own merge state, and its commits reach the gateway:

```sh
openclaw config set plugins.entries.wtc.config.sandboxGit '{"port":18790}' --strict-json
openclaw config set agents.defaults.sandbox.docker.network wtc-sandboxes
openclaw config set agents.defaults.sandbox.docker.env \
  '{"WTC_GATEWAY_GIT":"http://<gateway>:18790"}' --strict-json
```

The gateway and its sandboxes share the Docker network (`wtc-sandboxes`
here; the gateway's container joins it too), and `<gateway>` is the gateway's
address or container name on it. A bridge network also routes out to the
internet: restricting that is the network's job, not wtc's. Sandboxes
already running keep their settings until `openclaw sandbox recreate`.

The service answers only sandbox containers of this gateway. It takes a
connection's address, finds the sandbox container that has it, the managed
worktree that container works on and the session that owns it, and serves
that session's collection: each repository's collection branches and what
origin has on the gateway. Other collections' branches stay hidden and
cannot be fetched, a push is refused, and no forge credential enters the
sandbox. Then, in the sandbox:

- The checkout gets a `gateway` remote: the collection's branches as
  `gateway/openclaw/…`, origin's as `origin/…`, with all their history.
  The first fetch deepens the copy. Other repositories of the collection
  clone from `$WTC_GATEWAY_GIT/<repo>`.
- `wtc catch-up` merges `origin/<base>` there, as the gateway last fetched it
  (at most five minutes old). A conflict stops the merge for the agent to
  resolve as in any checkout, with `git log --merge` and the stages; then
  `wtc catch-up --continue`. Uncommitted edits are set aside around the merge
  and put back after.
- Then the gateway takes the session's branch, the sandbox's commits and the
  merge included, if it extends the gateway's branch, merges only origin's
  commits and leaves no conflict markers. The commits keep the sandbox's
  author; one that carries a `Co-authored-by` trailer or the publication
  marker is refused, since [credit](#credit-for-the-people-who-asked) is the
  gateway's to add. Its branch and index move to it;
  the files arrive from the sandbox, as every edit does. Publishing stays the
  person's: ↑ in the card.
- If the gateway committed since (a publication), the sandbox's history
  starts over from the gateway's branch, with the files unchanged.
- `wtc doctor` reports `fetch: read-only from the gateway` and fails when a
  credential that could push is in the sandbox.

For a gateway that people with different rights share, sandbox the agents
that do development work. Each repository then gets its own conversation
(`wtc add-repo --separate`), working in its sandbox's private copy, which the
gateway reconciles into the managed worktree and publishes. The collection
folder, one conversation driving several repositories, needs the agent's
commands on the gateway itself, with the trust that implies. Do not bind the
managed worktrees or the gateway's clones into a sandbox to get it: that goes
around the private copy and its reconciliation
([#105](https://github.com/lcorneliussen/wtc-cli/issues/105)).

## Git the gateway runs

The card's status, catch-up and publish make the gateway run `wtc` and git in
agents' worktrees, as the gateway's user. A repository's own config can make
git run programs, so wtc does not let it:

- every git command runs with fsmonitor, hooks, signature checking and
  signing turned off;
- a checkout whose repository or worktree config sets anything else that can
  run a program (a filter or merge driver, `core.sshCommand`, an
  upload-pack, an include) is refused, with the key and how to remove it;
- the gateway's global and system config, the admin's, stay in effect, so a
  Git LFS filter configured there still works;
- the branch list for **Add repository** reads only a project's origin URL
  and runs `git ls-remote` outside the clone.

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

## Workers

A task whose parts can go on in parallel gets **workers**: one writer per
checkout, exchanging work through git (#106).

- `wtc add-repo <repo> ... --worker <w> --work "<part>"` spawns hidden
  sessions with managed worktrees of collection `<task>--<w>`, on branch
  `openclaw/<task>--<w>`. Each worktree starts from the task's branch in
  that repository when there is one, otherwise from the repository's base;
  `--base` overrides. The worker is told to commit only in its own
  worktrees and never to publish, and reports to the task's session when it
  ends.
- `wtc catch-up --from <w>`, in the task's worktree of a repository, merges
  the worker's branch: the worktrees are in the same clone, so nothing is
  fetched. Like catch-up, it never rebases and aborts a conflicted merge.
- `wtc status <task>` lists the workers and, per repository, how many of
  each worker's commits the task's branch has not merged yet. `--json` has
  them under `workers`.
- A worker uses the task's harness for names and default branches, but gets
  no worktree of the harness unless its part is the harness.

The Collection panel does not group workers under their task yet; it shows
each worker's collection as its own.

## wtc projects

A **wtc project** is a named set of repositories that belong together, with
the instructions for working in them. On a gateway each one is a configured
harness: a repository whose `.harness-repos.yml` lists the project's
repositories and whose `AGENTS.md` holds its instructions. A collection is
one task's worktrees of a project's repositories. An **ad-hoc** collection
works without a project and takes any repository. The projects are
configuration, set by whoever maintains the gateway (the keys keep the
harness name):

```json
"plugins": { "entries": { "wtc": { "config": {
  "harnesses": [
    { "name": "platform", "remote": "https://github.com/example-org/platform-harness.git", "ref": "main" }
  ],
  "defaultHarness": "platform"
} } } }
```

`wtc projects` lists them with their repositories and roles, so that the
agent can match what the person asks to work on before `wtc add-repo` starts
the collection.

A conversation starts without a collection, wherever OpenClaw's New session
put it, and often only its first messages say where the work is. So the card
of a conversation without a collection suggests one: "Work in wtc project
**wtc** with wtc-site?", from the person's latest message. **Start** asks
the conversation to run `wtc add-repo` for it. **Change…** opens the panel,
and ✕ dismisses it. The suggestion comes from the gateway's decision model
when one is selected (`agents.defaults.decisionModel`), as one choice among
the projects' repositories by name and role. Without one, it comes from
matching the message's words against those names and roles.

OpenAI's decision model (`openai/gpt-6-luna`, which needs an OpenAI Platform
API key; a ChatGPT or Codex subscription doesn't authorize it) suits this
best. In a small comparison
([#155](https://github.com/lcorneliussen/wtc-cli/pull/155)), on twelve test
messages over two projects, it chose right eleven times,
in about 200 ms and a few hundred tokens each. The local ONNX classifiers
chose right at most six times, mostly the same repository whatever was
asked, and the word match nine times.

Each harness must also be cloned on the gateway (an OpenClaw project), like
the repositories its registry names: **Add repository** in the Collection
panel, or the gateway's own project settings. `ref` defaults to `main`.

A collection works in at most one wtc project:

- **Which one.** The collection keeps the project it started with. A new
  collection takes `--project <name>` if given (`--harness` is the same
  flag under its earlier name); otherwise the registries decide: the
  project that lists the repositories asked for, by name, URL or
  `owner/name` (of several that list some, the ones that list all; of
  several still, `defaultHarness`). When none lists them, `defaultHarness`
  or the only configured project. Otherwise `wtc add-repo` asks the agent
  to ask the person; `--project none` starts an ad-hoc collection. Without
  projects configured, every collection is ad hoc, as before.
- **A repository the project does not list** is the person's choice, even
  with a default project. `wtc add-repo` stops and the agent asks: ad hoc
  beside the project's repositories in this collection (`--ad-hoc`),
  registered in the project first (an entry in `.harness-repos.yml` in the
  collection's worktree of the harness, published like any change), or a
  collection of its own in an ad-hoc project (`--project none` and another
  name). When another configured project lists it, a fourth choice suggests
  a new conversation for that project instead.
- **One project per conversation.** Every configured project can be listed,
  so that the right one is picked, but a collection never reads from or
  writes to another project's repositories unless the person added them ad
  hoc.
- **A worktree like the others.** The first `wtc add-repo` of a collection
  adds a managed worktree of the harness, on the collection's branch, so a
  task can change the harness and publish that like any repository.
- **Names resolve through it.** `wtc add-repo api` takes `api`'s remote and
  `default_ref` from the harness's `.harness-repos.yml`: from the
  collection's worktree of the harness once there is one, before that from
  the gateway's clone at `ref`. `--base` still wins. A URL or `owner/name`
  that is in the registry resolves to its entry; one that is not is the
  person's choice (above).
- **Instructions.** `wtc status` shows the project and the worktree of its
  harness; the agent reads its `AGENTS.md` from there, and it says which
  other repositories a task needs.
- **Every task starts one.** The skill has the agent start a collection
  whenever the person wants to work on a repository, also only to read or
  analyze it, rather than work from memory or another collection.

Not yet: hooks, secret links and mise pins from a harness do not run on a
gateway, and herdr layouts do not apply. Several projects in one collection,
stacked rather than merged, are tracked in
[#100](https://github.com/lcorneliussen/wtc-cli/issues/100).

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
   `openclaw plugins install … --force --accept-capabilities`, then
   `openclaw plugins reload wtc` so that a running gateway uses it without a
   restart. Running it consents to the capabilities the plugin declares: the
   Collection panel and its session actions. For a gateway on another host,
   `wtc openclaw install --dir ./wtc-plugin` writes the plugin for
   `openclaw plugins install ./wtc-plugin --force --accept-capabilities` there.
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
   Collection panel, or the gateway's own project settings. With a harness,
   add it the same way and configure it (see [wtc projects](#wtc-projects)).
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
    "entries": {
      "bitbucket": { "config": { "execLogin": false } },
      "wtc": { "config": { "harnesses": [{ "name": "platform", "remote": "https://github.com/example-org/platform-harness.git" }] } }
    }
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
