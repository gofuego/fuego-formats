# openapi — parser contract

A TreeParser for OpenAPI 3.x specs (YAML or JSON): one spec file becomes a
routed section — an API index page plus real pages per tag, per operation, and
per component schema. Children carry their own envelopes, so taxonomies,
collections, and pagination see them natively, and every page of the tree
lists the spec as its manifest `source_path` (editable-as-the-spec in
fuego-studio).

The document is loaded with [kin-openapi] but deliberately **not** run through
full spec validation — an imperfect spec still renders. A spec that fails to
load at all (bad YAML/JSON, unresolvable refs; external `$ref`s are not
followed) is a parse error for that file only: the engine records a LocalFatal
and the rest of the site builds.

[kin-openapi]: https://github.com/getkin/kin-openapi

## Claims

Default patterns — the compound suffixes plus the well-known literal names:

```
*.openapi.yaml   *.openapi.json
openapi.yaml     openapi.json
swagger.yaml     swagger.json
```

Override entirely with `formatkit.WithPatterns(...)` (the escape hatch for a
brownfield repo whose specs are named `*.api.yaml`). Claims match base names
only — no path scoping, no content sniffing. Under specificity dispatch the
compound patterns safely outrank a site-wide bare `yaml`/`json` parser.

Note on URLs: the engine's filesystem-mirror tier strips only the final
extension, so `billing.openapi.yaml` routes to `/billing.openapi/`. Use a
well-known name (`openapi.yaml` → `/openapi/`), a config route pattern for
the `openapi` type, or a directory whose index the spec is, to control the
section's root URL.

## Envelope keys

All values are JSON-shaped (strings, bools, `[]any`), so pages stay
cache-eligible.

Root (API index) — layout `openapi`:

| Key | Type | Derived from |
|---|---|---|
| `title` | string | `info.title` |
| `version` | string | `info.version` |
| `summary` | string | first non-empty line of `info.description` |

Tag page — layout `openapi-tag`: `title` (tag name), `description` (tag
description, when present).

Operation page — layout `openapi-operation`:

| Key | Type | Derived from |
|---|---|---|
| `title` | string | summary, else operationId, else `METHOD /path` |
| `method` | string | uppercase HTTP method (`GET`) |
| `path` | string | the literal path template (`/invoices/{invoiceId}`) |
| `tags` | `[]any` of string | the operation's tags — **taxonomy-scannable** |
| `operation_id` | string | when set |
| `deprecated` | bool | only when true |

Schema page — layout `openapi-schema`: `title` (component name), `type` (the
rendered type string, e.g. `object`).

Missing layouts fall back to the base template silently.

## Node types

All exported as Go constants, prefixed `openapi-`. "slug" attributes in ref
nodes are the target child's slug path **relative to the root page's URL** —
a theme composes the link as root URL + slug.

| Constant | Value | Where | Content / attributes |
|---|---|---|---|
| `NodeInfo` | `openapi-info` | root | content = API description; attrs `title`, `version` |
| `NodeServer` | `openapi-server` | root | attrs `url`, `description` |
| `NodeTagRef` | `openapi-tag-ref` | root | attrs `name`, `slug`, `description` |
| `NodeSchemaRef` | `openapi-schema-ref` | root | attrs `name`, `slug` |
| `NodeOperationRef` | `openapi-operation-ref` | root (untagged ops), tag pages | attrs `method`, `path`, `slug`, `summary`, `deprecated` |
| `NodeOperation` | `openapi-operation` | operation pages | content = description; attrs `method`, `path`, `summary`, `operation_id`, `deprecated` |
| `NodeParameter` | `openapi-parameter` | operation pages | content = description; attrs `name`, `in`, `required`, `type` |
| `NodeRequestBody` | `openapi-request-body` | operation pages | content = description; attrs `required`, `content_types` (`[]any`) |
| `NodeResponse` | `openapi-response` | operation pages, ascending status order | content = description; attrs `status`, `content_types` |
| `NodeSchema` | `openapi-schema` | schema pages | content = description; attrs `name`, `type` |
| `NodeProperty` | `openapi-property` | schema pages, sorted by name | content = description; attrs `name`, `type`, `required` |

Type strings (`type` attributes) render as: a referenced component's name
(`LineItem`), `array of X`, or the primitive with its format
(`string (date-time)`).

## Tree shape

```
<root>                          the API index (a routed page of its own)
├── tags/<tag-slug>/            one page per tag (declared order, then
│   └── <op-slug>/                referenced-only tags alphabetically);
│                                 one child page per operation in the tag
├── operations/<op-slug>/       operations with no tags
└── schemas/<schema-slug>/      one page per component schema, sorted by name
```

An operation with several tags gets **one page under each of its tags** (same
content, distinct stable URLs). Paths are walked in sorted order, methods in
sorted order within a path, so output is deterministic. Two operations whose
slugs collide under the same parent are a parse error (LocalFatal), never a
silent overwrite.

## Slug derivation

A **stability promise** — changing these rules is a breaking release:

- **Operation:** the slugified `operationId` when one is set
  (`listInvoices` → `list-invoices`), else the slugified `METHOD /path`
  (`POST /invoices/{invoiceId}/void` → `post-invoices-invoice-id-void`).
- **Tag / schema:** the slugified name.
- **Slugify:** lowercase; camelCase boundaries become hyphens; every run of
  characters outside `[a-z0-9]` collapses to one hyphen (path parameters lose
  their braces); no leading/trailing hyphens.

The parser never emits URLs or routes — the engine routes the root page
through its normal three tiers and composes child URLs beneath it.

## Stability

Pre-1.0: node types, envelope keys, tree shape, and slug rules may change
between minor versions, and **each such change is a breaking release of this
module**. The machine-checked form of this contract is the golden tree dump
under `testdata/` (`*.openapi.yaml` input → `*.golden.json`), regenerated with
`go test ./openapi -update`. Out of scope for v1: full spec validation,
external `$ref` resolution, callbacks/webhooks pages, and per-media-type
schema detail beyond content-type lists.
