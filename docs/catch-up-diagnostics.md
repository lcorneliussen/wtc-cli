# Catch-up PR lookup diagnostics

When catch-up cannot establish PR state, it leaves the branch untouched and
reports `needs-owner`. The reason retains `PR state unavailable; branch left
untouched`. Forge lookup failures identify the repository and attempted forge
operation. Enlisted lookups name the PR number; a GitHub branch search names the
branch instead. Failures reading local collection configuration, PR records or
origin configuration identify the local operation and the input to inspect.

For example, an enlisted lookup can report:

```text
PR state unavailable; branch left untouched; github.com/example/widget PR #7: gh pr view failed: authentication or authorization failed; check CLI login and repository access, then retry
```

Check the CLI's login and repository access for authentication failures, network
connectivity for transport failures or timeouts, and CLI installation or
compatibility for execution or response errors. A generic failure reports the
exit status: inspect the named operation locally for more detail before retrying.
Bitbucket branches without an enlisted PR number need `wtc pr enlist` before
catch-up can read their PR state. Unsupported or invalid origin remotes need
local configuration inspection; the diagnostic does not print the remote URL.

PR lookup diagnostics classify stderr into fixed causes. They never reproduce raw stderr,
response bodies, or parse errors, which can contain credentials and private
URLs. Branch labels are length-bounded and ASCII-escaped; invalid repository
labels are redacted. Keep detailed local CLI output private when investigating.

The report remains schema version 1 with the existing failure fields, outcome,
SHA values and owner handoff fields. Lookup diagnostics add detail only to the
reason. Dirty, busy, concurrent-branch, merge, landing and retirement decisions
are unchanged. The separate catch-up conflict handoff sentence contract
`conflicts in: … The merge was aborted` and its action JSON are unchanged.
