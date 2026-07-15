package mermaid_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
	"github.com/gofuego/fuego-formats/mermaid"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

func TestParserClaimsMMD(t *testing.T) {
	fp, ok := mermaid.Parser().(core.FilenameParser)
	if !ok {
		t.Fatal("mermaid.Parser() must implement core.FilenameParser")
	}
	got := fp.Filenames()
	if len(got) != 1 || got[0] != "*.mmd" {
		t.Errorf("Filenames() = %v, want [*.mmd]", got)
	}
	if fp.Type() != "mermaid" {
		t.Errorf("Type() = %q, want mermaid", fp.Type())
	}
}

func TestWithPatternsOverride(t *testing.T) {
	fp := mermaid.Parser(formatkit.WithPatterns("*.mermaid")).(core.FilenameParser)
	got := fp.Filenames()
	if len(got) != 1 || got[0] != "*.mermaid" {
		t.Errorf("Filenames() = %v, want [*.mermaid]", got)
	}
}

func TestEnvelopeLayoutAlwaysSet(t *testing.T) {
	env, _, err := mermaid.Parser().Parse([]byte("graph TD\n  A-->B\n"))
	if err != nil {
		t.Fatal(err)
	}
	if env["layout"] != mermaid.Layout {
		t.Errorf("layout = %v, want %q", env["layout"], mermaid.Layout)
	}
	if _, ok := env["title"]; ok {
		t.Errorf("title should be unset when no directive present, got %v", env["title"])
	}
}

func TestTitleFromFrontmatter(t *testing.T) {
	env, _, err := mermaid.Parser().Parse([]byte("---\ntitle: My Flow\n---\ngraph TD\n  A-->B\n"))
	if err != nil {
		t.Fatal(err)
	}
	if env["title"] != "My Flow" {
		t.Errorf("title = %v, want %q", env["title"], "My Flow")
	}
}

func TestTitleFromDirective(t *testing.T) {
	env, _, err := mermaid.Parser().Parse([]byte("pie showData\n  title Pets\n  \"Dogs\" : 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if env["title"] != "Pets" {
		t.Errorf("title = %v, want %q", env["title"], "Pets")
	}
}

func TestSingleRawNodeWrapsSource(t *testing.T) {
	_, nodes, err := mermaid.Parser().Parse([]byte("graph TD\n  A-->B\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 {
		t.Fatalf("want exactly one node, got %d", len(nodes))
	}
	n := nodes[0]
	if n.Type != mermaid.NodeDiagram {
		t.Errorf("node type = %q, want %q", n.Type, mermaid.NodeDiagram)
	}
	if !n.Raw {
		t.Error("node must be Raw so the <pre> passes through unescaped")
	}
	if !strings.Contains(n.Content, `<pre class="mermaid">`) {
		t.Errorf("content must wrap in <pre class=\"mermaid\">, got %q", n.Content)
	}
	if n.Attributes["source"] != "graph TD\n  A-->B" {
		t.Errorf("source attribute = %q, want trimmed diagram source", n.Attributes["source"])
	}
}

func TestContentEscapesDiagramText(t *testing.T) {
	_, nodes, err := mermaid.Parser().Parse([]byte("graph TD\n  A[\"x < y & z\"]-->B\n"))
	if err != nil {
		t.Fatal(err)
	}
	// The '<' and '&' in the diagram must be entity-escaped inside the <pre> so
	// the browser hands mermaid.js the literal text, not broken markup.
	if strings.Contains(nodes[0].Content, "x < y & z") {
		t.Error("diagram text must be HTML-escaped inside the <pre> block")
	}
	if !strings.Contains(nodes[0].Content, "x &lt; y &amp; z") {
		t.Errorf("expected escaped diagram text, got %q", nodes[0].Content)
	}
}

// dump is the golden node-dump: the parser's full output for a fixture input,
// serialized deterministically.
type dump struct {
	Envelope core.Envelope `json:"envelope"`
	Nodes    []core.Node   `json:"nodes"`
}

// TestGoldenDump is simultaneously the regression test and the shipped contract
// example. Regenerate with: go test ./mermaid -update
func TestGoldenDump(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.mmd"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.mmd fixtures found")
	}

	for _, in := range inputs {
		in := in
		name := strings.TrimSuffix(filepath.Base(in), ".mmd")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			env, nodes, err := mermaid.Parser().Parse(raw)
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
