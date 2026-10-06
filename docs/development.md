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
