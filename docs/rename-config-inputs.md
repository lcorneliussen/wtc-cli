# Configuration inputs for the taba rename

This is a phase-0 increment of [#201](https://github.com/lcorneliussen/wtc-cli/issues/201), preparing existing `wtc 0.1.x` clients for the rename. Use the target release's notes to determine whether it has shipped. There is no migration command or need to rename files yet.

## Accepted names

| Input | Legacy name | Alternate name |
| --- | --- | --- |
| Harness configuration | `wtc.toml` | `taba.toml` |
| Harness CLI version pin | `.wtc-cli-version` | `.taba-version` |
| Control-root defaults | `wtc.env` | `taba.env` |
| User configuration directory | `$XDG_CONFIG_HOME/wtc` | `$XDG_CONFIG_HOME/taba` |
| User data directory | `$XDG_DATA_HOME/wtc` | `$XDG_DATA_HOME/taba` |
| Collection task context | `WTC-SCOPE.md` | `SCOPE.md` |
| Skill overlay base digest | `.wtc-base.sha256` | `.taba-base.sha256` |

Configuration and pin files stay in the harness directory. Ad-hoc configuration accepts `taba.toml` inside the existing `.wtc/config` directory. The `.taba/config` directory is not selected by this increment.

When XDG variables are unset, user directories use `~/.config` and `~/.local/share` as before. Explicit config/data-root settings keep their existing precedence over these defaults; both `TABA_` and `WTC_` prefixes are accepted as described in [environment compatibility](rename-compatibility.md).

## Selection and writes

For each pair, an existing legacy path wins. Only when that path is absent does the alternate name become eligible. Older clients still write legacy files, so this order prevents a newer client from silently selecting a second configuration. An invalid or dangling legacy input is not bypassed in favor of an alternate file. The files are not combined.

This differs from environment variable spelling: **within the selected source**, `TABA_X` wins over `WTC_X`. For example, if both defaults files exist, only `wtc.env` is read; a `TABA_STATUS_WATCH` assignment inside that file wins over `WTC_STATUS_WATCH` there. Environment settings still precede file defaults.

Reads do not create, rename, copy or symlink the selected inputs. When neither path exists, fresh scaffolding and generated output keep their legacy names. Existing `SCOPE.md` task context is preserved by scope seeding. Harness initialization refuses to create legacy files beside existing alternate config/pin files.

An explicit `wtc upgrade` updates the selected pin and configuration **in place**, including alternate filenames. Its dry-run report and dirty-file protection refer to those actual files. It does not create legacy files that would shadow them. Forking an ad-hoc collection carries over the selected configuration's contents but writes the fresh sibling's usual `wtc.toml`.

For user config/data roots, the existing legacy directory likewise wins over an alternate directory. If neither exists, the default stays `wtc`; lookup does not create either directory. Explicit root overrides remain available when a particular location must be selected.

## Later increments

Runtime and generated-state namespaces such as `.wtc`, `.wtc-prs`, service runner state, review records and caches retain their existing names. Changing a live runtime namespace needs a coordinated transition so clients do not start separate runners for one collection. Invocation-aware branding and eventual default-name changes remain separate work in the [rename plan](https://taba.sh/wtc-is-now-taba/).
