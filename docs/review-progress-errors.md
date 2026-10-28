# Review progress posting failures

`wtc review run <bundle> --post` attempts a progress comment before starting
independent concerns and the lead. If that progress write fails, the reviewers
still run locally. The command reports the posting failure immediately and
returns it after the local run, with exit status 1. A reviewer execution error
is retained alongside the posting error.

Completed findings, `summary.md`, `verdict`, and run diagnostics remain in the
bundle. When the run completes, the command prints its result even if posting
failed; JSON output uses `ok: false` with the completed local result in `data`.
A local passing verdict does not establish a posted or trusted review.

After a progress-post error, that invocation makes no further posting attempts.
A remote write may have succeeded without returning a usable comment ID, so
posting the summary or failure automatically could create a duplicate. No
successful publication receipt is manufactured. If a saved comment ID exists,
a failed update retains it and returns the error instead of creating a new
comment; an explicit retry uses the same ID. The receipt for an earlier passing
publication is invalidated before attempting the update.

Inspect the remote comment and reconcile any ambiguous create before explicitly
posting again. Recovery of an unknown created ID, transport migration, and
writable Bitbucket contract certification remain separate work under #60.
This change does not certify a CLI version or authentication contract.

Hosted-review deferral still occurs before the progress attempt. Publication
checks and the trusted ready gate continue to apply to every posting attempt.
