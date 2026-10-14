# Development and validation

Build with Go 1.27 or newer:

```sh
go build -o wtc ./cmd/wtc
go test ./...
go vet ./...
```

Tagged releases build macOS and Linux binaries through GoReleaser. Generic
instructions and skills live under `internal/wtc/defaults/` and are embedded
in the CLI. Project-owned overrides belong in harness repositories.

[Release notes](release-notes.md) are committed and embedded separately from
generated harness guidance. Before tagging, add the matching numbered note;
the release workflow publishes that exact file as the GitHub release body.

[Public demos](demos/README.md) record actual collection and bootstrap commands
against synthetic local repositories. They need no forge account or agent.

## OpenClaw integration tests

`internal/openclaw` holds what `wtc` does under OpenClaw ([guide](openclaw.md)).
Its integration tests drive a throwaway gateway in Docker and are not part of
`go test ./...`:

```sh
go test -tags openclaw_integration ./internal/openclaw/ -v
```

They build a test image from `internal/openclaw/testdata/gateway.Dockerfile`
(the OpenClaw image named by `WTC_OPENCLAW_IMAGE`, a Git and GitHub CLI recent
enough to publish, and `wtc` from this checkout) and run:

- the intent contract against synthetic repositories, with no account at all;
- whole conversations, in which a scripted model
  (`testdata/stub-model.mjs`) makes real agent turns run `wtc` and carry out
  its plans;
- the collection plugin from `openclaw-plugin/`;
- a conversation whose commands run in OpenClaw's Docker sandbox, from an
  image built out of `testdata/sandbox.Dockerfile`. Its gateway gets the
  Docker socket at `/var/run/docker.sock` and keeps its state on the Docker
  host under `/var/lib/wtc-openclaw-e2e/`, because the daemon resolves the
  sandbox's bind mounts there.

Tests that publish need a throwaway repository and a login, and skip without:

| Variables | Test |
|---|---|
| `WTC_OPENCLAW_GITHUB_REPO=<owner>/<repo>`, `GH_TOKEN` | `GitHub`: the gateway commits, pushes and opens a draft pull request |
| `WTC_OPENCLAW_BITBUCKET_REPO=<workspace>/<repo>`, `TWG_BBC_USER`, `TWG_BBC_TOKEN`, `WTC_OPENCLAW_BITBUCKET_SKILL=<checkout of the Bitbucket skill>` | `Bitbucket`: the skill pushes and opens the pull request |
| the GitHub variables | `SandboxedConversation`: a sandboxed agent turn runs `wtc publish` and `github_publish` opens the draft pull request |
| `OPENAI_API_KEY` with the GitHub variables | `RealModel`: an unscripted model, told only where wtc's guidance is, adds a repository and publishes; the test checks the outcome on GitHub |

Logins exist only inside the test container. The tests leave default branches
alone and remove the branches and pull requests they create.
`WTC_OPENCLAW_KEEP=1` leaves the gateway container running for a look inside
(with its sandbox containers and state directory), and `TestNotYetCovered` lists what no test reaches yet.
