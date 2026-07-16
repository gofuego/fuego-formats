// Package kubernetes is a fuego-formats parser for Kubernetes manifests.
//
// One manifest becomes one page of structured nodes: a resource header
// (kind, apiVersion, name, namespace), metadata labels/annotations, and
// kind-specific detail — containers, env references, volumes, service
// selectors and ports, config/secret data, ingress rules. Relationship
// hooks read those node attributes to build cross-resource graphs — the
// selector maps, ref kinds/names, and container images are public API; a
// consumer like fuego-devops never re-parses YAML. See schema.md.
//
// The parser accepts optional YAML frontmatter (a scanner front-end like
// fuego-devops emits title/source_path/resource_kind that way); a bare
// manifest without frontmatter parses the same.
//
// Usage:
//
//	eng := engine.New()
//	eng.Register(kubernetes.Parser())
//
// The default claims are the *.k8s extension (the scanner-emitted form) plus
// the *.k8s.yaml / *.k8s.yml compound suffixes for repos that name manifests
// directly. Claiming bare *.yaml would be far too greedy — override with
// formatkit.WithPatterns(...) if your layout needs it.
package kubernetes

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gofuego/fuego/core"
	"gopkg.in/yaml.v3"

	"github.com/gofuego/fuego-formats/formatkit"
)

// Type is the parser's Type() — the format slug and every page's type. It is
// deliberately the short "k8s" (matching the scanner-emitted extension and
// the k8s- node prefix), not the module name.
const Type = "k8s"

// Node types emitted by the parser. All are prefixed with the format slug so
// a theme's renderer templates never collide with another format's.
const (
	// NodeResourceHeader opens every page: attrs kind, apiVersion, name,
	// namespace. Relationship hooks use it as the resource's identity.
	NodeResourceHeader = "k8s-resource-header"
	// NodeMetadata is one labels or annotations block (content "Labels" or
	// "Annotations"; attributes are the key-value pairs verbatim).
	NodeMetadata = "k8s-metadata"
	// NodeReplicas is a workload's replica count (attrs: count).
	NodeReplicas = "k8s-replicas"
	// NodePodTemplateLabels carries a workload's pod template labels
	// verbatim — what Service selectors match against.
	NodePodTemplateLabels = "k8s-pod-template-labels"
	// NodeContainerSpec is one container (content = image; attrs name,
	// image, ports, limits, requests, envCount, volumeMounts, init).
	NodeContainerSpec = "k8s-container-spec"
	// NodeEnvRef is one env/envFrom reference to a ConfigMap or Secret
	// (attrs: refKind, refName, container).
	NodeEnvRef = "k8s-env-ref"
	// NodeServiceAccountRef is a pod spec's serviceAccountName (attrs: name).
	NodeServiceAccountRef = "k8s-service-account-ref"
	// NodeVolume is one pod volume (attrs: name, volumeType, refName when
	// the source references a named resource).
	NodeVolume = "k8s-volume"
	// NodeServiceSpec is a Service's spec summary (attrs: serviceType,
	// selector, selectorMap).
	NodeServiceSpec = "k8s-service-spec"
	// NodePortMapping is one Service port (attrs: port, targetPort, protocol).
	NodePortMapping = "k8s-port-mapping"
	// NodeConfigData is one ConfigMap data entry (content = value; attrs: key).
	NodeConfigData = "k8s-config-data"
	// NodeSecretData is a Secret's data summary — values are never emitted
	// (attrs: keys).
	NodeSecretData = "k8s-secret-data"
	// NodeIngressRule is one Ingress host/path rule (attrs: host, path,
	// pathType, serviceName, servicePort).
	NodeIngressRule = "k8s-ingress-rule"
	// NodeSpec is the fallback for unrecognized kinds: content is the spec
	// rendered back as YAML.
	NodeSpec = "k8s-spec"
)

// DefaultPatterns are the built-in filename claims: the scanner-emitted *.k8s
// extension plus the compound suffixes for directly-named manifests. Claims
// match base names only.
var DefaultPatterns = []string{"*.k8s", "*.k8s.yaml", "*.k8s.yml"}

// Option re-exports formatkit.Option so callers configure the parser without
// importing formatkit directly for the common case.
type Option = formatkit.Option

// Parser returns a Fuego parser claiming the DefaultPatterns. Pass
// formatkit.WithPatterns(...) to override the claims.
func Parser(opts ...Option) core.Parser {
	all := append([]Option{formatkit.WithDefaultPatterns(DefaultPatterns...)}, opts...)
	return formatkit.NewParser(Type, parse, all...)
}

// parse splits optional YAML frontmatter off the manifest, then emits the
// structured nodes for its kind. Like the docker module, it deliberately
// emits no layout key, so a consuming pack's layout semantics are untouched.
// A manifest that fails to unmarshal is a parse error — the engine records a
// LocalFatal for the file and the rest of the site builds.
func parse(raw []byte) (core.Envelope, []core.Node, error) {
	env, payload, err := core.SplitFrontmatter(raw)
	if err != nil {
		return nil, nil, err
	}
	if env == nil {
		env = make(core.Envelope)
	}
	nodes, err := parseManifest(payload)
	if err != nil {
		return nil, nil, err
	}
	return env, nodes, nil
}

func parseManifest(payload []byte) ([]core.Node, error) {
	var manifest map[string]any
	if err := yaml.Unmarshal(payload, &manifest); err != nil {
		return nil, fmt.Errorf("k8s: parsing manifest YAML: %w", err)
	}
	if manifest == nil {
		return nil, nil
	}

	kind, _ := manifest["kind"].(string)
	apiVersion, _ := manifest["apiVersion"].(string)

	var nodes []core.Node

	name, namespace := extractMetadata(manifest)
	nodes = append(nodes, core.Node{
		Type: NodeResourceHeader,
		Attributes: map[string]any{
			"kind":       kind,
			"apiVersion": apiVersion,
			"name":       name,
			"namespace":  namespace,
		},
	})

	if md, ok := manifest["metadata"].(map[string]any); ok {
		if labels, ok := md["labels"].(map[string]any); ok && len(labels) > 0 {
			nodes = append(nodes, core.Node{
				Type:       NodeMetadata,
				Content:    "Labels",
				Attributes: labels,
			})
		}
		if annotations, ok := md["annotations"].(map[string]any); ok && len(annotations) > 0 {
			nodes = append(nodes, core.Node{
				Type:       NodeMetadata,
				Content:    "Annotations",
				Attributes: annotations,
			})
		}
	}

	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob":
		nodes = append(nodes, parseWorkload(manifest)...)
	case "Service":
		nodes = append(nodes, parseService(manifest)...)
	case "ConfigMap":
		nodes = append(nodes, parseConfigMap(manifest)...)
	case "Secret":
		nodes = append(nodes, core.Node{
			Type:    NodeSecretData,
			Content: "Secret data keys are hidden",
			Attributes: map[string]any{
				"keys": extractSecretKeys(manifest),
			},
		})
	case "Ingress":
		nodes = append(nodes, parseIngress(manifest)...)
	default:
		if spec, ok := manifest["spec"]; ok {
			specYAML, _ := yaml.Marshal(spec)
			nodes = append(nodes, core.Node{
				Type:    NodeSpec,
				Content: string(specYAML),
			})
		}
	}

	return nodes, nil
}

func extractMetadata(manifest map[string]any) (string, string) {
	md, _ := manifest["metadata"].(map[string]any)
	if md == nil {
		return "", ""
	}
	name, _ := md["name"].(string)
	namespace, _ := md["namespace"].(string)
	return name, namespace
}

func parseWorkload(manifest map[string]any) []core.Node {
	var nodes []core.Node

	spec := dig(manifest, "spec")
	if spec == nil {
		return nodes
	}

	if replicas, ok := spec["replicas"]; ok {
		nodes = append(nodes, core.Node{
			Type:       NodeReplicas,
			Content:    fmt.Sprintf("%v", replicas),
			Attributes: map[string]any{"count": replicas},
		})
	}

	// Pod template labels (used for Service selector matching)
	templateLabels := dig(spec, "template", "metadata")
	if templateLabels != nil {
		if labels, ok := templateLabels["labels"].(map[string]any); ok && len(labels) > 0 {
			nodes = append(nodes, core.Node{
				Type:       NodePodTemplateLabels,
				Attributes: labels,
			})
		}
	}

	podSpec := dig(spec, "template", "spec")
	if podSpec == nil {
		return nodes
	}

	containers, _ := podSpec["containers"].([]any)
	for _, c := range containers {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		nodes = append(nodes, parseContainer(cm)...)
	}

	initContainers, _ := podSpec["initContainers"].([]any)
	for _, c := range initContainers {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		containerNodes := parseContainer(cm)
		if len(containerNodes) > 0 {
			containerNodes[0].Attributes["init"] = true
		}
		nodes = append(nodes, containerNodes...)
	}

	if sa, ok := podSpec["serviceAccountName"].(string); ok && sa != "" {
		nodes = append(nodes, core.Node{
			Type:       NodeServiceAccountRef,
			Attributes: map[string]any{"name": sa},
		})
	}

	volumes, _ := podSpec["volumes"].([]any)
	for _, v := range volumes {
		vm, ok := v.(map[string]any)
		if !ok {
			continue
		}
		name, _ := vm["name"].(string)
		volType, refName := identifyVolumeSource(vm)
		attrs := map[string]any{
			"name":       name,
			"volumeType": volType,
		}
		if refName != "" {
			attrs["refName"] = refName
		}
		nodes = append(nodes, core.Node{
			Type:       NodeVolume,
			Attributes: attrs,
		})
	}

	return nodes
}

func parseContainer(cm map[string]any) []core.Node {
	name, _ := cm["name"].(string)
	image, _ := cm["image"].(string)

	attrs := map[string]any{
		"name":  name,
		"image": image,
	}

	var extraNodes []core.Node

	if ports, ok := cm["ports"].([]any); ok {
		var portStrs []string
		for _, p := range ports {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			cp := pm["containerPort"]
			proto := pm["protocol"]
			if proto == nil {
				proto = "TCP"
			}
			portStrs = append(portStrs, fmt.Sprintf("%v/%v", cp, proto))
		}
		attrs["ports"] = strings.Join(portStrs, ", ")
	}

	if resources, ok := cm["resources"].(map[string]any); ok {
		if limits, ok := resources["limits"].(map[string]any); ok {
			attrs["limits"] = formatResources(limits)
		}
		if requests, ok := resources["requests"].(map[string]any); ok {
			attrs["requests"] = formatResources(requests)
		}
	}

	// Env vars: count + valueFrom references
	if env, ok := cm["env"].([]any); ok {
		attrs["envCount"] = len(env)
		for _, e := range env {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			vf, ok := em["valueFrom"].(map[string]any)
			if !ok {
				continue
			}
			if cmkr, ok := vf["configMapKeyRef"].(map[string]any); ok {
				extraNodes = appendEnvRef(extraNodes, "ConfigMap", cmkr, "name", name)
			}
			if skr, ok := vf["secretKeyRef"].(map[string]any); ok {
				extraNodes = appendEnvRef(extraNodes, "Secret", skr, "name", name)
			}
		}
	}

	// envFrom references
	if envFrom, ok := cm["envFrom"].([]any); ok {
		for _, ef := range envFrom {
			efm, ok := ef.(map[string]any)
			if !ok {
				continue
			}
			if cmRef, ok := efm["configMapRef"].(map[string]any); ok {
				extraNodes = appendEnvRef(extraNodes, "ConfigMap", cmRef, "name", name)
			}
			if secRef, ok := efm["secretRef"].(map[string]any); ok {
				extraNodes = appendEnvRef(extraNodes, "Secret", secRef, "name", name)
			}
		}
	}

	if mounts, ok := cm["volumeMounts"].([]any); ok {
		var mountList []any
		for _, m := range mounts {
			mm, ok := m.(map[string]any)
			if !ok {
				continue
			}
			mountName, _ := mm["name"].(string)
			mountPath, _ := mm["mountPath"].(string)
			mountList = append(mountList, map[string]any{"name": mountName, "mountPath": mountPath})
		}
		if len(mountList) > 0 {
			attrs["volumeMounts"] = mountList
		}
	}

	nodes := []core.Node{{
		Type:       NodeContainerSpec,
		Content:    image,
		Attributes: attrs,
	}}
	return append(nodes, extraNodes...)
}

// appendEnvRef emits one NodeEnvRef when the reference map names a resource.
func appendEnvRef(nodes []core.Node, refKind string, ref map[string]any, key, container string) []core.Node {
	refName, _ := ref[key].(string)
	if refName == "" {
		return nodes
	}
	return append(nodes, core.Node{
		Type: NodeEnvRef,
		Attributes: map[string]any{
			"refKind":   refKind,
			"refName":   refName,
			"container": container,
		},
	})
}

func parseService(manifest map[string]any) []core.Node {
	spec := dig(manifest, "spec")
	if spec == nil {
		return nil
	}

	svcType, _ := spec["type"].(string)
	if svcType == "" {
		svcType = "ClusterIP"
	}

	attrs := map[string]any{
		"serviceType": svcType,
	}

	if selector, ok := spec["selector"].(map[string]any); ok {
		parts := make([]string, 0, len(selector))
		for _, k := range sortedKeys(selector) {
			parts = append(parts, fmt.Sprintf("%s=%v", k, selector[k]))
		}
		attrs["selector"] = strings.Join(parts, ", ")
		attrs["selectorMap"] = selector
	}

	var nodes []core.Node
	nodes = append(nodes, core.Node{
		Type:       NodeServiceSpec,
		Attributes: attrs,
	})

	if ports, ok := spec["ports"].([]any); ok {
		for _, p := range ports {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			proto := pm["protocol"]
			if proto == nil {
				proto = "TCP"
			}
			nodes = append(nodes, core.Node{
				Type: NodePortMapping,
				Attributes: map[string]any{
					"port":       pm["port"],
					"targetPort": pm["targetPort"],
					"protocol":   proto,
				},
			})
		}
	}

	return nodes
}

func parseConfigMap(manifest map[string]any) []core.Node {
	var nodes []core.Node
	if data, ok := manifest["data"].(map[string]any); ok {
		for _, k := range sortedKeys(data) {
			nodes = append(nodes, core.Node{
				Type:    NodeConfigData,
				Content: fmt.Sprintf("%v", data[k]),
				Attributes: map[string]any{
					"key": k,
				},
			})
		}
	}
	return nodes
}

func parseIngress(manifest map[string]any) []core.Node {
	var nodes []core.Node
	spec := dig(manifest, "spec")
	if spec == nil {
		return nodes
	}

	rules, _ := spec["rules"].([]any)
	for _, r := range rules {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		host, _ := rm["host"].(string)
		http, _ := rm["http"].(map[string]any)
		paths, _ := http["paths"].([]any)

		for _, p := range paths {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			path, _ := pm["path"].(string)
			pathType, _ := pm["pathType"].(string)

			backend := dig(pm, "backend", "service")
			var svcName string
			var svcPort any
			if backend != nil {
				svcName, _ = backend["name"].(string)
				port := dig(backend, "port")
				if port != nil {
					if n, ok := port["number"]; ok {
						svcPort = n
					} else if n, ok := port["name"]; ok {
						svcPort = n
					}
				}
			}

			nodes = append(nodes, core.Node{
				Type: NodeIngressRule,
				Attributes: map[string]any{
					"host":        host,
					"path":        path,
					"pathType":    pathType,
					"serviceName": svcName,
					"servicePort": svcPort,
				},
			})
		}
	}

	return nodes
}

func extractSecretKeys(manifest map[string]any) []string {
	var keys []string
	if data, ok := manifest["data"].(map[string]any); ok {
		keys = append(keys, sortedKeys(data)...)
	}
	if data, ok := manifest["stringData"].(map[string]any); ok {
		keys = append(keys, sortedKeys(data)...)
	}
	return keys
}

func formatResources(r map[string]any) string {
	parts := make([]string, 0, len(r))
	for _, k := range sortedKeys(r) {
		parts = append(parts, fmt.Sprintf("%s: %v", k, r[k]))
	}
	return strings.Join(parts, ", ")
}

// identifyVolumeSource returns the volume type and the name of the referenced
// resource (if any).
func identifyVolumeSource(vm map[string]any) (string, string) {
	// Types that reference a named resource
	if cm, ok := vm["configMap"].(map[string]any); ok {
		refName, _ := cm["name"].(string)
		return "configMap", refName
	}
	if sec, ok := vm["secret"].(map[string]any); ok {
		refName, _ := sec["secretName"].(string)
		return "secret", refName
	}
	if pvc, ok := vm["persistentVolumeClaim"].(map[string]any); ok {
		refName, _ := pvc["claimName"].(string)
		return "persistentVolumeClaim", refName
	}

	// Types without a named reference
	otherTypes := []string{
		"emptyDir", "hostPath", "nfs", "awsElasticBlockStore",
		"gcePersistentDisk", "azureDisk", "projected", "downwardAPI",
	}
	for _, t := range otherTypes {
		if _, ok := vm[t]; ok {
			return t, ""
		}
	}
	return "unknown", ""
}

// sortedKeys returns a map's keys in sorted order — every map iteration that
// reaches the output goes through it, so builds are deterministic.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// dig navigates nested maps by key path.
func dig(m map[string]any, keys ...string) map[string]any {
	current := m
	for _, k := range keys {
		next, ok := current[k].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}
