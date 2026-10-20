# wtc unreleased

## Setup

- `wtc docs setup.md` is one entry point for an agent asked to set wtc up. It
  works out the runtime (terminal, herdr, OpenClaw), whether a harness is used
  and which one, and who maintains the environment, then points at the
  matching guide ([#101](https://github.com/lcorneliussen/wtc-cli/issues/101)).

## OpenClaw

- `wtc doctor` works under OpenClaw, and with `--openclaw` on a gateway host.
  It checks Git 2.42+, `gh`, the plugin and its version, custom Control UI
  plugins, and that each agent's `AGENTS.md` mentions wtc.
- `wtc docs openclaw-AGENTS.md` is the section to add to each agent's
  `AGENTS.md`, so that agents know about collections in every conversation.
- `docs/openclaw.md` covers gateways built by CI/CD: wtc and the plugin in the
  image, the plugins on a load path, configuration as code, upgrades as a
  version bump.
