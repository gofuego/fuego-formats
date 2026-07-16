# Dependency licensing

Per the repo convention, lib-backed modules record their license vetting here.
The open-core line requires permissive dependencies (no GPL).

| Dependency | License | Role |
|---|---|---|
| `gopkg.in/yaml.v3` | MIT (with Apache-2.0 portions inherited from libyaml) | Manifest YAML decoding and the NodeSpec fallback re-marshal |

`gopkg.in/yaml.v3` is already a transitive dependency of the fuego engine
itself (config and frontmatter parsing), so this module adds no new licenses
to a consumer's tree.
