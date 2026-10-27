## Review privacy

- Review bundles now default to public mode for public or unknown visibility,
  different forge namespaces, or an unknown harness namespace. Confirmed private
  targets in the harness namespace retain private mode; `--private` explicitly
  opts into harness context, while `--public` forces isolation.
- Public bundles use target base-commit concerns under `review/concerns/` and
  `.review/concerns/`, or embedded defaults, with embedded prompts. They exclude
  harness overlays, related patches, repository snapshots, and private rounds.
- Bundle output, manifests, and progress/verdict comments expose the selected
  mode. Private bundles require fresh private visibility before posting or
  resolving, including progress/failure posts; `--force` cannot bypass this.
- Harness adoption: update overridden local-review skills to use automatic
  selection and inspect the mode. Rebuild and rerun old private bundles with
  `--public` before publishing to public or unknown audiences. See
  `docs/review-audiences.md` for the exact contract and limitations.
