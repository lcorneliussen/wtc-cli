# Environment compatibility for the taba rename

This is the first part of the phase-0 groundwork in [#201](https://github.com/lcorneliussen/wtc-cli/issues/201), before the first taba CLI release. It adds new-prefix reads to an otherwise ordinary `wtc 0.1.x` release. See the [rename announcement and timeline](https://taba.sh/wtc-is-now-taba/) for the wider plan; use the target release's notes to determine whether this increment has shipped.

## Settings accepted under both names

For these settings, the CLI accepts the `TABA_` prefix as well as `WTC_`:

| Settings | Purpose |
| --- | --- |
| `CONFIG_ROOT`, `DATA_HOME`, `HARNESS_REPO` | Control-root, per-user data-root and harness selection |
| `COLLECTION` | Default managed worktree when resolving an OpenClaw command |
| `AGENT_KIND`, `AGENT_ARGS`, `LAYOUT`, `LAYOUT_NARROW_AT`, `PANES` | Workspace and agent launch preferences |
| `STATUS_WATCH`, `STATUS_WATCH_BG`, `STATUS_NO_CLICK`, `STATUS_REPOS`, `STATUS_RETURN_MERGED` | Status preferences and merged-worktree opt-out |
| `FORGE_CACHE_AGE`, `PR_ARCHIVE_HOURS` | Forge cache and archive timing |
| `TWG_SITE` | Workspace tool identity emitted as `TWG_SITE` |
| `REVIEW_AGENT_CMD` | Custom review launcher |

The OpenClaw plugin also accepts `TABA_DECISIONS_OPENAI_API_KEY` alongside `WTC_DECISIONS_OPENAI_API_KEY` after the matching plugin is installed. Gateway and plugin versions must be upgraded together as described in the [OpenClaw upgrade guide](openclaw.md#upgrade).

For example, where a command currently reads `WTC_FORGE_CACHE_AGE`, it also accepts `TABA_FORGE_CACHE_AGE`. Existing configuration can keep the old name.

## Precedence and conflicts

- Within a single source, `TABA_X` wins over `WTC_X`, regardless of assignment order.
- An explicitly set empty `TABA_X` suppresses the legacy value in that source. The setting's existing treatment of empty values still applies: launch/status preferences fall back to file defaults, while an empty TWG-site override or API key disables that selection.
- Environment settings still take precedence over workspace defaults in `wtc.env`, even when the environment uses `WTC_` and the file uses `TABA_`.
- In settings files, the last assignment of each spelling wins. The files remain literal assignment files; shell substitutions are never executed by these readers.
- When both spellings in a source are set differently, the process warns once per setting, naming the variables and the chosen spelling. Warnings go to stderr and never include values. JSON output stays on stdout.

New-prefix defaults can be written in the existing `wtc.env` file. A separate [configuration-input increment](rename-config-inputs.md) accepts `taba.env` when `wtc.env` is absent; the same source and variable precedence rules apply. The generated `.env.collection` reader also recognizes `TABA_CONFIG_ROOT`, although generation continues to emit `WTC_CONFIG_ROOT`.

## This increment keeps existing output names

Generated environments, hook variables, toolchain caches, internal gateway/retirement transport, and review-statistics transport continue to use their existing names. Development controls (`WTC_DEV_*`) and test controls (`WTC_TEST_*`) are not aliased. No directories, harness pins, skills, binaries, repositories, or plugin IDs are renamed by this change.

Persistent [configuration inputs](rename-config-inputs.md) now have dual-name selection in a separate phase-0 increment. Generated-state namespaces and invocation-aware command branding remain further work. Emitting both environment prefixes belongs to the first taba release, as tracked in [#202](https://github.com/lcorneliussen/wtc-cli/issues/202).
