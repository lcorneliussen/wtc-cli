## Status and catch-up

- A Bitbucket pull request whose `bb --json` reply is over 64KiB no longer
  stays `UNKNOWN` in status, catch-up and review. `bb` writes the reply in
  one call and exits, and on a pipe only the first 64KiB arrived. Forge CLI
  output is now read from a temporary file, which takes the whole reply, and
  `bb`'s stderr is kept out of it
  ([#126](https://github.com/lcorneliussen/wtc-cli/issues/126)).
