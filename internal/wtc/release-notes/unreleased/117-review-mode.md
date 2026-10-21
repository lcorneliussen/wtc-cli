## Review

- **Review someone else's PR without writing to it.** `wtc review start
  <repo>#<n>`, or `wtc new --pr <repo>#<n> --review`, creates a review-only
  collection ([#117](https://github.com/lcorneliussen/wtc-cli/issues/117)):
  - Pushing from the PR worktree is disabled in that worktree's own Git
    config, so neither an agent nor a tool can push by accident: every remote
    gets a dead push URL, other URLs are rewritten to it, and a `pre-push` hook
    refuses the rest. Only `--no-verify` to a URL that no remote had at
    creation gets past the hook. Other worktrees of the repository can still
    push. This turns on `extensions.worktreeConfig` (format version 1) for the
    repository; in a bare owner, `core.bare` moves into the owner's own
    `config.worktree`, as Git requires. A remote with its own
    `remote.<name>.pushurl` cannot be locked, so creation refuses it before
    changing the repository.
  - `wtc review bundle` skips catch-up there and moves the worktree to the PR's
    current head, so running it again after the author pushes starts the next
    round. A commit made in the collection stops it rather than being lost.
    Catch-up leaves the PR worktree alone.
  - `wtc review resolve` and `wtc review ready` refuse, `--user-authorized`
    included: those steps belong to the author.
  - The launch note, the first agent prompt and `wtc status` say review-only;
    status JSON carries `review_only` on the repository row.
