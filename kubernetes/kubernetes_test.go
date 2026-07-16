package kubernetes_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
	"github.com/gofuego/fuego-formats/kubernetes"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

func TestParserClaimsDefaults(t *testing.T) {
	fp, ok := kubernetes.Parser().(core.FilenameParser)
	if !ok {
		t.Fatal("kubernetes.Parser() must implement core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, kubernetes.DefaultPatterns) {
		t.Errorf("Filenames() = %v, want %v", got, kubernetes.DefaultPatterns)
	}
	if fp.Type() != "k8s" {
		t.Errorf("Type() = %q, want k8s", fp.Type())
	}
}

func TestWithPatternsOverride(t *testing.T) {
	fp := kubernetes.Parser(formatkit.WithPatterns("*.manifest.yaml")).(core.FilenameParser)
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"*.manifest.yaml"}) {
		t.Errorf("Filenames() = %v, want [*.manifest.yaml]", got)
	}
}

func loadFixture(t *testing.T, name string) (core.Envelope, []core.Node) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	env, nodes, err := kubernetes.Parser().Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return env, nodes
}

func nodesByType(nodes []core.Node) map[string][]core.Node {
	byType := map[string][]core.Node{}
	for _, n := range nodes {
		byType[n.Type] = append(byType[n.Type], n)
	}
	return byType
}

// The graph attribute contract: the identifiers a relationship hook reads
// (resource identity, selector maps, env/volume refs, container images) are
// public API of this module.
func TestGraphAttributeContract(t *testing.T) {
	env, nodes := loadFixture(t, "deployment.k8s")
	byType := nodesByType(nodes)

	if env["title"] != "api-server Deployment" {
		t.Errorf("frontmatter title = %v", env["title"])
	}
	if _, ok := env["layout"]; ok {
		t.Errorf("the parser must not emit a layout key, got %v", env["layout"])
	}

	header := byType[kubernetes.NodeResourceHeader]
	if len(header) != 1 || header[0].Attributes["kind"] != "Deployment" ||
		header[0].Attributes["name"] != "api-server" || header[0].Attributes["namespace"] != "prod" {
		t.Errorf("resource header = %+v", header)
	}

	labels := byType[kubernetes.NodePodTemplateLabels]
	if len(labels) != 1 || labels[0].Attributes["app"] != "api" || labels[0].Attributes["tier"] != "backend" {
		t.Errorf("pod template labels = %+v", labels)
	}

	// Both containers, init flagged on the init container's spec node.
	specs := byType[kubernetes.NodeContainerSpec]
	if len(specs) != 2 {
		t.Fatalf("want 2 container-spec nodes, got %d", len(specs))
	}
	if specs[0].Attributes["image"] != "registry.example.com/api:1.4.2" || specs[0].Attributes["ports"] != "8080/TCP, 9090/UDP" {
		t.Errorf("api container = %+v", specs[0].Attributes)
	}
	// Sorted resource keys: cpu before memory.
	if specs[0].Attributes["limits"] != "cpu: 500m, memory: 512Mi" {
		t.Errorf("limits = %v", specs[0].Attributes["limits"])
	}
	if specs[1].Attributes["init"] != true {
		t.Errorf("init container not flagged = %+v", specs[1].Attributes)
	}

	// env valueFrom (Secret + ConfigMap) plus envFrom (ConfigMap) = 3 refs.
	refs := byType[kubernetes.NodeEnvRef]
	if len(refs) != 3 {
		t.Fatalf("want 3 env-ref nodes, got %+v", refs)
	}
	seen := map[string]string{}
	for _, r := range refs {
		seen[r.Attributes["refName"].(string)] = r.Attributes["refKind"].(string)
		if r.Attributes["container"] != "api" {
			t.Errorf("env-ref container = %v", r.Attributes["container"])
		}
	}
	if seen["db-credentials"] != "Secret" || seen["app-config"] != "ConfigMap" {
		t.Errorf("env refs = %v", seen)
	}

	sa := byType[kubernetes.NodeServiceAccountRef]
	if len(sa) != 1 || sa[0].Attributes["name"] != "api-sa" {
		t.Errorf("service-account-ref = %+v", sa)
	}

	vols := byType[kubernetes.NodeVolume]
	if len(vols) != 4 {
		t.Fatalf("want 4 volume nodes, got %d", len(vols))
	}
	wantVols := map[string][2]string{
		"config-vol": {"configMap", "app-config"},
		"tls-vol":    {"secret", "tls-cert"},
		"data-vol":   {"persistentVolumeClaim", "data-pvc"},
		"tmp":        {"emptyDir", ""},
	}
	for _, v := range vols {
		name := v.Attributes["name"].(string)
		want := wantVols[name]
		refName, _ := v.Attributes["refName"].(string)
		if v.Attributes["volumeType"] != want[0] || refName != want[1] {
			t.Errorf("volume %s = %+v, want %v", name, v.Attributes, want)
		}
	}
}

func TestServiceSelector(t *testing.T) {
	_, nodes := loadFixture(t, "service.k8s")
	byType := nodesByType(nodes)

	spec := byType[kubernetes.NodeServiceSpec]
	if len(spec) != 1 || spec[0].Attributes["serviceType"] != "LoadBalancer" {
		t.Fatalf("service-spec = %+v", spec)
	}
	// selector renders with sorted keys; selectorMap stays a map for
	// label matching.
	if spec[0].Attributes["selector"] != "app=api, tier=backend" {
		t.Errorf("selector = %v", spec[0].Attributes["selector"])
	}
	sm, ok := spec[0].Attributes["selectorMap"].(map[string]any)
	if !ok || sm["app"] != "api" {
		t.Errorf("selectorMap = %#v", spec[0].Attributes["selectorMap"])
	}
	if ports := byType[kubernetes.NodePortMapping]; len(ports) != 2 {
		t.Errorf("port-mapping nodes = %+v", ports)
	}
}

// ConfigMap data and Secret keys emit in sorted order — deterministic output
// is a repo convention (the old in-pack parser iterated maps unsorted).
func TestSortedDataAndKeys(t *testing.T) {
	_, nodes := loadFixture(t, "configmap.k8s")
	var keys []string
	for _, n := range nodesByType(nodes)[kubernetes.NodeConfigData] {
		keys = append(keys, n.Attributes["key"].(string))
	}
	if !reflect.DeepEqual(keys, []string{"api_url", "cache_ttl", "flags", "log_format"}) {
		t.Errorf("config-data keys = %v", keys)
	}

	_, nodes = loadFixture(t, "secret.k8s")
	secret := nodesByType(nodes)[kubernetes.NodeSecretData]
	if len(secret) != 1 {
		t.Fatalf("secret-data nodes = %+v", secret)
	}
	got, ok := secret[0].Attributes["keys"].([]string)
	if !ok || !reflect.DeepEqual(got, []string{"password", "username", "connection"}) {
		t.Errorf("secret keys = %#v (data sorted, then stringData sorted)", secret[0].Attributes["keys"])
	}
	if strings.Contains(secret[0].Content, "hunter") {
		t.Error("secret values must never be emitted")
	}
}

func TestMalformedYAMLErrors(t *testing.T) {
	_, _, err := kubernetes.Parser().Parse([]byte("kind: Service\n  bad:\nindent"))
	if err == nil || !strings.Contains(err.Error(), "k8s:") {
		t.Fatalf("want a k8s-attributed parse error, got %v", err)
	}
}

// dump is the golden node-dump: the parser's full output for a fixture input,
// simultaneously the regression test and the shipped contract example.
type dump struct {
	Envelope core.Envelope `json:"envelope"`
	Nodes    []core.Node   `json:"nodes"`
}

// TestGoldenDump covers one fixture per parsed kind plus the unknown-kind
// fallback. Regenerate with: go test ./kubernetes -update
func TestGoldenDump(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.k8s"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.k8s fixtures found")
	}

	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".k8s")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			env, nodes, err := kubernetes.Parser().Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(dump{Envelope: env, Nodes: nodes}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			golden := filepath.Join("testdata", name+".golden.json")
			if *update {
				if err := os.WriteFile(golden, got, 0644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading golden (run with -update): %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
		})
	}
}
