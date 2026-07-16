# Dependency licenses — openapi module

Vetted 2026-07-15 against the open-core rule (permissive only, no copyleft).
Re-vet when bumping kin-openapi: `go list -deps` the module and check any new
transitive entry's LICENSE at the module root.

| Module | Version | License |
|---|---|---|
| github.com/getkin/kin-openapi | v0.142.0 | MIT |
| github.com/go-openapi/jsonpointer | v0.22.5 | Apache-2.0 |
| github.com/go-openapi/swag/jsonname | v0.25.5 | Apache-2.0 |
| github.com/oasdiff/yaml | v0.1.1 | MIT |
| github.com/oasdiff/yaml3 | v0.0.14 | MIT + Apache-2.0 (dual) |
| github.com/santhosh-tekuri/jsonschema/v6 | v6.0.2 | Apache-2.0 |
| golang.org/x/text | v0.14.0 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT + Apache-2.0 (dual) |
| github.com/gofuego/fuego, …/formatkit | — | Apache-2.0 (ours) |

Verdict: all permissive; no GPL/LGPL/MPL anywhere in the graph. ✅
