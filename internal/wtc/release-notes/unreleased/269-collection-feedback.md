## OpenClaw

- **Collection panels fill their sidebar host.** Inline-size containment no
  longer shrinks the panel inside a flex layout. Desktop chat-card headers gain
  top space while retaining their bottom inset and shared text baselines.
  ([#269](https://github.com/lcorneliussen/wtc-cli/issues/269)).

- **Collection actions keep their feedback in place.** The compact header
  summarizes progress and outcomes, with full details and recent results in the
  sidebar below Services. Selection augments the existing repository rows,
  an aligned All repos row offers bulk push/catch-up, and narrow layouts keep
  action targets and fact positions stable. Shared name/icon rails and compact
  narrow rows reduce near-alignments and empty PR bands. A publication preview
  can open above the collapsed bar while its repository details stay closed. Bulk arrows share repository arrow
  tracks; PR-history links and action-log result glyphs keep reserved slots. Text shares a baseline and secondary
  controls follow the host theme. Long branches can be expanded.
  Temporary controls extend above the composer card or below sidebar Services
  while the base slots stay in place.
  The browser card separates collection Services with an outer-margin heading
  and divider. Browser table labels use normal capitalization; terminal headings
  keep their uppercase style.
  ([#269](https://github.com/lcorneliussen/wtc-cli/issues/269)).
- **Publication has a separate confirmation preview.** Preparation shows the
  personal account, repository, branch and captured change count. Confirm push
  and Cancel are distinct controls; repeated taps on preparation cannot publish.
  Preparation and confirmed publication have distinct progress labels.
  Pre-publication status responses are discarded and a fresh status is read
  before the row stops showing work in progress. Uncommitted-only rows offer
  Commit changes instead of an unnumbered push arrow.
