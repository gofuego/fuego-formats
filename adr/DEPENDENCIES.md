# Dependency licensing

Per the repo convention, lib-backed modules record their license vetting here.
The open-core line requires permissive dependencies (no GPL).

| Dependency | License | Role |
|---|---|---|
| `github.com/yuin/goldmark` | MIT | Renders each ADR section's Markdown to HTML (GFM extensions) |

goldmark is the same library the fuego engine's first-party markdown parser
uses, so this module adds no new licenses to a consumer's tree.
