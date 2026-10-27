## Status

- **Bitbucket PR conflicts are visible before starting a merge.** When the reported source and destination commits are available locally, status predicts the merge without changing the checkout and shows confirmed conflicts in the repository and PR rows. JSON records the commit pair and conflicting paths. Unavailable predictions stay unknown ([#217](https://github.com/lcorneliussen/wtc-cli/issues/217)).
