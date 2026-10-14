# wtc unreleased

## Ad-hoc collections

- `wtc init` makes an empty folder a collection without a harness repository.
  Its registry, `.wtc/config/repos.yml`, is local and tracked nowhere;
  `.wtc/config/` also takes `wtc.toml`, hooks and overrides.
- In an ad-hoc collection, `wtc add-repo` accepts a clone URL or a path to an
  existing checkout. The first add asks for the development branch and PR
  target (`--target` answers it). Clones are shared per user under
  `$XDG_DATA_HOME/wtc/bare`, and added repositories are remembered by name for
  later ad-hoc collections.
- `wtc retire .` retires an ad-hoc collection in place when Herdr is not in use.
- In ad-hoc collections, `wtc new`, every `--all` sweep, `--collection`, and
  `open`, `catch-up` or `retire` of another collection are refused for now.

Harness-backed workspaces are unchanged and need no harness upgrade, with one
exception: `wtc retire` now also removes the generated `.wtc-catch-up.json`
report and its `.md` companion, which used to stay behind as leftovers.
