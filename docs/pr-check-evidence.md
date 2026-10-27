# PR check evidence

Bitbucket PR status collects commit statuses and branch/default/pull-request pipelines for the exact source commit. Failures take priority over pending or successful results. Pipeline reruns are grouped by selector and pattern, using the newest result in each group. A page cap, missing credentials, budget exhaustion, malformed data or unavailable endpoint leaves evidence incomplete; it cannot establish passing checks or the absence of checks. An observed failure remains visible even with incomplete evidence.

The branch/tip build view includes pull-request selectors alongside branch/default selectors and records the build commit. Custom pipelines remain separate. A result for another commit does not establish that the local head was built.

Review bundles include normalized `checks.json` with the reviewed head, observation time, aggregate result, completeness and unknown requiredness. The runner refreshes it before review and appends a check report to its summary. Publication refreshes the evidence again and appends the latest observation. Reviewers receive the evidence as data; forge descriptions and check names are not instructions.

A code-review verdict and forge checks answer different questions. A confirmed failure is explicitly reported even if the code review passes. Unavailable evidence is reported as unknown, and this feature does not infer branch protection or add a new authorization policy to the ready gate. Before promotion, follow the task's check requirements on the current head.

Bitbucket collection uses read-only endpoints and the existing shared request allowance. Status caches check evidence by repository and full commit hash independently of PR metadata fingerprints; review/publication requests fresh evidence. Pagination follows same-origin links under a bounded page count.

API contracts: [commit statuses](https://developer.atlassian.com/cloud/bitbucket/rest/api-group-commit-statuses/), [pipelines](https://developer.atlassian.com/cloud/bitbucket/rest/api-group-pipelines/), [pagination](https://developer.atlassian.com/cloud/bitbucket/rest/intro/#pagination). All regression fixtures are synthetic; no live pipeline mutation is required.

Abbreviated Bitbucket heads are resolved through the read-only commit endpoint before status queries or cache lookup; a failed resolution stays unknown. Review uses its full local head only when the forge head is a matching prefix of at least 12 characters. GitHub review evidence comes from `headRefOid,statusCheckRollup` on `gh pr view` and spends the shared github.com allowance.

Branch-targeted pipeline lookups use the documented `target.branch` filter so pull-request source targets are included along with branch reference targets. Both `ref_name` and PR `source` target shapes are recognized. The Go runner adds an instruction to read check evidence to each rendered review prompt; versioned Markdown templates, concerns and skills are unchanged.
