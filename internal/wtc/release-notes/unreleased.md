# wtc unreleased

## Added

- `wtc dev install` builds the wtc-cli checkout in a collection into `.wtc/dev/wtc`. From this release on, any `wtc` started inside that collection hands over to the dev build while it is the same version or newer, so panes, agents and hooks run the code under development without PATH changes. `wtc dev status` shows which build is in effect, and `wtc dev uninstall` removes it ([#79](https://github.com/lcorneliussen/wtc-cli/issues/79)).
