# Retirement readiness

`wtc status` exposes a retirement assessment for every selected collection. It
separates known **blockers** from **unknown** evidence and records when the facts
were observed. JSON uses a `retirement` array; Markdown and terminal status show
a summary and point to the detailed inspection. Older cached snapshots without
this property have no retirement assessment. A cached observation is never an
execution permit.

```sh
wtc status --local --json
wtc retire --dry-run
wtc retire example-task --dry-run --transcript /synthetic/evidence/task.jsonl --json
```

`retire --dry-run` uses the same assessment collector with a fresh read. It also
shows Git heads, source coverage, transcript provenance and proposed follow-up
work. It does not read a saved status verdict or run full status refresh. A dry
run defaults to the current collection; execution still requires an explicit
collection name. `--transcript` is only accepted with `--dry-run`, and `--force`
cannot suppress dry-run blockers.

The current collector reports `blocked` when preservation or unfinished work is
known, otherwise `unknown`. It does not emit `ready` while archive, forge/delivery
and external-resource coverage remain incomplete. An open PR, pending delivery
or active runtime fact already observed by status can add a blocker; a missing
fact cannot clear an unknown. Real retirement retains its existing fresh
preflight checks; stronger enforcement of a complete readiness assessment is a
separate decision. Cleanup automation and confirmation policy remain undecided.

Output can contain real paths, branch names and untrusted conversation text.
Keep sensitive output local and sanitize it before sharing. The dry-run collector
writes no report, cache, registry or source files. Normal status retains its
existing snapshot and refresh behavior; its assessment adds no transcript scans,
hooks or execution. Raw transcript hints are only read when explicitly supplied
to `retire --dry-run` and are never added to status caches.

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

Dirty or locally unpreserved work, unconsumed handoffs, preservation backups and
retired folders with evidence add blockers. Missing or conflicting information
stays unknown. No prose, embedded commands, role claims or old approvals grants
authority.

## Read-only boundary and next step

The assessment reuses collection/retirement-marker recognition and the bounded status
command runner. It does not run full status refresh or retirement preflight:
those broader workflows have different contracts. Git optional locks and the
filesystem monitor are disabled for assessment reads. No hooks, branch return,
fetch, archive change, resource stop, deletion or cleanup is performed.

Before a separate cleanup decision, the owner should verify the current heads
and delivery obligations, account for live resources, preserve unfinished work
and establish archive provenance. A future policy proposal should distinguish
preservation and missing-evidence cases from independently verified completion.
Whether any cleanup should be automatic or require confirmation remains undecided.
