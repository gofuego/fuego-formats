# adr — parser contract

A parser for Architecture Decision Records following the fuego-adr
convention: Markdown files named `NNN-slug.adr.md` with YAML frontmatter. One
ADR becomes one page — the frontmatter is normalized into the envelope, the
body splits on `## ` headings, and each section is rendered to HTML (goldmark,
GFM) as one raw node. Any site can render ADRs by registering this parser;
the [fuego-adr](https://github.com/gofuego/fuego-adr) pack adds the dashboard,
timeline, affects index, validation hooks, and theme on top.

The module also exports the convention helpers tooling builds on:
`ExtractADRNumber` (the `NNN-` filename prefix), `ValidateSections` (required
sections for accepted ADRs; returns plain section names for human-readable
warnings), `ValidStatuses`, and `RequiredSections`. These are public API of
the convention, shared by fuego-adr's hooks and CLI.

## Claims

Default pattern — the ADR compound suffix:

```
*.adr.md
```

Under specificity dispatch (fuego ADR-018) this safely coexists with a
markdown parser's bare `md` claim: `guide.adr.md` routes here, plain
`notes.md` to markdown. Override entirely with `formatkit.WithPatterns(...)`.
Claims match base names only — no path scoping, no content sniffing.

## Envelope keys

The parser normalizes the convention's frontmatter fields in place; keys the
author writes pass through. All normalized values are cache-registered types
(`[]string` and `[]int` are deliberate — consumer hooks assert them, and both
are in the engine's registered cache set). The parser emits **no `layout`
key**: the consuming pack (fuego-adr's AfterParse hook) owns layout
defaulting.

| Key | Normalized to | Meaning |
|---|---|---|
| `title` | string (as written) | The decision title. |
| `status` | lowercased string | One of `tbd`, `proposed`, `accepted`, `deprecated`, `superseded` (`ValidStatuses`) — the status flow: tbd → proposed → accepted → deprecated / superseded. |
| `author`, `approvers`, `tags`, `affects` | `[]string` | Scalar or list accepted; `affects` holds glob patterns of files/areas the decision governs. |
| `supersedes`, `superseded_by` | `[]int` | ADR numbers this one replaces / is replaced by (fuego-adr validates bidirectionality). |
| `date_proposed`, `date_accepted`, `date_deprecated`, `date_superseded`, `deadline` | `"YYYY-MM-DD"` string | YAML bare dates parse as time.Time; the parser formats them back. |

The ADR number is **not** an envelope key the parser sets — it lives in the
filename (`0012-use-postgres.adr.md`), extracted by `ExtractADRNumber`
(fuego-adr's hook stores it as `adr_number`).

## Node types

One node per `## ` section, in body order, each with `Content` = the
section's Markdown rendered to HTML and `Raw: true`. The node type is
`SectionNodeType(heading)` — `adr-` plus the slugified heading:

| Constant | Value | Source |
|---|---|---|
| `NodePreamble` | `adr-preamble` | content before the first `## ` heading |
| `NodeContext` | `adr-context` | `## Context` |
| `NodeDecision` | `adr-decision` | `## Decision` |
| `NodeConsequences` | `adr-consequences` | `## Consequences` |
| — | `adr-<heading-slug>` | any other heading (`## Options Considered` → `adr-options-considered`) |

`ValidateSections(status, nodes)` enforces `RequiredSections`
(context, decision, consequences) for `accepted` ADRs only, returning the
missing plain section names.

Cross-links: a rendered relative link to another ADR file
(`href="012-foo.adr.md"`, optionally with a fragment) rewrites to the
conventional decision route `href="decisions/012-foo.adr/"` — base-relative,
so it resolves under any deployment base URL. Absolute URLs are left alone.
The target route is the `adr: /decisions/{slug}` pattern fuego-adr's config
defaults establish; a standalone site configures the same route pattern for
the links to resolve.

## Tree shape

One page, N section nodes — this is not a TreeParser; an ADR emits no child
pages.

## Slug derivation

The parser never emits slugs or routes — routing is the engine's job. The
envelope `title` comes from frontmatter. Under fuego-adr's conventional route
pattern, `0012-use-postgres.adr.md` lands at `/decisions/0012-use-postgres.adr/`
(the mirror tier would otherwise strip only the final `.md`). Section node
types use the shared fuego-formats slug convention (`formatkit.Slugify`) on
the heading text.

## Stability

Pre-1.0: node types, envelope normalization, and the cross-link rewrite may
change between minor versions, and **each such change is a breaking release
of this module** — the convention helpers and section node types doubly so,
since fuego-adr's hooks, CLI, and theme depend on them. The machine-checked
form of this contract is the golden node dump under `testdata/`
(`*.adr.md` input → `*.golden.json`), regenerated with `go test ./adr -update`.
Out of scope for v1: heading levels other than `## `, per-section frontmatter,
and rewriting cross-links to routes other than the conventional
`decisions/{slug}`.
