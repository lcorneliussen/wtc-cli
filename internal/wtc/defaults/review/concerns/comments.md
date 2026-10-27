---
id: comments
title: Code comments and refactoring
tier: fast
applies: *.go *.rb *.py *.js *.jsx *.ts *.tsx *.cs *.fs *.c *.h *.cpp *.hpp *.rs *.java *.kt *.swift *.php *.sh *.bash *.zsh *.sql *.nix *.yaml *.yml *.toml *.vue *.svelte Dockerfile* Makefile*
---
Review comments in changed code and the nearby code needed to understand it.
Produce a short cleanup plan, not a repository-wide comment purge. Do not edit
the source. Group findings by root cause to avoid a stream of stylistic nits.

Separate the plan into:

- **Delete** — comments that merely repeat obvious code, stale descriptions,
  abandoned commented-out code, and change-history narration already in Git.
  Confirm that removal loses no useful explanation or active tooling directive.
- **Refactor first** — a comment compensates for confusing names, tangled
  control flow, duplicated logic, hidden ordering or a fragile workaround.
  Explain the underlying problem and a concrete refactor (for example naming,
  extraction or an explicit invariant). Keep the explanation until the code
  makes the contract clear. A necessary compatibility workaround may need to
  stay; do not propose removing it without evidence its constraint is gone.
- **Keep** — rationale, invariants, external contracts, non-obvious algorithms,
  performance or safety constraints, compatibility references, and public API
  documentation. Preserve license notices, generated-file markers, compiler or
  lint directives and documentation consumed by tools. A TODO with a concrete
  unresolved constraint is useful evidence, not automatically a deletion.

In `notes`, give a high-level Delete / Refactor first / Keep plan with the most
useful examples, or say no actionable cleanup was found. Emit findings only
when supported by a specific file and line. Prefix actionable titles with
`Delete:` or `Refactor first:` and explain what information must be preserved.
Existing debt outside the change belongs in notes unless this PR makes it worse.
Never demand a refactor solely to avoid a useful comment.

Intentional behavior does not excuse an inaccurate explanation. A comment can
misdescribe an intentional sentinel, refusal or inference; name the misleading
claim and propose a clarification without silently changing the behavior.
Judge the explanation separately from a request to change the chosen policy.
Purely optional style preferences belong in notes and have no gate significance.
Do not suppress a demonstrated clarity defect merely because an author calls
the underlying behavior intentional.

Severities:

- **major** — a misleading comment or workaround description hides a realistic
  behavioral defect introduced by this change; explain the defect.
- **minor** — stale or misleading explanation, dead commented-out code, or a
  concrete maintainability problem this change introduces.
- **nit** — a small redundant comment whose removal clearly improves reading;
  consolidate repeated cases into one finding.
- Do not mark cosmetic comment preferences as blockers.
