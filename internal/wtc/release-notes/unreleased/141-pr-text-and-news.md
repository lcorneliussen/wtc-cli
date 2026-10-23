## OpenClaw: pull requests

- **The agent's title and description reach the pull request.** `wtc publish`
  takes `--title` and `--body` or the new `--body-file`, and records them for
  the branch in the worktree's git directory, never committed. `wtc status
  --json` reports them as `proposal`. The card's ↑ opens the pull request with
  them and shows the title before you push. Without one, the pull request
  still opens as "Publish openclaw/<name>". Bitbucket's publish takes the
  title only
  ([#141](https://github.com/lcorneliussen/wtc-cli/issues/141)).
- **The conversation hears what happened to its pull requests.** It's told
  when one opens or becomes ready, is merged or closed, fails or passes its
  checks, is approved, gets a request for changes, or conflicts with its
  base, with a turn of its own. The plugin looks whenever a card loads, and
  every 5 minutes at collections with an open pull request. The new plugin
  settings `pullRequestEvents` (`wake`, `queue`, `off`) and
  `pullRequestPollMinutes` change that. GitHub only
  ([#141](https://github.com/lcorneliussen/wtc-cli/issues/141)).
