# docker — parser contract

A parser for Dockerfiles: one file becomes one page with a node per build
stage (`FROM`), per instruction, and per comment. Stage and instruction
attributes carry the identifiers relationship hooks read — the stage image
and alias, each instruction's enclosing stage, and `COPY --from` references —
so a consumer like fuego-devops builds its architecture graph from node
attributes alone, never re-parsing the file. **That attribute contract is
public API** (see Node types); fuego-devops's `builds` edges (Dockerfile →
workload) match stage `image` values against container images.

The parser accepts optional YAML frontmatter (a scanner front-end like
fuego-devops emits `title`/`source_path`/`resource_kind` that way); a plain
Dockerfile without frontmatter parses identically.

## Claims

Default patterns — the upstream naming conventions plus the `*.dockerfile`
extension form scanner front-ends emit:

```
Dockerfile   Dockerfile.*   *.dockerfile
```

The `*.dockerfile` pattern is **load-bearing**: under specificity dispatch
(fuego ADR-018) declared patterns are a parser's complete claim set — its
`Type()` is not an extension claim — so extension-named files parse only
because this pattern claims them. Override entirely with
`formatkit.WithPatterns(...)` (e.g. `Containerfile`). Claims match base names
only — no path scoping, no content sniffing.

## Envelope keys

All values are JSON-shaped (strings, `[]any`), so pages stay cache-eligible.
The parser emits **no `layout` key** — a deliberate deviation from the other
fuego-formats modules, so pages use the consuming site's default layout and a
pack's layout semantics (fuego-devops's `default.html`) are untouched.

| Key | Type | Derived from |
|---|---|---|
| `title` | string | frontmatter `title` when present; else `Dockerfile (<alias>)` from the **last** stage's alias; else `Dockerfile — <image>` from the first base image; else `Dockerfile` |
| `images` | `[]any` of string | every `FROM` base image, in order; only when at least one exists |
| `resource_kind` | string | always `Dockerfile` (drives a by-kind taxonomy) |

Frontmatter keys pass through unchanged (fuego-devops adds `source_path` and
`resource_kind` there; the parser's `resource_kind` overwrite is identical).

## Node types

All exported as Go constants, prefixed `docker-`. Emitted in file order.

| Constant | Value | Content / attributes |
|---|---|---|
| `NodeStage` | `docker-stage` | content = the full `FROM` line; attrs `image`, `alias` (empty without `AS`) |
| `NodeInstruction` | `docker-instruction` | content = the instruction's arguments; attrs `instruction` (uppercased), `stage` (enclosing stage's alias, only when named), `copyFrom` (the `--from=` stage of a COPY, only when present) |
| `NodeComment` | `docker-comment` | content = the comment text after `#` |

Graph-relevant attributes (the public contract consumers read): `image` and
`alias` on stages, `stage` and `copyFrom` on instructions.

## Tree shape

One page, N nodes — this is not a TreeParser; a Dockerfile emits no child
pages.

## Slug derivation

The parser never emits slugs or routes — routing is the engine's job. The
envelope `title` derives as documented under Envelope keys; note that the
engine's filesystem-mirror tier strips only the final extension
(`api.dockerfile` → `/api/`, `Dockerfile.prod` → `/Dockerfile/` — keep
variants in their own directories, or route by pattern).

## Stability

Pre-1.0: node types, envelope keys, and attribute names may change between
minor versions, and **each such change is a breaking release of this
module** — the graph-attribute contract above doubly so, since hook code
depends on it. The machine-checked form of this contract is the golden
node dump under `testdata/` (`*.dockerfile` input → `*.golden.json`),
regenerated with `go test ./docker -update`. Out of scope for v1: line
continuations (`\`), heredocs, `ARG` substitution in `FROM`, and multi-line
instruction folding — each physical line is one node.
