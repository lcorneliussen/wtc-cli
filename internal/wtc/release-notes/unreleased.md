# wtc unreleased

- Agents now run `/wtc-local-review` without being asked once a PR's scope is
  complete and CI is green. `wtc env setup` renders `WTC_LOCAL_REVIEW=auto`;
  set `[review] auto_local = false` in the harness `wtc.toml` to turn it off
  (`WTC_LOCAL_REVIEW=off`). Harnesses that override `collection-AGENTS.md`,
  `wtc-pr`, `wtc-draft-pr` or `wtc-local-review` should add the same rule.
