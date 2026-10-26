## Next-action controls

- The terminal table, live status view, OpenClaw card and Collection panel
  show the next action and its reason for each repository.
- In the live view, `n` selects actions. Enter runs a catch-up needing no
  approval, opens a pull request, or asks the idle collection agent for the
  named skill. A plain terminal shows the instruction to give the agent.
- OpenClaw rechecks status before running a selected action. Agent handoffs
  use the existing skills; publishing retains the personal-account
  confirmation. Repeated clicks cannot start the same action concurrently.

- Actions are offered manually in this release. Automatic reactions and
  immediate refresh after agent completion remain follow-up work.
