## Status

- **A branch that deploys itself is the PROD build.** When `production_ref` and `release_ref` are empty, status uses the development ref, except `dev`, whose production ref is `master`. That single branch is the PROD column. A different production ref still keeps the development ref as TEST ([#166](https://github.com/lcorneliussen/wtc-cli/issues/166)).
