## Status

- **Status shares one Bitbucket and GitHub allowance.** Build lookups are cached with pull requests. One fetch serves every status pane asking for that record. Bitbucket then allows a short burst and about one call every 8 seconds; GitHub allows a larger burst and about one call every 2 seconds. A rate-limit response pauses that forge and status keeps the last saved record ([#215](https://github.com/lcorneliussen/wtc-cli/issues/215)).
