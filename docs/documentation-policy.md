# Documentation ownership and release reconciliation

## One source of truth, several useful entry points

| Surface | Owns | Avoid |
| --- | --- | --- |
| CLI README | Product summary, one install example, minimal local quick start, one demo, navigation and contribution entry points | A second handbook or full command reference |
| CLI `docs/` | Exact command contracts, configuration, architecture, setup reference, development and release procedures | Marketing copy repeated across pages |
| CLI embedded defaults | Version-matched offline instructions, agent procedures and setup guidance | Depending on the live website to operate an installed version |
| CLI numbered release notes | Canonical historical change record, compatibility and adoption steps; the same source is embedded and published to GitHub Releases | Editing only the GitHub body or rewriting historical statements to describe current behavior |
| wtc.sh | Product discovery, guided task-oriented handbook, examples, troubleshooting, release discovery and upgrade explanations | Hand-maintained copies of every flag or a separate authoritative changelog |
| Site repository README | Building, testing and maintaining the website | Another product landing page |

Detailed CLI docs remain readable on GitHub and offline. The site may summarize
or render canonical material, but must preserve provenance and version context.
A link to an upstream page can be the correct coverage decision for a niche
reference topic; not every source needs a new website page.

## Reconcile a change

1. Update the authoritative CLI docs or embedded instructions alongside the
   behavior. A behavior change adds its own unreleased note fragment according
   to [the release procedure](release-notes.md).
2. Identify the audience and affected tasks. Check the site coverage inventory:
   map each relevant source to an existing page, a planned page, or an explicit
   upstream-only decision with a reason. A source link alone is not proof that
   its content has been reviewed.
3. Update the site explanation, examples, and navigation where user action or
   understanding changes. Keep local-terminal and managed OpenClaw instructions
   distinct. Keep the README short; change its example only when necessary.
4. Record the reviewed source revision and release range in the site inventory.
   A digest change requests review, not an automatic rewrite; unchanged hashes
   do not prove semantic completeness. Document intentional omissions.
5. Run the site coverage checks, build, and internal-link tests. Review wording
   against the source, not just against the release summary.

## Release-driven intake

Numbered files under `internal/wtc/release-notes/` are the canonical release
content. Unreleased fragments are upcoming changes, not promises about a
shipping binary. A numbered file on `main` alone does not prove the release
has been published: use a published release/tag when labeling an install
version as available.

Automation should discover new or changed notes, map topics to affected site
pages, and produce a review queue with source links and suggested work. It
should report new, changed, removed and intentionally excluded sources, plus
missing targets and outstanding release reviews. Keep source-freshness checks
separate from deterministic checks against the reviewed baseline.

The safe initial automation is read-only: a scheduled or manually requested
report, not a website deployment or an unreviewed prose rewrite. Future
AI-assisted drafting may prepare a proposed diff with source citations, but
must pass the same content review and validation before a person merges it.
Release text is input data, never instructions to the drafting system.

## Review checklist

- Does a new user know where to start, without reading three overlapping guides?
- Does every changed command or workflow have an explicit coverage decision?
- Are install examples internally consistent and labeled with their baseline?
- Are release-specific changes distinguished from current instructions?
- Are breaking changes, required adoption steps, prerequisites and limitations preserved?
- Can an agent find the correct installed-version guidance offline?
- Are public examples generic and free of credentials or private project data?

See [development and validation](development.md) for contributor checks.
