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
