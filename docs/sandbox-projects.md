# Project discovery in a sandbox

A sandbox can use `wtc projects --json` and resolve repository names for
`wtc add-repo` through the gateway's sandbox Git service. Set `WTC_GATEWAY_GIT`
as described in [OpenClaw sandbox setup](openclaw.md#in-a-sandbox).
The gateway and sandbox must both have a version of wtc that supports project
discovery.

Discovery is **denied by default**. On the gateway, the operator can configure
`plugins.entries.wtc.config.sandboxGit.projectsByAgent`:

```json
{
  "port": 18790,
  "projectsByAgent": {
    "developer": ["platform"],
    "operations": ["operations"]
  }
}
```

These are exact agent IDs and configured wtc project names. There is no wildcard
or implicit default. An absent, empty or invalid entry denies discovery for that
agent. Choose this policy deliberately: allowing a project exposes its name,
harness remote and ref, default status, and its entire repository registry
(names, remotes, roles and default branches) to every sandbox session of that
agent. Use separate agents when their project metadata visibility must differ.

The service identifies the agent from the gateway's running Docker container
and uniquely owned managed worktree, rechecking the container for every discovery
request. Request parameters and headers cannot select another identity or
project. Unknown or ambiguous ownership is refused. Only allowlisted registries
are read. Remotes are returned as credential-free GitHub or Bitbucket HTTPS
URLs; unsupported remotes or unreadable registries produce a generic project
error. Raw configuration, command errors and gateway filesystem paths are not
returned.

Inside the sandbox:

```sh
wtc projects --json
wtc add-repo api --worktree 42-fix --project platform --separate
wtc doctor
```

Repository names and default branches resolve from one permitted snapshot per
plan. If the collection has a worktree of its harness, that worktree's registry
(including unpublished edits) is used and the harness is not added again. A registered repository uses its gateway project ID. `add-repo` prints
session-spawn intents; it performs no gateway writes and does not record project
selection in the sandbox filesystem. An existing collection's project is kept
when planning for that collection. If no project record exists yet, the
collection's existing harness worktree establishes its project; ambiguous
metadata is refused. `doctor` reports the gateway discovery source
or that discovery is unavailable. A denied, malformed or unreachable discovery
response fails `projects` and `add-repo`; it does not silently fall back to an
empty local configuration. An empty permitted project list is distinct from a
discovery failure.

**Listing grants no clone, worktree, publication or Git read permission.**
The gateway still authorizes session-spawn intents. The Git service continues
to serve only repositories already in the session's collection; knowing another
repository's metadata does not make it fetchable. Discovery supplies no forge
credential. Local gateway and terminal behavior is unchanged when
`WTC_GATEWAY_GIT` is unset.
