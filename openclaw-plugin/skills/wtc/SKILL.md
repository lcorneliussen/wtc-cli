---
name: wtc
description: "wtc projects and their collections under OpenClaw: work on one or more repositories, driven from one conversation. Use whenever the person wants to work on, look into, analyze or change a repository, a product or site built from one, or a wtc project; when a task names an issue; or when they mention wtc, a collection, the Collection panel or its card. Also before adding a repository, publishing, catching up or running a service."
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

- A **wtc project** is a named set of repositories (a harness the gateway is
  configured with); a **collection** is this conversation's worktrees of the
  repositories one task needs. An ad-hoc collection works without a project.
- **Work happens in this conversation's collection**, even when it is only
  reading or analyzing. When the person wants to work on something and the
  conversation has no collection yet, run `wtc projects` to match what they
  named ("the site", "the CLI") to a project's repositories, then start one
  with `wtc add-repo`. Never work in another conversation's collection
  folder, the workspace or a clone of your own, and don't answer from memory
  what the repositories can tell.
- `wtc add-repo <repo> [...] --worktree <issue-id>-<slug>` creates nothing:
  it prints `sessions_spawn` intents. Call `sessions_spawn` once per intent,
  with its `args` unchanged. The wtc project whose repositories these are is
  picked, and a worktree of its harness comes first. Read the harness's
  `AGENTS.md` (`wtc status` shows where it is) right away: it says which
  other repositories a task needs, so add them too.
- A conversation works in one wtc project. When `wtc add-repo` asks which,
  ask the person, then pass `--project <name>`. When it says the project
  does not list a repository, put its choices to the person and run the one
  they pick. When another wtc project lists it, suggest working on that in a
  new conversation for that project. Never read or change another
  project's repositories from this collection unless the person added them
  here ad hoc.
- `wtc status <name>` shows the collection folder; edit, build, test and
  commit in `<folder>/<repo>`.
- **Pushing is the person's.** When work is committed and ready for review,
  get its pull request ready with the **wtc-pr** skill; the person pushes it
  with ↑ in the card. Never `git push`.
- **A "wtc: pull request news" event**: act on it with the **wtc-follow**
  skill.
- Parallel parts of a task go to workers, never to two writers in one
  checkout: `wtc add-repo <repo> --worker <name> --work "<part>"`, then
  `wtc catch-up --from <name>` in `<folder>/<repo>` to merge one in. A
  worker is only for a part that really runs beside yours; work you would do
  next anyway goes in your own checkout. When a worker ends, merge it in, or
  tell the person what is left so they can discard it in the card: none is
  left behind.
- **Catching up** a repository, merging a worker in, and resolving the
  conflicts either brings: the **wtc-catch-up** skill. It never rebases and
  never pushes.
