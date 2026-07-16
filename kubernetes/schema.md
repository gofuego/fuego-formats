# kubernetes — parser contract

A parser for Kubernetes manifests: one manifest becomes one page of
structured nodes — a resource header (kind, apiVersion, name, namespace),
metadata labels/annotations, and kind-specific detail for workloads
(Deployment, StatefulSet, DaemonSet, Job, CronJob), Services, ConfigMaps,
Secrets, and Ingresses, with a YAML-spec fallback for every other kind.

Relationship hooks read node attributes to build cross-resource graphs —
**that attribute contract is public API** (see Node types). fuego-devops
derives its edges entirely from it: Ingress→Service from `serviceName`,
Service→workload from `selectorMap` matched against pod template labels,
workload→ConfigMap/Secret from `refKind`/`refName`, mounts from
`volumeType`/`refName`, and Dockerfile→workload from container `image`.

The parser accepts optional YAML frontmatter (a scanner front-end like
fuego-devops emits `title`/`source_path`/`resource_kind` that way); a bare
manifest parses the same. A manifest that fails to unmarshal is a parse error
for that file only (LocalFatal) — the rest of the site builds.

## Claims

Default patterns — the scanner-emitted `*.k8s` extension plus the compound
suffixes for repos that name manifests directly:

```
*.k8s   *.k8s.yaml   *.k8s.yml
```

Claiming bare `*.yaml` would be far too greedy; override entirely with
`formatkit.WithPatterns(...)` if your layout needs different names. Claims
match base names only — no path scoping, no content sniffing. The parser's
`Type()` is `k8s` (matching the extension and node prefix), not the module
name.

## Envelope keys

The parser adds **no keys of its own** and emits **no `layout` key** (a
deliberate deviation from the other fuego-formats modules, so pages use the
consuming site's default layout). Frontmatter keys pass through unchanged —
fuego-devops's scanner supplies `title`, `source_path`, and `resource_kind`
(which drives a by-kind taxonomy). All frontmatter values are the author's;
keep them JSON-shaped for cache eligibility.

## Node types

All exported as Go constants, prefixed `k8s-`. Emitted in manifest order;
every map iteration that reaches the output is sorted, so builds are
deterministic. Graph-relevant attributes are marked **(graph)**.

| Constant | Value | Where | Content / attributes |
|---|---|---|---|
| `NodeResourceHeader` | `k8s-resource-header` | every page, first | attrs `kind`, `apiVersion`, `name`, `namespace` **(graph — resource identity)** |
| `NodeMetadata` | `k8s-metadata` | when labels/annotations exist | content `Labels` or `Annotations`; attributes are the pairs verbatim |
| `NodeReplicas` | `k8s-replicas` | workloads | attrs `count` |
| `NodePodTemplateLabels` | `k8s-pod-template-labels` | workloads | attributes are the pod template labels verbatim **(graph — selector matching)** |
| `NodeContainerSpec` | `k8s-container-spec` | workloads, containers then initContainers | content = image; attrs `name`, `image` **(graph)**, `ports` (`"8080/TCP, 9090/UDP"`), `limits`/`requests` (sorted `"cpu: …, memory: …"`), `envCount`, `volumeMounts` (`[]any` of `{name, mountPath}`), `init` (true on init containers) |
| `NodeEnvRef` | `k8s-env-ref` | workloads, after their container | attrs `refKind` (`ConfigMap`/`Secret`), `refName`, `container` **(graph)** |
| `NodeServiceAccountRef` | `k8s-service-account-ref` | workloads | attrs `name` |
| `NodeVolume` | `k8s-volume` | workloads | attrs `name`, `volumeType`, `refName` (only for configMap/secret/persistentVolumeClaim sources) **(graph)** |
| `NodeServiceSpec` | `k8s-service-spec` | Services | attrs `serviceType` (default `ClusterIP`), `selector` (sorted `"app=api, tier=backend"`), `selectorMap` (the map, for matching) **(graph)** |
| `NodePortMapping` | `k8s-port-mapping` | Services | attrs `port`, `targetPort`, `protocol` (default `TCP`) |
| `NodeConfigData` | `k8s-config-data` | ConfigMaps, sorted by key | content = value; attrs `key` |
| `NodeSecretData` | `k8s-secret-data` | Secrets | attrs `keys` (`data` keys sorted, then `stringData` keys sorted); **values are never emitted** |
| `NodeIngressRule` | `k8s-ingress-rule` | Ingresses, per host/path | attrs `host`, `path`, `pathType`, `serviceName` **(graph)**, `servicePort` (number or name) |
| `NodeSpec` | `k8s-spec` | unrecognized kinds | content = the spec re-rendered as YAML |

## Tree shape

One page, N nodes — this is not a TreeParser; a manifest emits no child
pages. (Multi-document YAML streams are not split here — a scanner front-end
splits them into one file per resource before parsing.)

## Slug derivation

The parser never emits slugs, routes, or titles — the envelope `title` comes
from frontmatter when a scanner supplies one; otherwise the engine derives it.
The filesystem-mirror tier strips the final extension (`api.k8s` → `/api/`,
`api.k8s.yaml` → `/api.k8s/`).

## Stability

Pre-1.0: node types, attribute names, and value formats may change between
minor versions, and **each such change is a breaking release of this
module** — the graph-attribute contract above doubly so, since hook code
depends on it. The machine-checked form of this contract is the golden node
dump under `testdata/` (`*.k8s` input → `*.golden.json`), regenerated with
`go test ./kubernetes -update`. Out of scope for v1: multi-document streams,
CRD schema awareness, `envFrom` prefixes, projected-volume sub-sources, and
kinds beyond the list above (they fall back to the YAML spec node).
