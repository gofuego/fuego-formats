# playwright — parser contract

A TreeParser for Playwright spec files — explicitly **not** a JavaScript
parser. A shallow structural scanner reads `describe`/`test` block structure,
titles, `@tags` in titles, and skip/fixme/slow annotations, and expands one
spec file into a routed section: a file root page plus one real page per
suite and per test. Test and suite envelopes carry their effective tags
(`[]any`, **taxonomy-scannable**), so a `tags` taxonomy gives "browse tests
by tag" for free; every page of the tree lists the spec as its manifest
`source_path` (editable-as-the-spec in fuego-studio).

The scanner **never fails**: dynamic or computed titles, multi-line trickery,
and unbalanced structure degrade to best-effort text and best-effort shape —
a file that runs under Playwright always produces a tree. (There is no
LocalFatal path for this format; contrast the strict dbml module.)

## Claims

Default patterns — upstream Playwright's own compound-suffix convention:

```
*.test.js   *.test.ts
*.spec.js   *.spec.ts
```

Override entirely with `formatkit.WithPatterns(...)`. Claims match base names
only — no path scoping, no content sniffing. Under specificity dispatch the
compound patterns safely outrank a site-wide bare `js`/`ts` parser, if any.

Note on URLs: the engine's filesystem-mirror tier strips only the **final**
extension, so the compound suffix keeps its middle part:
`login.spec.ts` routes to `/login.spec/` (and its tests to
`/login.spec/<slug>/`). Use an explicit `slug`, a config route pattern for
the `playwright` type, or a directory whose index the spec is, to control the
section's root URL.

## Envelope keys

All values are JSON-shaped (strings, bools, `[]any`), so pages stay
cache-eligible.

Root (file page) — layout `playwright`: no title (the parser cannot see the
filename; a filename-derived title is the engine's/theme's concern).

Suite and test pages — layouts `playwright-suite` / `playwright-test`:

| Key | Type | Derived from |
|---|---|---|
| `title` | string | the block's title with `@tag` tokens removed and whitespace collapsed (the raw title if that leaves nothing) |
| `tags` | `[]any` of string | **effective** tags: every ancestor suite's plus the block's own, first-seen order, deduplicated; only when non-empty — **taxonomy-scannable** |
| `annotations` | `[]any` of string | effective annotations (`skip`, `fixme`, `slow`, `only`), inherited the same way; only when non-empty |

Tag text is kept as written (minus the `@`); the engine lowercases taxonomy
term URLs itself (`@Validation` → `/by-tag/validation/`).

Missing layouts fall back to the base template silently.

## Node types

All exported as Go constants, prefixed `playwright-`. "slug" attributes in
ref nodes are the target child's slug path **relative to the root page's
URL** — a theme composes the link as root URL + slug. All attribute keys are
always present (`[]any` values may be empty).

| Constant | Value | Where | Content / attributes |
|---|---|---|---|
| `NodeSuiteRef` | `playwright-suite-ref` | root and suite pages, source order | attrs `title` (cleaned), `slug`, `tags`, `annotations` (effective) |
| `NodeTestRef` | `playwright-test-ref` | root and suite pages, source order | attrs `title` (cleaned), `slug`, `tags`, `annotations` (effective) |
| `NodeSuite` | `playwright-suite` | suite pages (one per page) | attrs `title` (**raw, as written**), `titlepath` (`[]any` of raw ancestor titles plus its own — the Playwright test identity, the stable identifier cross-artifact linking reads), `tags`, `annotations` (effective), `dynamic` (bool) |
| `NodeTest` | `playwright-test` | test pages (one per page) | same attributes as `NodeSuite` |

`dynamic` marks a title that was not a plain string literal (template literal
with placeholders, expression, string continued past the literal) and is
therefore best-effort raw text.

## Tree shape

```
<root>                      the file page (a routed page of its own)
├── <suite-slug>/           one page per test.describe, source order,
│   ├── <test-slug>/          nesting as path segments
│   └── <suite-slug>/...
└── <test-slug>/            top-level tests
```

Children are emitted in source order. Annotation recognition:
`test.skip('title', fn)` / `test.fixme('title', fn)` / `test.only(...)` and
the `describe` variants annotate their declaration; a conditional
`test.skip(condition, ...)`/`test.fixme(condition, ...)` (no title string)
and a bare `test.slow(...)` annotate the **enclosing** block instead of
declaring a test. `test.describe.configure(...)` is structural noise and is
ignored.

## Slug derivation

A **stability promise** — changing these rules is a breaking release:

- **Suite / test:** the slugified cleaned title, nested under its ancestor
  suites' slugs (`Checkout @checkout` → `checkout`, a test inside it →
  `checkout/<test-slug>`).
- **Slugify** (the shared fuego-formats convention, `formatkit.Slugify`):
  lowercase; camelCase boundaries become hyphens; every run of characters
  outside `[a-z0-9]` collapses to one hyphen; no leading/trailing hyphens.
- **Degradations, in keeping with never-fail:** a title that slugifies to
  nothing gets its kind as the slug (`test`, `suite`); colliding sibling
  slugs (repeated dynamic titles) uniquify with a numeric suffix in source
  order (`tc-name`, `tc-name-2`).

The parser never emits URLs or routes — the engine routes the root page
through its normal three tiers and composes child URLs beneath it.

## Stability

Pre-1.0: node types, envelope keys, tree shape, and slug rules may change
between minor versions, and **each such change is a breaking release of this
module**. The machine-checked form of this contract is the golden tree dump
under `testdata/` (`*.spec.ts` input → `*.golden.json`), regenerated with
`go test ./playwright -update`.

**v1 scope boundary — shallow structural, by design.** The scanner recognizes
declarations only at the start of a line, titles only as the first argument
on that same line, and tags only as `@tag` tokens inside title strings.
Nested logic, dynamic titles, and parameterized tests are represented
best-effort as raw text nodes (`dynamic: true`), never a parse failure: a
`test(variable, ...)` inside a loop becomes **one** page titled with the raw
expression text, not one per iteration. Out of scope for v1: evaluating any
JavaScript, tags/annotations passed via the options object
(`{ tag: '@smoke' }`, `{ annotation: ... }`), `test.step` structure, hooks
(`beforeEach`/`afterAll`), fixtures, aliased or re-exported `test` objects,
and brace counting inside `${...}` template-literal placeholders (a spec
whose structure hinges on those may nest imprecisely — still a tree, never an
error).
