# fuego-formats

Reusable, independently-versioned **format parsers** for the
[Fuego](https://github.com/gofuego/fuego) meta-engine. Point Fuego at the
machine-readable artifacts your repo already has — diagrams, API specs, database
schemas, test suites — pick the formats you want, and build (or have an agent
build) a theme that visualizes your whole system. No opinionated product pack,
no theme you didn't choose.

This is a **flat multi-module monorepo**: one Go module per format at the repo
root, tagged per module (`mermaid/v0.1.0`, `formatkit/v0.1.0`, …), so a heavy
per-format dependency never leaks into a format you didn't install.

Requires Go 1.25+ and Fuego v0.4.7+.

## Format index

Each format module exposes a `Parser(opts ...Option) core.Parser`, exported
node-type constants prefixed with the format slug, a hand-written `schema.md`
contract, and a golden node-dump fixture. Register a parser at the user tier:

```go
eng := engine.New()
eng.Register(mermaid.Parser())
```

**Tiers:** *trivial* — one raw node, minimal logic; *hand-rolled* — a bespoke
parser for a block DSL; *lib-backed* — wraps an existing parser library.

| Format | Import path | Tier | Status | Contract |
|--------|-------------|------|--------|----------|
| Mermaid | `github.com/gofuego/fuego-formats/mermaid` | trivial | available | [schema.md](mermaid/schema.md) |
| Markdown | `github.com/gofuego/fuego/parsers/markdown` | trivial | available (in the engine repo) | [schema.md](https://github.com/gofuego/fuego/blob/develop/parsers/markdown/schema.md) |
| OpenAPI | `github.com/gofuego/fuego-formats/openapi` | lib-backed | available | [schema.md](openapi/schema.md) |
| DBML | `github.com/gofuego/fuego-formats/dbml` | hand-rolled | available | [schema.md](dbml/schema.md) |
| Playwright | `github.com/gofuego/fuego-formats/playwright` | hand-rolled | available | [schema.md](playwright/schema.md) |
| Dockerfile | `github.com/gofuego/fuego-formats/docker` | hand-rolled | available | [schema.md](docker/schema.md) |
| Kubernetes | `github.com/gofuego/fuego-formats/kubernetes` | lib-backed | available | [schema.md](kubernetes/schema.md) |
| ADR | `github.com/gofuego/fuego-formats/adr` | lib-backed | available | [schema.md](adr/schema.md) |

Markdown deliberately stays in the engine repo as the co-versioned default
parser — the most common case needs no second module — while still appearing
here in the index (and, later, in the scaffolder) for uniformity.

## formatkit

`github.com/gofuego/fuego-formats/formatkit` is the shared plumbing every format
module uses to implement the filename-claim convention identically. It is
functional-options boilerplate, not a framework:

```go
// inside a format module:
func Parser(opts ...formatkit.Option) core.Parser {
	all := append([]formatkit.Option{formatkit.WithDefaultPatterns("*.mmd")}, opts...)
	return formatkit.NewParser("mermaid", parse, all...)
}

// user side, overriding the default claim for a brownfield repo:
eng.Register(mermaid.Parser(formatkit.WithPatterns("*.mermaid")))
```

## The schema.md contract

Every format ships a `schema.md` at its module root with six required sections
— **Claims, Envelope keys, Node types, Tree shape, Slug derivation, Stability**
— so a theme author (or a coding agent vibecoding a theme) knows exactly what
each parser emits without reading its source. The template lives in
[docs/schema-template.md](docs/schema-template.md); CI
([`tools/schemalint`](tools/schemalint)) fails the build if any `schema.md` is
missing a required section. Changing an emitted node type, attribute, or slug
rule is a **breaking release** for that module.

## Repo layout

```
fuego-formats/
  formatkit/          shared claims/options plumbing (module)
  mermaid/            Mermaid diagram parser (module) + schema.md + testdata/
  openapi/            OpenAPI 3.x TreeParser (module) + schema.md + DEPENDENCIES.md
  dbml/               DBML TreeParser, hand-rolled exemplar (module) + schema.md
  playwright/         Playwright spec TreeParser, shallow structural (module) + schema.md
  docker/             Dockerfile parser, migrated from fuego-devops (module) + schema.md
  kubernetes/         K8s manifest parser, migrated from fuego-devops (module) + schema.md
  adr/                ADR parser + convention helpers, migrated from fuego-adr (module) + schema.md
  tools/schemalint/   CI lint for schema.md required sections (module)
  docs/               schema-template.md
  go.work             local dev: ties the modules together pre-tag
```

## Contributing

Contributions require signing the [Contributor License Agreement](CLA.md) — the
CLA-assistant bot will prompt you on your first pull request. Work on `develop`
(the default branch); `main` is protected and updated by PR.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
