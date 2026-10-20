# wtc unreleased

## OpenClaw

- `wtc openclaw install` also reloads the plugin in the running gateway, so
  installing or upgrading needs no gateway restart. If the reload fails, it
  says to restart
  ([#97](https://github.com/lcorneliussen/wtc-cli/issues/97)).
- The Collection panel warns when the gateway's `wtc` is a different release
  from the one its plugin came from, and names the command that fixes it.
- `docs/openclaw.md` has an Upgrade section, for openclaw-bitbucket too.
