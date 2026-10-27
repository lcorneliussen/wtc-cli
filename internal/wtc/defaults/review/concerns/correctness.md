---
id: correctness
title: Correctness
tier: strong
applies: always
---
Does the change do what `pr.md` says it does, and does it keep doing what the
code did before where it was not meant to change?

Check, reading the surrounding code and not just the hunks:

- logic errors: wrong condition, inverted check, off-by-one, wrong variable,
  unreachable or dead branch, missing `return`/`break`
- data handling: nulls/empties, types and units, duplicates, ordering,
  time zones, integer vs float, join fan-out or row loss in queries
- error paths: failures swallowed, partial writes, retries that are not
  idempotent, resources not released
- every caller of a changed function, query, or config key still matches its
  new behaviour (grep the repo)
- concurrency and ordering where the code runs more than once or in parallel
- the change is complete: renamed or removed things have no leftover references

For a structural defect (race, stale overwrite, missing guard, retry or cache
inconsistency), name the invariant before proposing a fix. Trace the affected
state's writers, readers and publication paths, including fallback, forced
refresh, empty-result and error paths. Consolidate every confirmed violation
of that invariant into one finding, listing the paths checked and any coverage
limits. Suggest a shared boundary or invariant-level fix instead of a patch
to the first offending line. Independent root causes remain separate findings.

Support each finding with a concrete trigger and observable consequence. Check
likelihood, persistence, recovery and impact before assigning severity. A rare
case that self-corrects at the next audit is normally minor or an explicit
accepted risk; explain any higher-impact exception. Do not inflate severity
for an unverified suspicion or weaken a demonstrated data-loss/security risk.

Severities:

- **blocker** — wrong results, data loss/corruption, crash or failed run on a
  path that will execute in normal use
- **major** — a realistic setup produces a persistent wrong state, or an error
  path hides a consequential failure; give the setup and its impact
- **minor** — wrong only on an unlikely input, or a latent bug not reachable
  today
- **nit** — do not use for this concern except for a misleading name or
  comment that will cause a future bug
