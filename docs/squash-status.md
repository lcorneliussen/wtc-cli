# Status after squash landing

A squash merge preserves a change with different commit identities. A retained
feature branch can therefore have a nonzero ahead count even after all its work
landed. Status keeps `ahead` and `behind` as commit counts for compatibility.
Neither count measures changed files or proves that a branch needs pushing.

Repository rows add `ahead_ref` and `ahead_basis` (`upstream` or
`development-tip`). The count is always `<ahead_ref>..HEAD`. Behind still counts
against the development tip or an open PR's base. `upstream_state` distinguishes
`tracking`, `gone` (Git reports a configured upstream missing), and `none`.
A gone upstream adds `[gone]` to the displayed branch. Terminal views explain
nonzero development-tip ahead counts below the dense tables; Markdown names the
ref beside the count. Old snapshots without these additions remain readable.

An enlisted merged PR still associated with its branch gets a `landing`
observation, also exposed on its repository row:

| State | Evidence and display |
| --- | --- |
| `landed` | Existing ancestry, matching PR head and reachable merge, cumulative squash patch, or cherry equivalence establishes full historical landing. The label says **fully landed; catch-up**. Residual ahead counts stay visible. |
| `follow-up` | Full landing is not established, the recorded PR merge is reachable from the development tip, and HEAD has commits after that same PR's recorded head. The label says **follow-up; inspect unlanded work**. |
| `unknown` | Available evidence does not establish full landing or a descendant of the recorded PR head. The label says **landing unknown; inspect branch**. |

These observations describe historical landing, not equality with today's tip.
Later edits, renames, and reverts on the base do not erase the fact that work
landed. Conversely, comparing only the paths a branch touched cannot reliably
establish landing. A `follow-up` observation identifies additional commits; it
does not prove their content is absent from every later squash. Missing forge
facts or local objects stay unknown rather than becoming a safe-to-drop claim.
Custom squash commit messages need no special handling: evidence uses commit
identities and patches rather than a PR-number search through messages.

Newly recorded final merges save the observed PR head in a separate compatible
comment row in the collection PR registry:

```text
# merged-head widget 7 <merge-sha> <observed-pr-head-sha>
```

Readers associate it by repository, PR number, and matching merge SHA. Existing
records without this row still use available local landing evidence; status does
not guess a head or mix another PR's head with the recorded merge. The saved head
is the forge's observed PR head, not a guarantee that an API preserves an
immutable pre-merge head after later pushes.

The observation does not change next-action authority, automatic return policy,
catch-up landing checks, retirement checks, or ref pruning. Automatic return can
be disabled with `WTC_STATUS_RETURN_MERGED=off` or `[status] return_merged = false`
to inspect retained merged branches. Safe pruning continues to use
`git branch -d`; ancestry refusal retains a squash branch even when its content
landed. No destructive-loss behavior is established by residual counts alone.

Synthetic regressions cover normal ancestry, full squash landing with unrelated
base changes, custom messages, late pushes, partial squash without head evidence,
missing and mismatched merge/head facts, later base edits/reverts/renames, gone
upstreams, raw-count compatibility, and retained local refs.
