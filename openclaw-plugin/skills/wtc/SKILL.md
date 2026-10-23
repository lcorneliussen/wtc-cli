---
name: wtc
description: "Worktree collections under OpenClaw: one task across several repositories, driven from one conversation. Use when a task names an issue, spans repositories, or the person mentions wtc, a collection, the Collection panel or its card; before adding a repository, publishing, catching up or running a service for it."
metadata: { "openclaw": { "emoji": "🗂️", "requires": { "bins": ["wtc", "git"] } } }
---

# wtc

`wtc` is on this gateway's PATH, and the gateway shows a conversation's
collection in the Control UI: a card above the message box and a Collection
panel. Its instructions come with the binary, so they always match it:

```bash
wtc docs instructions/openclaw.md
```

Read that before the first `wtc` command of a task and follow it. In short:

- `wtc add-repo <repo> --worktree <issue-id>-<slug>` creates nothing: it
  prints `sessions_spawn` intents. Call `sessions_spawn` once per intent,
  with its `args` unchanged.
- A gateway with harnesses resolves repository names through the
  collection's harness, and adds a worktree of the harness first. When
  `wtc add-repo` asks which harness, ask the person, then pass
  `--harness <name>`. When it says the harness does not list a repository,
  put its three choices to the person and run the one they pick. Read the
  harness's `AGENTS.md` (`wtc status` shows where it is) before working in
  the collection: it says which other repositories a task may need.
- `wtc status <name>` shows the collection folder; edit, build, test and
  commit in `<folder>/<repo>`.
- **Pushing is the person's.** When work is committed, write the pull
  request's description to a file: what changed and why, how it was checked,
  what is left (the repository's PR template when it has one). Then run
  `wtc publish --title "…" --body-file <file>` in `<folder>/<repo>`. It checks
  the work and records the text, and the person's push opens the pull request
  with it. Tell them it is ready: they push it with ↑ for that repository in
  the card, as themselves, after confirming. Carry out the publish plan
  yourself only when they explicitly ask you to. Never `git push`.
- **A "wtc: pull request news" event** says what happened since: a pull
  request opened, checks failed or passed, a review was approved or asked for
  changes, a conflict, or a merge. Update the task's progress with it. For a
  failure, a review or a conflict, look at it (`gh pr checks`,
  `gh pr view --comments`) and tell the person what you will do. After a
  merge, run `wtc catch-up` in that repository's checkout so it continues
  from the merge, then say what is done and what is left. Don't answer
  review comments on the forge yourself.
- Parallel parts of a task go to workers, never to two writers in one
  checkout: `wtc add-repo <repo> --worker <name> --work "<part>"`, then
  `wtc catch-up --from <name>` in `<folder>/<repo>` to merge one in.
- `wtc catch-up` in `<folder>/<repo>` merges the base in; it never rebases
  and never pushes.
- In a sandbox with `WTC_GATEWAY_GIT` set, `wtc catch-up` in the checkout
  merges the base in there, with full history, and hands the branch, your
  commits included, to the gateway. On a conflict, resolve it with git,
  `git add` each file, then `wtc catch-up --continue`. There it takes no
  other flags: `--from` runs in a session outside the sandbox.
