---
name: wtc-local-review
description: Review a draft PR with separate headless concern agents before marking it ready.
---

# Local review gate

Use `wtc review` from a worktree collection. A draft PR becomes ready only
after a separate reviewer process posts a passing summary for its current head.
The review is independent of the authoring agent; do not write its findings,
summary, or verdict yourself.

1. Build a bundle: `wtc review bundle <repo> <pr> --public --no-catch-up` for a
   public PR. Inspect the bundle before giving it to an external reviewer.
   Public bundles exclude other repositories and local concern overlays.
   The current native bundle command requires both flags; use the collection's
   shell tool when private snapshots or catch-up are needed.
2. Run `wtc review run <bundle-dir>`. For a private destination, `--post`
   creates one progress comment and updates it with the final summary. For a
   public or unknown audience, run without `--post`, inspect `summary.md` and
   planned inline comments, then run `wtc review post <bundle-dir>`.
3. Read `summary.md`, `verdict`, and any error output in `run.log`. Address or
   answer every open finding. Use `wtc review resolve <bundle-dir> --reply
   "..."` for inline threads after answering them. A later round retains the
   original inline thread; resolve it using the bundle that first posted it.
4. Push fixes and build a new bundle on the new head. Continue until the
   verdict is `pass`, or `pass-with-notes` with remaining notes answered.
5. Only when the user has asked to mark the draft ready, use
   `wtc review ready <pr> --repo <repo>`. It requires a current posted review
   with a local receipt. `pending`, `error`, and stale reviews keep the gate
   closed.

The default model tiers come from `HARNESS_REVIEW_STRONG`,
`HARNESS_REVIEW_STANDARD`, `HARNESS_REVIEW_FAST`, and `HARNESS_REVIEW_LEAD`.
`--strong`, `--standard`, `--fast`, and `--lead` override them per run. Each
spec is an `agent:model` chain. Supported agents are Claude, Codex, and Grok.

Before every external write, inspect the exact destination and outgoing text.
Public PRs must not include private project names, paths, or adoption links.
