# mermaid — schema

The contract for `github.com/gofuego/fuego-formats/mermaid`. It follows the
repo's [schema template](../docs/schema-template.md). Changing an emitted node
type, attribute, or slug rule is a breaking release for this module.

Mermaid is the **trivial tier**: one `.mmd` file becomes one page carrying a
single raw node. Rendering is entirely client-side — the parser never renders a
diagram to SVG; it wraps the source for `mermaid.js` to render in the browser.

## Claims

Default claim pattern: `*.mmd` (base names only — no path scoping, no content
sniffing). Override it per registration with `formatkit.WithPatterns(...)` (also
re-exported as `mermaid.Option`), e.g. for a repo whose diagrams are `*.mermaid`:

```go
eng.Register(mermaid.Parser(formatkit.WithPatterns("*.mermaid")))
```

## Envelope keys

| Key | Type | Always set | Derived from |
|-----|------|-----------|--------------|
| `layout` | string | yes | constant `mermaid` (`mermaid.Layout`) — a theme provides `theme/layouts/mermaid.html`; absent it, the engine falls back to the base template |
| `title` | string | no | the diagram's own title metadata (see Slug derivation); omitted when the diagram declares none |

All values are JSON-shaped, so pages stay cache-eligible.

## Node types

One node type, exported as a Go constant and prefixed with the format slug:

| Constant | Value | Raw | Content | Attributes |
|----------|-------|-----|---------|------------|
| `mermaid.NodeDiagram` | `mermaid-diagram` | true | the diagram source wrapped in `<pre class="mermaid">…</pre>`, with the inner text HTML-escaped so `<`/`&` in the diagram survive into the DOM as text | `source`: the trailing-newline-trimmed raw diagram source, carried verbatim so a hook or alternate theme can re-render it differently |

The node is `Raw: true` so the engine's default renderer passes the `<pre>`
through unescaped. A theme that loads `mermaid.js` and calls `mermaid.run()`
replaces the `<pre class="mermaid">` block with the rendered SVG.

## Tree shape

One page, one node. No child pages — Mermaid is not a TreeParser format. The
`.mmd` file routes as a normal single page under whatever URL the engine's
filesystem/slug/route tiers assign it.

## Slug derivation

The parser emits no slug or route — routing is the engine's job (filesystem
mirror, explicit `slug`, or config route), same as any single-file format.

The envelope `title` is derived, in order:

1. A Mermaid YAML config-frontmatter block (`--- … ---` at the top of the file)
   with a top-level `title:` key.
2. Otherwise, a `title <text>` directive line (the form used by `pie`, `gantt`,
   and `xychart` diagrams); the first such line wins.
3. Otherwise **unset**. The parser receives only the file bytes (`Parse(raw
   []byte)`) and cannot see the filename, so a filename-stem title is the
   engine's/theme's concern, not this parser's — the theme's base template
   typically falls back to the routed slug.

## Stability

Pre-1.0. The node type, its attributes, and the envelope keys above are the
contract; any change to them is a breaking release for this module. The golden
node-dump fixtures under [`testdata/`](testdata/) (`*.golden.json`, regenerated
with `go test ./mermaid -update`) are the machine-checked form of this contract
and double as worked input→output examples.
