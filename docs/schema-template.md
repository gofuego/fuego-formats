# schema.md template

Every format module in this repo ships a hand-written `schema.md` at its module
root (`<format>/schema.md`). It is the **contract** a theme author — or a coding
agent vibecoding a theme — reads to know exactly what the parser emits, without
reverse-engineering the source. Changing an emitted node type, attribute, or
slug rule is a **breaking release** for that module.

A `schema.md` MUST contain these six section headings, spelled exactly. CI
(`tools/schemalint`) fails the build if any are missing:

- `## Claims`
- `## Envelope keys`
- `## Node types`
- `## Tree shape`
- `## Slug derivation`
- `## Stability`

Copy the skeleton below when adding a new format.

---

## Claims

Which files this parser claims by default (the base-name glob patterns and any
well-known literal filenames), and how to override them (`WithPatterns(...)`).
State that claims match **base names only** — no path scoping, no content
sniffing.

## Envelope keys

Every key the parser writes into the page envelope, its type, and how the value
is derived. All values are JSON-shaped (string, number, bool, or nested
maps/slices of those) so pages stay cache-eligible. Note the conventional keys
the engine reads — `title`, `layout` — and any format-specific keys.

## Node types

Every `core.Node.Type` the parser emits, each an exported Go constant. Node
types are **prefixed with the format slug** (e.g. `mermaid-diagram`) so two
formats' renderer templates never collide. Document each node's `Content`,
`Raw` flag, and `Attributes`.

## Tree shape

The shape of the emitted page tree: for a single-page format, "one page, N
nodes". For a TreeParser format, the root plus the child slug paths and how they
nest. Trivial formats state plainly that they emit one node and no child pages.

## Slug derivation

How the routed URL / title is derived from the source. Parsers never emit slugs
or routes themselves — routing is the engine's job — but document what the
envelope `title` is derived from and any slug-affecting behavior.

## Stability

The stability posture of this contract (e.g. pre-1.0: node types and envelope
keys may change between minor versions; each change is a breaking module
release). Point at the golden fixture under `testdata/` as the machine-checked
form of this contract.
