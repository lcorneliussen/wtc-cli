---
name: wtc-local-review
description: Review a draft PR with separate headless concern agents before marking it ready.
---

# Local review gate

Use `wtc review` from a worktree collection. A draft PR becomes ready only
after a separate reviewer process posts a passing summary for its current head.
The review is independent of the authoring agent; do not write its findings,
summary, or verdict yourself.

1. Build a bundle with `wtc review bundle <repo> <pr>`. For a public or
   unknown-audience PR, add `--public` to exclude other repositories and local
   concern overlays. A known PR checks out its branch and catches up the
   collection before the diff; `--no-catch-up` skips that refresh. Branch-only
   reviews use current local refs. Inspect the bundle before giving it to an
   external reviewer.
2. Run `wtc review run <bundle-dir>` without `--post`. Inspect `summary.md`
   and planned inline comments against the destination's audience, then run
   `wtc review post <bundle-dir>`.
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
