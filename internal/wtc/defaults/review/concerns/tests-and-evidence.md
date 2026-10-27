---
id: tests-and-evidence
title: Tests and evidence
tier: standard
applies: always
---
Is the change exercised, and does the evidence in `pr.md` match the diff?

Check:

- changed behaviour is covered by a test, a check the pipeline runs, or
  concrete evidence in `pr.md` (commands with output, counts, sample rows or
  payloads)
- tests actually assert the new behaviour and would fail without the change;
  not only happy paths when the diff adds error handling
- tests or checks removed, skipped, loosened, or marked expected-to-fail
- evidence is consistent with the diff: names, counts, fields, commands and
  targets refer to what the code now does; evidence predates a later commit in
  `log.txt` that changed the tested code
- repro commands would run as written (paths, flags, targets exist)

Before claiming coverage is absent, inspect all relevant tests and subcases,
including end-to-end assertions outside the nearest unit-test table. Name a
concrete uncovered scenario and verify the cited source line exists. A branch
without its own dedicated test is not by itself a missing behavior test.

For races and stale writes, require evidence that forces the violating order
with a barrier, injected ordering hook or controlled scheduler. A sleep alone
does not prove a regression: explain why the old code would fail the assertion
and the corrected invariant would pass. Cover the affected writers together.

Do not demand tests where the repo has no test layer for that kind of code;
then judge the evidence instead.

Severities:

- **blocker** — evidence contradicts the diff (claims a result the code cannot
  produce), or tests were disabled to make the change pass
- **major** — non-trivial behaviour change with neither test nor evidence
- **minor** — evidence stale or incomplete for part of the change; weak
  assertions
- **nit** — repro command cosmetics
