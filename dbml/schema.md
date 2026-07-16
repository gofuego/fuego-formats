# dbml — parser contract

A TreeParser for DBML (Database Markup Language) schemas: one `.dbml` file
becomes a routed section — a schema index page (project block, enums, table
groups) plus one real page per table, with columns, indexes, and
relationships as nodes. Children carry their own envelopes, so taxonomies,
collections, and pagination see them natively, and every page of the tree
lists the schema as its manifest `source_path` (editable-as-the-schema in
fuego-studio).

The scanner is hand-rolled and dependency-free — the block-DSL exemplar of
this repo. It is strict at the top level (an unrecognized statement, an
unclosed block, or a malformed ref is a parse error for that file only: the
engine records a LocalFatal attributed to the file and the rest of the site
still builds) and lenient inside blocks (unknown column/index settings are
ignored). Node attributes carry the stable identifiers — table and column
names, with aliases resolved — that future cross-artifact linking hooks
consume.

## Claims

Default pattern — DBML has its own extension, so no compound suffix is
needed:

```
*.dbml
```

Override entirely with `formatkit.WithPatterns(...)`. Claims match base names
only — no path scoping, no content sniffing.

Note on URLs: the engine's filesystem-mirror tier strips the final extension,
so `inventory.dbml` routes to `/inventory/` and its tables to
`/inventory/tables/<slug>/`.

## Envelope keys

All values are JSON-shaped (strings, bools, `[]any`), so pages stay
cache-eligible.

Root (schema index) — layout `dbml`:

| Key | Type | Derived from |
|---|---|---|
| `title` | string | the Project block's name (unset without one — the parser cannot see the filename; a filename-derived title is the engine's/theme's concern) |
| `database_type` | string | the Project block's `database_type`, when set |
| `summary` | string | first non-empty line of the project note |

Table page — layout `dbml-table`: `title` (the table name, verbatim — not
slugified).

Missing layouts fall back to the base template silently.

## Node types

All exported as Go constants, prefixed `dbml-`. "slug" attributes in ref
nodes are the target child's slug path **relative to the root page's URL** —
a theme composes the link as root URL + slug. String attributes are always
present (empty when the source omits them); bools are always present.

| Constant | Value | Where | Content / attributes |
|---|---|---|---|
| `NodeProject` | `dbml-project` | root (when a Project block exists) | content = project note; attrs `name`, `database_type` |
| `NodeTableRef` | `dbml-table-ref` | root, declaration order | attrs `name`, `alias`, `slug`, `summary` (first line of the table note) |
| `NodeEnum` | `dbml-enum` | root, declaration order | content = enum note; attrs `name`, `values` (`[]any` of value names; per-value settings are dropped) |
| `NodeTableGroup` | `dbml-table-group` | root, declaration order | content = group note; attrs `name`, `tables` (`[]any` of table names, aliases resolved) |
| `NodeTable` | `dbml-table` | table pages | content = table note; attrs `name`, `alias` |
| `NodeColumn` | `dbml-column` | table pages, declaration order | content = column note; attrs `name`, `type` (verbatim, e.g. `numeric(10, 2)`), `pk`, `not_null`, `unique`, `increment` (bools), `default` (quotes/backticks stripped: `` `now()` `` → `now()`) |
| `NodeIndex` | `dbml-index` | table pages, declaration order | content = index note; attrs `columns` (`[]any`), `name`, `type`, `pk`, `unique` |
| `NodeRef` | `dbml-ref` | pages of **both** endpoint tables (once for a self-reference), file order | attrs `name` (the ref's name, often empty), `from_table`, `from_column`, `to_table`, `to_column`, `relation`, `on_delete`, `on_update` |

`relation` is spelled out relative to the left endpoint: `>` → `many-to-one`,
`<` → `one-to-many`, `-` → `one-to-one`, `<>` → `many-to-many`. Inline column
refs (`[ref: > users.id]`) emit the same node shape with the declaring
table/column as the from side. Endpoint aliases (`Table stock_items as SI`)
resolve to table names.

## Tree shape

```
<root>                      the schema index (a routed page of its own)
└── tables/<table-slug>/    one page per table, declaration order (leaves)
```

Enums and table groups are nodes on the root page, not pages. Everything is
emitted in declaration order — no map iteration — so output is deterministic.
Two tables whose slugs collide (`user_profiles` and `"user profiles"`) are a
parse error (LocalFatal), never a silent overwrite.

## Slug derivation

A **stability promise** — changing these rules is a breaking release:

- **Table:** `tables/` plus the slugified table name
  (`stock_items` → `tables/stock-items`).
- **Slugify** (the shared fuego-formats convention, `formatkit.Slugify`):
  lowercase; camelCase boundaries become hyphens; every run of characters
  outside `[a-z0-9]` collapses to one hyphen; no leading/trailing hyphens.

The parser never emits URLs or routes — the engine routes the root page
through its normal three tiers and composes child URLs beneath it.

## Stability

Pre-1.0: node types, envelope keys, tree shape, and slug rules may change
between minor versions, and **each such change is a breaking release of this
module**. The machine-checked form of this contract is the golden tree dump
under `testdata/` (`*.dbml` input → `*.golden.json`), regenerated with
`go test ./dbml -update`.

v1 scope boundary of the scanner: `Project`, `Table` (with `as` aliases,
quoted identifiers, column settings, `Note:` statements with `'...'` or
`'''...'''` values, `indexes` blocks), `Enum`, `TableGroup`, and `Ref` (single-
line and block form, inline `ref:` settings, composite endpoints joining as
`"a, b"`, `delete:`/`update:` settings), plus `//` line comments. Out of
scope for v1: `/* */` block comments, `Note { ... }` block statements,
sticky notes, multi-file projects, and per-enum-value settings (the value
names are kept, their settings dropped). Unknown top-level statements are a
parse error; unknown settings inside blocks are ignored.
