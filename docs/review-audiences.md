# Review audiences and rule ownership

A collection can review a repository outside its harness context. The bundle
mode controls which inputs the reviewers receive; it does not authorize
publication. Usually the upstream project runs its own review gate. A consuming
harness should not impose its internal rules on an upstream contribution.

## Automatic selection

`wtc review bundle <repo> [pr-number]` looks up the target repository's current
visibility using GitHub (`visibility`) or Bitbucket (`is_private`). The default
is **public mode** if visibility is public or unknown, including lookup errors,
missing fields, and unsupported responses. Even a confirmed private repository
uses public mode when its forge or owner/workspace differs from the harness's,
or the harness namespace cannot be established. GitHub owner comparison ignores
case. GitHub `INTERNAL` visibility counts as private for this decision.

Only a confirmed private target in the harness's forge namespace defaults to
**private mode**. Harness visibility and authentication are not evidence about
the target audience. Branch-only bundles follow the same selection rules.

- `--public` always selects isolated public mode.
- `--private` explicitly selects private mode, including for a public or unknown
  target, for local analysis. It does not grant publication permission.
- The two flags cannot be combined. Omitting them selects automatically.

The CLI prints the selected mode, audience, and reason to stderr, preserving the
bundle path on stdout. JSON output includes `public`, `audience`, and
`mode_reason` in the manifest; the sourceable manifest also includes
`REVIEW_MODE`, `PUBLIC`, `AUDIENCE`, and `MODE_REASON`.

## Bundle contents

Public mode reads concern Markdown blobs from the **target repository's review
base commit**, under `review/concerns/` and `.review/concerns/`. IDs must be unique
across these directories. Symlinks are not concern files. If neither directory
contains concern Markdown blobs, it uses embedded CLI concerns. Changes in the
PR or working tree cannot replace the base criteria.

Public mode uses embedded CLI prompts and excludes harness concerns, concern
and prompt overlays, related PR patches, and upstream/downstream snapshots.
Earlier local summaries are included only from public bundles for the same
repository and PR (or branch for branch-only reviews). Target PR text, diff,
commit log, and available forge comments remain review inputs.

Private mode retains the existing harness concern layers, prompt overrides,
related enlisted patches, and registry-defined repository snapshots. It may use
earlier private or public rounds. Treat this context as private even if one
repository in the collection happens to be public.

## Posting and changing visibility

Progress, failure, and verdict comments display the **public/private bundle
mode**. Before posting any private bundle, including legacy manifests without
an explicit mode, the CLI fetches current target visibility again. A public or
unknown destination refuses the entire operation before a summary, inline
finding, reply, or resolution is written. A private-to-public visibility change
between bundling and posting therefore closes the publication path.

`--force` bypasses stale-head checks only. It never bypasses the audience check;
`--private` is not a posting override either. Bitbucket CLI comment and
resolution calls explicitly select the checked workspace and repository; they
do not inherit the worktree or CLI default destination. Rebuild with `--public`, rerun the
review, and inspect the new output to publish on a public or unknown target.
Public bundles can be posted to either audience. An audience lookup is a
point-in-time check, not a lock on repository visibility.

Isolation does not sanitize target content or generated prose. Inspect the
exact bundle before sending it to an external reviewer and the summary and
planned inline comments before posting. The CLI does not scan private terms or
claim that public mode proves every generated sentence safe.
