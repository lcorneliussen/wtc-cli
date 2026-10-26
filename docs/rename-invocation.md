# Invocation-aware command names

This phase-0 increment of [#201](https://github.com/lcorneliussen/wtc-cli/issues/201) prepares the executable for the taba rename. Use a release's notes to determine whether it includes this groundwork. Releases still package `wtc`; shipping both executable names belongs to phase 1.

The command reads its invoked executable name from `argv[0]`. Invoking the same binary through a `taba` symlink or renamed executable selects `taba` for command paths in help, examples, diagnostic prefixes, command hints and terminal headings. Invoking it as `wtc` keeps the existing text. The corresponding `.exe` names are recognized too; unknown executable names retain `wtc`. A shell alias that executes `wtc` still supplies `wtc` as the executable name.

A pinned executable that hands over to a collection-local dev build retains the caller's invocation name, even though the dev artifact remains `.wtc/dev/wtc`.

Only explicitly marked, authored text follows the invocation. Substitutions happen before user values are formatted into messages. User paths, repository names, error details and JSON payload values are not passed through a text-rewriting filter. Human-readable summaries can include the selected command name.

Technical identities retain their current spellings: environment variables, file paths, mise backend, executable packaging, OpenClaw plugin ID, skill names, protocol/schema markers, and machine action commands. For example, help invoked through `taba` still correctly says `openclaw plugins reload wtc`; the argument is a plugin ID. Generated scripts, embedded documents and historical release notes also retain their existing content. This increment adds no rename notice or automatic migration.

Environment-prefix and persistent-input compatibility are documented separately in [environment compatibility](rename-compatibility.md) and [configuration inputs](rename-config-inputs.md). Runtime/generated-state aliases remain further phase-0 work; default-name changes follow the [rename timeline](https://taba.sh/wtc-is-now-taba/).
