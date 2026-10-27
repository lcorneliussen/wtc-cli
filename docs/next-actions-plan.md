# Plan: per-repository choices, mixed batches and conditional follow-through

Status: the fixture foundation, shared choices and explicit single-step selection
are implemented. Shared gateway review jobs and conditional continuation remain
planned; this document describes their intended contract. Tracking: public CLI
issue #179; independent review execution and shared review ownership are in
#218. Review integration remains owned by that separate task.

## Product contract

Each repository shows one recommended action, an explanation, and alternatives.
The person can select suggestions across repositories and run the selection as
one batch. The selected actions can differ. A collection-wide shortcut such as
Catch up or Review is a selection/filter convenience over the same rules; it
does not become a second planner.

Run suggested selects only the recommendations currently shown for eligible
repositories. It does not select blocked rows, replace the person's alternative
choices, or include new repositories that appear later. Present the exact
selection and outstanding authority requirements before dispatch. A batch is
not transactional: successful work stays successful when another target fails.

Follow-through is optional. The person can authorize continuing selected work
to a named boundary, such as draft PR prepared or review completed. After every
action, refresh facts and derive the next action. The displayed chain is a
forecast, not an executable script. Pause a target when its next step is outside
the selected scope, needs unavailable authority, reaches the boundary, fails, or
encounters an unexpected blocker. Independent targets continue.

Existing authorization persists. A continuation boundary is not itself a grant
to publish or merge: retain explicit standing authority where configured and
honor each host's credential/confirmation contract. The current gateway confirms
publication of exact commits; it cannot silently confirm newly authored commits
because an earlier batch was allowed to continue through review. An explicit
future grant for conditional publication would need its own bounded contract.

## 1. Separate facts, choices, plans and execution

Keep deterministic derivation in the CLI so the card and TUI use identical
choices. Agent judgment executes named skills; it does not invent status rules.

| Layer | Owns | Must not do |
|---|---|---|
| Observations | Git/forge/review/job facts, provenance, observed time and exact input identity | Turn failed observations into successful or absent requirements |
| Choices | Recommendation, alternatives, disabled choices and reasons per repository | Mutate the checkout or perform external writes |
| Batch plan | Selected action per repository, dependencies, resources and continuation bounds | Expand selection silently or grant new authority |
| Execution | Claims, dispatch, results, refreshed observations and bounded continuation | Replay an old plan after its preconditions change |

Proposed contract (names are illustrative, not a committed JSON API):

```text
RepositoryChoices {
  input: snapshot identity + observation freshness,
  recommended: action ID or null,
  actions: [{ id, kind, availability, reason, blockedBy,
              executor, requiredAuthority, resources, inputIdentity }]
}
BatchPlan {
  targets: [{ repository, selectedAction, inputIdentity }],
  dependencies, continuation: { stopAt, allowedKinds, limits },
  authorityReferences
}
```

Availability distinguishes available, blocked, waiting and already running.
Execution state is separately queued, running, succeeded, failed, cancelled,
paused or stale. A disabled Merge action is useful even when Resolve conflicts
is the recommendation. Running review offers View progress rather than a
second start request. Request reviewer and Run review can both be available;
they are not forced into an exclusive choice.

Retain the existing `next` field as a compatibility projection of the primary
recommendation while adding the richer choices. A user-visible Wait/View action
is not automatically executable by a reactor. Keep service/tunnel work outside
the first batch implementation.

## 2. Enrich the observations before recommending PR progress

The current planner knows branch/worktree state, checks, forge review decision,
PR lifecycle and mergeability. It returns one step using a priority switch.
It lacks several facts needed to advance a PR usefully:

- Local checkout head, remote PR head, target branch head and unpublished work
  as distinct values; no assumption that local HEAD is the reviewed head.
- Independent review state: absent, queued, running, current result, stale or
  failed; its owner and full context identity, not just the main PR head.
- Requested reviewers and required approval/check policy, including observed
  source and unknown/unavailable cases. An absent forge review decision does
  not prove that review is optional or that no independent review exists.
- Draft/readiness gate, unresolved required findings/threads, permission to
  request/post/merge and the host's usable credentials.
- Active author ownership, in-progress operations, reviewer capacity, and
  related-PR dependencies. A dirty checkout is not consent to commit every file.

Separate unknown, stale, unavailable and genuinely not required. Unknown
mergeability or policy must not be described as merge-ready. Checks with no
reported runs are distinct from unavailable check data. Revalidate exact
head/context and all applicable requirements at dispatch and publication.

Proposed recommendations, subject to policy:

| Situation | Primary | Alternatives / blocked explanation |
|---|---|---|
| Unfinished operation | Resume/resolve operation with owner | Inspect; no parallel checkout writer |
| Dirty author checkout | Review intended changes / commit | Inspect changes; no blanket staging |
| Merged PR, no further work | Return to tip | Inspect retained PR/delivery state |
| Work after merge or detached commits | Preserve work on a new branch | Inspect; never publish to completed branch |
| Failed checks / requested changes / conflicts | Address the relevant blocker | Other blockers remain visible; pick a suitable first task |
| Unpublished intended work | Prepare/update draft PR | Review local change where supported; explain remote-head difference |
| Ready PR with no current independent review | Run review | Request reviewer; Merge gated by actual requirements |
| Review already queued/running | View review progress | Request reviewer if useful; no duplicate job |
| Current review complete; required approval absent | Request reviewer | Inspect review; optional additional review |
| All merge requirements established | Merge or open merge UI, according to host capability | View PR; request additional review |
| Required facts unavailable | Refresh/reconnect/inspect | Explain exactly which actions cannot yet be evaluated |

Independent review can run while CI runs. Failing CI need not make every review
impossible, but fixing it is normally the primary recommendation. A draft can
be reviewed; making it ready remains subject to the existing review gate.
Review completion, a passing review verdict, forge approval and merge authority
are different facts. Do not invent mandatory human approval on repositories
whose policy does not require it, or waive an existing requirement.

## 3. Plan and run a mixed batch

1. Collect one coherent set of observations with per-source freshness. Display
   the recommendations and alternatives without acquiring execution locks.
2. Capture the person's selected actions, targets and continuation bounds.
   Show counts for runnable, waiting, blocked and authority-dependent targets.
3. Recheck selected facts, resolve dependencies and create a plan. A materially
   changed action is stale; do not substitute a new action just because the
   button occupied the same row. An unselected new blocker is not new consent.
4. Acquire resources and atomically claim each dispatchable job. Revalidate
   under the claim. Independent tasks start concurrently within configured
   capacity. A queue wait requires another recheck when dispatched.
5. Record actual results and refresh affected facts immediately. Refresh all
   consumers affected by a dependency change. An agent accepting a prompt is
   only dispatched/running, not completed; require a completion/result signal.
6. If continuation permits it, derive and dispatch the next eligible action
   for that selected target. Bind continuation to observed transitions caused
   by this run; unrelated edits, an external push or changed target/policy pause
   it for reassessment. Do not simply reuse a prior approval on a new head.
7. Stop at the boundary or report a pause/failure per target. Report a collection
   result with successful, waiting, paused, failed and cancelled targets.

Resources include a writer lock for the checkout, capacity for agents/review
workers, a session that can actually execute the handoff, and collection/shared
operations that mutate common env/hooks. Do not start concurrent prompts in a
single author conversation. Initially serialize those handoffs unless the host
supports safely isolated workers. A scoped catch-up can still refresh shared
collection infrastructure: separate/coalesce those steps or serialize them.

Immutable review snapshots allow read-only review work to run beside authors
without sharing their mutable checkout. Review slots, cross-gateway/local
claims and fencing come from #218. The batch coordinator must use that shared
protocol, not a second in-memory review lock. Its own progress and continuation
state must survive restart; never restart unfinished external writes blindly.
Adapters without a shared claim mechanism must report that limitation and
restrict execution accordingly rather than claim duplicate prevention.

Cancellation stops queued work and signals active workers. It does not undo
commits, pushes or comments. An uncertain external-write result requires
reconciliation against the forge before retry. Deduplicate each operation by
action/input/target, then record the resulting commit/PR/review identity.
Retries and review/fix loops have finite budgets and no-progress detection.

## 4. Walkthrough A: four different suggestions

All names and identifiers below are synthetic. Assume publication authority is
available for the exact already-visible docs commit, but not for future commits.

### Snapshot A0: before selection

| Repo | Facts | Primary | Alternatives |
|---|---|---|---|
| api | Clean PR head a1; checks failed | Fix checks | Inspect failed run; review current head |
| web | Clean ready PR w1; green; no current review | Run review | Request reviewer |
| docs | Clean branch d1; two unpublished commits; no PR | Prepare draft PR | Inspect changes |
| tools | Detached; three behind; idle and clean | Catch up | Inspect incoming changes |

Select all four suggestions. Choose continue through review for api/docs,
this step only for web/tools. Preview shows four selected targets, different
actions, one author handoff, one independent review and two deterministic jobs.
The scheduler can run web review, docs publication and tools catch-up while
api's author works, if host/resource constraints permit it.

### Snapshot A1: after the first results

| Repo | Observed result | Next state in the batch |
|---|---|---|
| api | Fix committed as a2; local checks pass | Pause at publication authority for a2; never reuse a1's facts |
| web | Review of w1 completes with findings | Selected review completed; offer Address findings without silently starting it |
| docs | Draft PR exists at d1 | Continue to independent review of d1 if its executor/claim is available |
| tools | Now detached at target tip | Succeeded; no further action |

### Snapshot A2: after later events

The person authorizes api publication, CI starts for a2, and its independent
review can run concurrently. Docs review passes: the continuation boundary is
reached; Mark ready is offered only through its gate and is not automatically
selected. Web remains at its findings. Tools remains complete. The summary can
say two targets completed, one continuing and one waiting for findings to be
addressed; it must not show the original four buttons as though nothing ran.

## 5. Walkthrough B: review and source changes race

| Time | Observations / event | Required behavior |
|---|---|---|
| B0 | api review a1 already owned by another runner; web has a ready PR w1 | api shows owner/progress; only web's Run review is newly selectable |
| B1 | web is selected; user pushes w2 before dispatch | w1 selection is stale; no silent substitution of a new review target |
| B2 | person selects review of w2; slot capacity is full | Job is queued visibly; no claim that review has started |
| B3 | job reaches its slot; w2 is still current | Revalidate/claim exact context and start once |
| B4 | author pushes w3 while review of w2 runs | Mark w2 result stale; never count it as review of w3 |
| B5 | runner crashes after posting but before recording success | Reconcile existing publication; recover without duplicate comments |

A continuation policy may allow review of new heads produced by its own
authorized preceding action. That does not automatically include unrelated
external pushes such as w3. Re-running on external heads belongs to an explicit
watch policy, with its own limits, rather than an accidental endless batch.

## 6. Walkthrough C: linked changes and a partial failure

| Repo | Initial state | Selected action |
|---|---|---|
| api | Clean change to a shared contract; behind base | Catch up, then review |
| client | PR explicitly depends on api's proposed contract | Review the coordinated set |
| docs | Independent ready PR | Request reviewer |

The api catch-up must settle before pinning the api/client review context.
One review set represents both selected PRs and consumes the applicable review
resources once. The docs request can run independently. If api catch-up
conflicts, api pauses, client waits with an explicit dependency reason, and docs
can still finish. A failure does not roll docs back or cancel unrelated work.

After conflict resolution, the person can resume the affected targets. Build a
new manifest of exact api/client heads. If both pass review and policy permits
merging, show the dependency order and evaluate compatibility of each supported
intermediate delivery state. If there is a genuine cycle, require coordinated
review/delivery semantics rather than pretending that either arbitrary order is
safe. No automatic grouping follows from an incidental PR mention.

## 7. Walkthrough D: incomplete evidence and an occupied checkout

Docs is green according to an old cache, but current approval/policy reads fail.
API has uncommitted work owned by an active author. Tools has an unfinished
merge. The view should show Refresh/inspect for docs and explain why merge
readiness is unknown; API's Commit suggestion waits for its owner; tools offers
Resume operation with its owner. Run suggested must not turn those rows into
three concurrent mutations or treat unavailable evidence as no requirements.

## 8. Test strategy: generate observations, then replay histories

The first fixture foundation is implemented alongside this plan:

- `internal/next/nexttest.Cases()` builds deterministic synthetic combinations
  from detached/branched/open/draft/merged/follow-up/closed/in-progress profiles.
  Orthogonal dimensions include host, dirty work, divergence, unpublished
  commits, checks, review decision and mergeability. It constrains conflicting
  PRs to divergent bases and keeps unpublished work on branches.
- `go run ./internal/next/nexttest/cmd` exports JSONL with each input and the
  **current** planner output; `-summary` aggregates it and `-filter` selects
  reproducible case IDs. These types are development fixtures, not a new status
  API. Current outputs do not implement or validate the proposed batch planner.
- `TestGeneratedStatusInvariants` checks properties across those inputs:
  derivation is deterministic and does not mutate its input; collection/action
  identity stays scoped; dirty author work is not bypassed; merged branches are
  never published; merge prerequisites and gateway publication authority hold.

Run it from the CLI repository:

```sh
go test ./internal/next/...
go run ./internal/next/nexttest/cmd -summary
go run ./internal/next/nexttest/cmd -filter 'openclaw/open/dirty=0/behind=0/unpublished=0/checks=success'
```

The initial corpus contains **1,350 cases**, split equally between local and
gateway hosts. It passes the current invariants. Characterizing the actual
outputs confirms eight clean/green/unreviewed combinations without an action,
and two approved/green combinations offering the merge link despite unknown
mergeability. These are explicit gaps for the choices work, not passing tests
of the proposed behavior. The high no-action count also includes legitimate
dirty/waiting states; it is not itself a defect count.

The current switch also prioritizes remote failed checks before unpublished
commits. The A1 walkthrough therefore needs head-associated check evidence:
after a fix is committed locally, a failure on the old remote head must not
keep sending the agent back to fix that same failure instead of publishing the
fix. Add this as a named regression when enriching the observations.

Extend the shared fixture vocabulary in the following implementation stages:

1. Add review/job/ownership/policy/freshness dimensions as those observations
   become real planner inputs. Model unknown and unavailable explicitly, and
   include different local/remote/dependency heads. Validate fixture consistency
   independently from action derivation. Maintain a separate adversarial corpus
   for malformed or contradictory observations instead of calling it realistic.
2. Keep hand-authored expected choices for important scenarios, including the
   walkthroughs above. The generator supplies inputs, never computes its own
   expected recommendations with the production rule being tested.
3. Cover small bounded dimensions exhaustively; use measured pairwise coverage
   and targeted three-way combinations for larger spaces. Add seeded stateful
   generation plus shrinking/minimization; log case ID, seed and event trace so
   a failure can be replayed without credentials or live services. A large case
   count alone is not coverage.
4. Generate legal event histories with a virtual clock, fake forge, deterministic
   job executor and separate expected resource/authority ledger. Events include
   push, base movement, dirty edits, check/review completion, policy change,
   expired lease, duplicate completion, cancellation, restart and uncertain
   network outcomes. Exercise A0→A1→A2, not just disconnected status pictures.
5. Assert temporal properties: one writer per checkout; no duplicate active
   review per context; bounded review slots/fan-out; dependency barriers hold;
   authority is never broadened; stale evidence cannot authorize publication;
   cancelled/stale queued work cannot start; unrelated targets keep progressing;
   retries converge without duplicate external writes; continuation terminates.
6. Feed normalized fixtures through local and gateway adapters and compare
   semantic choices, allowing documented host execution differences. Export
   equivalent collection fixtures to the existing plugin DOM tests and TUI
   tests for selection, alternatives, narrow layout, progress, errors and focus.
   No runtime fixture endpoint or fake status data in a real conversation.
7. Use a small set of real temporary Git histories to verify collector fidelity:
   non-main PR target, merged/post-merge commits, remote head divergence, dirty
   checkout and merge conflicts. Pure synthetic facts cannot prove that the
   collectors or real Git operations behave correctly.

## 9. Delivery slices and acceptance

1. **Plan and fixture foundation (this change):** document the contract and
   walkthroughs, generate existing facts, assert current safety properties.
2. **Choices and missing PR states:** enrich observations and derive alternatives,
   blocked reasons and current-review progress. Keep `next` compatibility.
   Accept when the green/unreviewed PR case always has an explainable path.
3. **Selection and single-step batches:** card/TUI choose heterogeneous targets,
   preview them, revalidate and execute with resource control and per-target
   results. Initially use existing supported actions; serialize unsafe shared
   handoffs. Accept with mixed success/failure/stale/cancel scenarios.
4. **Independent review adapter:** integrate the shared job/claim/slot contract
   from #218. Show unavailable functionality honestly until that contract ships;
   do not duplicate its scheduler in this task.
5. **Conditional continuation:** persist explicit bounds, results and authority
   references; derive each next action from fresh observations. Add restart
   reconciliation, finite budgets and dependency-aware review sets.
6. **Optional reactors and polish:** default to explicit selection, add opt-in
   automatic dispatch only for configured classes/authority; agent completion
   triggers immediate refresh. Document behavior in CLI and companion handbook,
   test an isolated gateway/TUI, then follow normal independent review/release.

Review each slice as a coherent change. Do not ship a visual Run all control
that dispatches only some of its advertised action types, or a continuation
control backed only by predicted `then` strings. Make unsupported choices and
host limitations explicit. No release or live-gateway update is part of the
planning/fixture change.


## Implemented first runtime slice

`choices` supplements `next` in local and gateway repository status. The
recommendation projects into `next`; alternatives retain availability and blocked
reasons. Selection keys include local/base identity, unpublished work and available
PR observations, including the remote head. Green PRs with incomplete review
facts offer Check PR progress rather than disappearing. An unpublished local fix
is offered for publication ahead of old remote check failures; unknown mergeability
never produces a merge-ready recommendation. Dirty merged work first needs a new
branch. Dirty file contents are rechecked by the author handoff, not granted a
blanket staging approval by their file count.

The card/panel and TUI preview explicit heterogeneous selections, then revalidate
individual targets. Existing gateway publication confirmations remain separate.
Selected commands are serialized; author tasks use one prompt per conversation.
A handoff is pending completion. Failures, stale selections and cancellation are
reported without rolling back independent results. No follow-up action is inferred
from `then`, and no new repository silently joins a batch.

Independent review status, required policy and global ownership are not inferred
from missing observations. Check PR progress is an existing skill handoff. The
local independent-review alternative uses the existing skill; the gateway review
alternative is disabled with an adapter requirement. Shared claims, review slots,
persisted continuation and coordinated PR sets remain in subsequent slices.
The fixture exporter now includes actual choices and the compatibility projection.
