# CLAUDE.md — fuego-formats Contributor Guide

## What is fuego-formats?

A public, Apache-2.0 **flat multi-module monorepo** of reusable format parsers
for the [Fuego](https://github.com/gofuego/fuego) meta-engine. One Go module per
format at the repo root, tagged per module (`mermaid/v0.1.0`,
`formatkit/v0.1.0`, …), so heavy per-format dependencies stay isolated. Requires
Go 1.25+ and Fuego v0.5.0+ (formatkit's TreeParser support needs the v0.5.0
engine; mermaid alone still builds against v0.4.7).

Each format module exposes a `Parser(opts ...Option) core.Parser` registered at
the **user tier** (`eng.Register(...)`) — no pack wrapper, no bundled theme.
Parsers claim files by base-name glob (no path scoping, no content sniffing),
emit format-slug-prefixed node types, and carry JSON-shaped envelope values so
pages stay cache-eligible.

## Layout

```
fuego-formats/
  formatkit/          shared claims/options plumbing (module)
  mermaid/            Mermaid parser (module) + schema.md + testdata/ golden dumps
  openapi/            OpenAPI 3.x TreeParser (module) + schema.md + DEPENDENCIES.md
  dbml/               DBML TreeParser, the hand-rolled block-DSL exemplar (module)
  playwright/         Playwright spec TreeParser, shallow structural (module)
  docker/             Dockerfile parser, migrated from fuego-devops (module)
  kubernetes/         K8s manifest parser, migrated from fuego-devops (module)
  tools/schemalint/   CI lint: every <format>/schema.md has the required sections
  docs/               schema-template.md (the six required sections)
  go.work             local dev only: resolves inter-module deps from sibling dirs
```

`go.work` ties the modules together so format modules resolve `formatkit` (and
each other) from the sibling dirs instead of the last published tags — local
changes are visible immediately. External consumers ignore it
— `go get`/`go install` use each module's go.mod requires.

## Conventions

- **Branch workflow:** `develop` is the default branch; `main` is protected
  (PR-only). Deploy/tag from `main` after the user merges the PR.
- **The schema.md contract:** every format ships a `schema.md` at its module
  root with six required sections — Claims, Envelope keys, Node types, Tree
  shape, Slug derivation, Stability (template in `docs/schema-template.md`). CI
  runs `tools/schemalint` to fail the build if a section is missing. Changing an
  emitted node type, attribute, or slug rule is a **breaking release** for that
  module.
- **Golden fixtures:** each format's `testdata/*.golden.json` is both its
  regression test and its shipped contract example, regenerated with
  `go test ./<format> -update`.
- **Node types** are exported Go constants, prefixed with the format slug
  (`mermaid.NodeDiagram == "mermaid-diagram"`).
- **formatkit** carries the filename-claim boilerplate: `NewParser`,
  `NewTreeParser` (for formats whose artifacts expand into a page tree),
  `WithDefaultPatterns` (module baseline), `WithPatterns` (user override).
- **Commit trailer:** end commit messages with
  `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.
- Race-clean: `go test ./... -race` in each module before merging.
- Permissive dependencies only (no GPL) — the open-core line.

## Adding a format module

1. `mkdir <format>` with `<format>/go.mod`
   (`module github.com/gofuego/fuego-formats/<format>`, require fuego + formatkit).
2. Add it to `go.work`, and to the CI matrix in `.github/workflows/ci.yml`.
3. `Parser(opts ...Option) core.Parser` via `formatkit.NewParser` (or
   `formatkit.NewTreeParser` for multi-page artifacts) with a
   `WithDefaultPatterns` baseline; export node-type constants.
   Lib-backed modules record their license vetting in `DEPENDENCIES.md`.
4. Write `<format>/schema.md` from the template and a golden fixture pair under
   `testdata/`.
5. Add a README index row.

## CI

`.github/workflows/ci.yml` matrixes the reusable `gofuego/.github` `go-ci.yml`
over each module directory (build + vet + test-race), plus a `schema-lint` job
running `tools/schemalint`. `.github/workflows/cla.yml` is the CLA-assistant
gate (signatures on `cla-signatures`, built-in `GITHUB_TOKEN`).
