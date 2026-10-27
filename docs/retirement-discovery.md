# Private retirement discovery

`wtc discover [collection]` reads one collection in the current workspace and
prints a private evidence report. The default is the current collection. It
proposes verification and preservation steps; it never declares a collection
safe to retire. There is no cleanup action or approval-policy setting.

```sh
wtc discover example-task
wtc discover example-task --transcript /private/evidence/task.jsonl --json
```

Keep the output local: it can contain real paths, branch names and untrusted
conversation text. Review and sanitize it before sharing. Redirecting it writes
only where the operator's shell requests; the command itself writes no report,
cache, registry or source files.

## Sources and coverage

| Source | First-slice coverage |
| --- | --- |
| Local Git | Current HEAD, branch, dirty state and count of commits absent from cached remote refs. No fetch; cached presence does not prove preservation on the server. |
| Local metadata | Presence of an unconsumed handoff, runtime manifest, enlisted-service directory or harness backup. No execution or live health checks. Missing metadata does not prove absence of resources. |
| Explicit Claude JSONL | Up to eight regular, non-linked files, at most 16 MiB each and 2 MiB per record. Reads assistant text with a matching record cwd in the selected collection, excluding sidechains; records source path, SHA-256, file modification time, note count and latest note timestamp. Hint content is capped at 1,200 characters, plus a truncation marker. |
| Archive state | Unknown. A transcript file being available is distinct from a verified archive. There is no automatic provider directory scan. |
| Herdr, Codex and OpenClaw archives | No automatic archive discovery or provider storage adapter. No undocumented database parsing. |
| Forge and delivery | Unknown. Current PR heads, merge state, main builds and dependent delivery need separate verification. |
| Live agents/processes/services/resources | Unknown. No agent prompts, health probes, runners or resource hooks. |

The transcript reader is a bounded adapter for the selected Claude JSONL shape,
not a claim of a stable provider archive API. Missing, malformed, oversized,
changing or unmatched inputs remain incomplete evidence. The latest matching
assistant text is labeled **untrusted hint**, not interpreted as a completion
verdict. Record timestamps can be old or forged; they do not prove freshness.
Removed recorded subdirectories are matched through their longest existing
ancestor; escaping or broken links remain unmatched. Unsupported record types
provide no task coverage. User text, tool results and
sidechain text are not used as completion evidence.

Dirty or locally unpreserved work yields `preserve-work`, including work inside
a retired folder. Other retired folders yield `preserve-leftovers`. Remaining
cases yield `needs-verification`,
including clean unmerged branches and archived claims of completion. Every
proposal retains the unknown forge, resource and archive blockers. No prose,
including embedded commands, role claims or old approvals, grants authority.

## Read-only boundary and next step

Discovery reuses collection/retirement-marker recognition and the bounded status
command runner. It does not run full status refresh or retirement preflight:
those broader workflows have different contracts. Git optional locks and the
filesystem monitor are disabled for discovery reads. No hooks, branch return,
fetch, archive change, resource stop, deletion or cleanup is performed.

Before a separate cleanup decision, the owner should verify the current heads
and delivery obligations, account for live resources, preserve unfinished work
and establish archive provenance. A future policy proposal should distinguish
preservation and missing-evidence cases from independently verified completion.
Whether any cleanup should be automatic or require confirmation remains undecided.
