# Pull-request mergeability in status

Status shows a confirmed pull-request conflict as `✗ conflict` in the repository and enlisted PR rows. A clean checkout can still have a PR conflict: the working-tree conflict flag refers to an already-started merge, while the PR merge field describes the proposed merge.

For Bitbucket, wtc supplements unknown mergeability with a local `git merge-tree` prediction when the exact source and destination commit objects reported by the forge are already available. It does not fetch objects, check out branches or start a merge to render that observation. The prediction uses the reported commit pair even when the local branch contains additional changes.

A conflicting pair produces `CONFLICTING`; a clean pair with new base commits produces `BEHIND`; an already-contained base produces `CLEAN`. Missing commit IDs, unavailable objects, unsupported Git versions and histories Git cannot merge remain `UNKNOWN`. A behind count alone never proves a conflict. Predictions describe the forge snapshot's commits, which can be cached; they are not a merge authorization or a statement about unseen remote changes.

JSON includes `merge_source: "local"`, `merge_head` and `merge_base` for successful local predictions, plus `merge_conflicts` when paths conflict. These optional fields appear on repository PR facts and enlisted PR rows. The existing next-action policy receives the conflict observation; a dirty checkout still waits for its work to be handled.
