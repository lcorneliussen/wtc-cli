# Development and validation

Build with Go 1.27 or newer:

```sh
go build -o wtc ./cmd/wtc
go test ./...
go vet ./...
```

## Running a development build in a collection

When a collection has the wtc-cli repository checked out beside the harness,
build it into the collection with:

```sh
wtc dev install    # builds the wtc-cli sibling into .wtc/dev/wtc
wtc dev status     # which wtc is in effect here
wtc dev uninstall  # back to the pinned release
```

From then on, any `wtc` started inside that collection (a pane, an agent, a
hook; harness-based or ad-hoc) hands over to the dev build, as long as the dev
build is the same version as that `wtc` or newer. The dev build's version comes
from `git describe`: a build three commits past `v0.1.40` is `0.1.41-dev.3`,
above 0.1.40 and below 0.1.41. When the pinned release moves past the build, the
pinned binary runs itself again and notes this on a terminal. Run
`wtc dev install` again after each change. `WTC_DEV_BUILD=off` skips the
handover for one command, and `wtc --version` names the version a dev build
stands in for.

`wtc retire` removes the dev build with the collection.

The dev build is run only if it is an executable owned by you and not writable
by others. Plain `go build` binaries report version `dev` and never hand over.
The handover itself needs a pinned release that includes it (0.1.41 or later).

Tagged releases build macOS and Linux binaries through GoReleaser. Generic
instructions and skills live under `internal/wtc/defaults/` and are embedded
in the CLI. Project-owned overrides belong in harness repositories.

## Runtime integration

The Go suite covers opt-in configuration, read-only status, resource ownership,
lifecycle failures, and retirement. To enable the real runner tests:

```sh
WTC_TEST_DEKIT=/absolute/path/to/dekit go test ./internal/wtc -run TestDekitRuntimeIntegration -v
WTC_TEST_DEKIT=/absolute/path/to/dekit WTC_TEST_DOCKER_IMAGE=postgres:17-alpine go test ./internal/wtc -run TestDekitSharedDockerPostgres -v
```

Use dekit 0.10.0 and pull the supplied Docker image first. The Docker test
creates one disposable container, without host ports, and two synthetic
collection-owned databases. It removes the container and its volumes afterward.
It does not use an existing shared database.

CI verifies the dekit archive checksum and uses a pinned Postgres image digest.
Actual Azure provisioning and connectivity across machines remain onboarding
checks for the project using them.

[Release notes](release-notes.md) are committed and embedded separately from
generated harness guidance. Before tagging, add the matching numbered note;
the release workflow publishes that exact file as the GitHub release body.

[Public demos](demos/README.md) record actual collection and bootstrap commands
against synthetic local repositories. They need no forge account or agent.
