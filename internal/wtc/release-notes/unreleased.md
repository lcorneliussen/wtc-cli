# wtc unreleased

## OpenClaw

- The Collection card no longer offers to push a branch whose pull request
  merged once a catch-up has brought it to the base. The merge commit came
  from the base and was counted as unpublished, so ↑ showed a commit and the
  push failed with nothing to publish. Only commits that are on neither the
  published head nor the base count now.
