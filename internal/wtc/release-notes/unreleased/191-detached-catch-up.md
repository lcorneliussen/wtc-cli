## OpenClaw: catching up

- **A detached repository catches up.** ↓ on a repository that rests
  detached at its base, like the harness or one with no work yet, failed
  with "not on an openclaw/<name> branch". It now moves it to the base's new
  tip. One with commits of its own on no branch is refused until they are on
  a branch ([#191](https://github.com/lcorneliussen/wtc-cli/issues/191)).
- **A failed action reads as one.** The card and the panel show it with ⚠,
  in red, instead of in the muted line where "Catching up…" stood, and
  without wtc's own "wtc: " prefix.
