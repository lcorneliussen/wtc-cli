## OpenClaw: sandboxes

- **A sandboxed session catches up in its sandbox, and its commits reach the
gateway.** With the plugin's new git service (`sandboxGit` in the plugin's
config, `WTC_GATEWAY_GIT` in the sandboxes' environment), the sandbox reads
the gateway's clones, read-only and with history, and `wtc catch-up` merges
the base there with Git's own conflict state; `wtc catch-up --continue`
finishes a resolved merge. The gateway then takes the session's branch, the
sandbox's commits included, once it has checked that the branch extends its
own, merges only origin's commits and leaves no conflict markers. The service
answers only the gateway's sandbox containers, tells them apart by their
address, serves each only its collection's branches and origin's, refuses
pushes and hands out no forge credential. In a sandbox, `wtc doctor` checks
the service and fails when a credential that could push is present
([#113](https://github.com/lcorneliussen/wtc-cli/issues/113)).
