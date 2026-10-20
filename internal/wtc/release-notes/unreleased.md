# wtc unreleased

## OpenClaw

- Git that wtc and its plugin run in agents' worktrees can no longer be made
  to run programs by the worktree's config. Fsmonitor, hooks, signature
  checking and signing are off for every command, and a checkout whose
  repository or worktree config sets another key that can run a program is
  refused with the key and its fix. The plugin's branch list runs outside the
  clone ([#96](https://github.com/lcorneliussen/wtc-cli/issues/96)).
- `docs/openclaw.md` recommends sandboxed, per-repository conversations for
  gateways shared by people with different rights, and explains what the
  gateway's git trusts ([#105](https://github.com/lcorneliussen/wtc-cli/issues/105)).
