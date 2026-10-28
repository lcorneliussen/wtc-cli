# Enlisted repository identity

PR enlistment, status enrichment and enlisted branch matching share repository
identity rules. A directory prefix alone does not establish an identity.

For a registered repository named `widget`, a checked-out `ext.widget` is an
alias only when its origin identifies the same forge, owner/workspace and
repository as the registry remote. SSH and HTTPS spellings of that identity
are equivalent. New enlistments using either spelling use the registry name.

For an unmanaged `ext.widget`, the bare spelling `widget` is accepted when
the origin is a supported forge and its repository basename is `widget`.
New enlistments use the actual directory name, `ext.widget`. An unrelated
`ext.notes` pointing at a repository named `widget` does not gain a `notes`
alias. Prefixes are not stripped repeatedly.

The resolver considers Git roots inside the current collection. It does not
borrow an enclosing repository's origin or follow a directory symlink outside
the collection. An unknown name stays unresolved. If a spelling identifies
different repositories or more than one worktree, enlistment reports ambiguity
and status does not guess a forge or attach that spelling to a branch. Use
distinct directory names to remove the collision. This is PR bookkeeping;
it does not rename worktrees or change registry and catch-up selector syntax.

Existing enlistment rows retain their spelling, merge markers and merge commit
metadata when updated through a verified alias. Re-enlisting the same PR does
not add another alias row. Conflicting duplicate rows require reconciliation;
the command does not silently choose one. Status reads existing rows without
requiring a migration. Forge caches continue to use the full forge/repository
identity and PR number.

When an enlisted PR is absent from the open list, status already looks it up
by number. A missing branch or merge marker does not prevent that lookup once
the repository resolves. Scoped status records final merged facts only after
the merge time and settled checks are known. Forge errors, unknown states and
unsettled checks retain their existing behavior; this alias fix adds no new
landing or branch-deletion policy.
