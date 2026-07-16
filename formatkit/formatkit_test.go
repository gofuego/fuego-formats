package formatkit_test

import (
	"reflect"
	"testing"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
)

func stubParse(raw []byte) (core.Envelope, []core.Node, error) {
	return core.Envelope{"seen": string(raw)}, []core.Node{{Type: "x"}}, nil
}

func TestNewParserReportsDefaultPatterns(t *testing.T) {
	p := formatkit.NewParser("mermaid", stubParse, formatkit.WithDefaultPatterns("*.mmd"))

	if p.Type() != "mermaid" {
		t.Errorf("Type() = %q, want mermaid", p.Type())
	}

	fp, ok := p.(core.FilenameParser)
	if !ok {
		t.Fatal("NewParser result must implement core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"*.mmd"}) {
		t.Errorf("Filenames() = %v, want [*.mmd]", got)
	}
}

func TestWithPatternsOverridesDefaults(t *testing.T) {
	p := formatkit.NewParser("mermaid", stubParse,
		formatkit.WithDefaultPatterns("*.mmd"),
		formatkit.WithPatterns("*.mermaid", "*.diagram.mmd"),
	)
	fp := p.(core.FilenameParser)
	want := []string{"*.mermaid", "*.diagram.mmd"}
	if got := fp.Filenames(); !reflect.DeepEqual(got, want) {
		t.Errorf("Filenames() = %v, want %v", got, want)
	}
}

func TestFilenamesReturnsCopy(t *testing.T) {
	p := formatkit.NewParser("mermaid", stubParse, formatkit.WithDefaultPatterns("*.mmd"))
	fp := p.(core.FilenameParser)
	got := fp.Filenames()
	got[0] = "mutated"
	if fp.Filenames()[0] != "*.mmd" {
		t.Error("Filenames() must return a copy; caller mutation leaked into the parser")
	}
}

func TestParseDelegates(t *testing.T) {
	p := formatkit.NewParser("mermaid", stubParse, formatkit.WithDefaultPatterns("*.mmd"))
	env, nodes, err := p.Parse([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if env["seen"] != "hello" {
		t.Errorf("parse not delegated: env = %v", env)
	}
	if len(nodes) != 1 || nodes[0].Type != "x" {
		t.Errorf("nodes not delegated: %v", nodes)
	}
}

func stubParseTree(raw []byte) (*core.PageTree, error) {
	return &core.PageTree{
		Envelope: core.Envelope{"seen": string(raw)},
		Nodes:    []core.Node{{Type: "root-node"}},
		Children: map[string]*core.PageTree{
			"child": {Envelope: core.Envelope{"title": "Child"}},
		},
	}, nil
}

func TestNewTreeParserImplementsInterfaces(t *testing.T) {
	p := formatkit.NewTreeParser("openapi", stubParseTree, formatkit.WithDefaultPatterns("*.openapi.yaml"))
	tp, ok := p.(core.TreeParser)
	if !ok {
		t.Fatal("NewTreeParser must return a core.TreeParser")
	}
	fp, ok := p.(core.FilenameParser)
	if !ok {
		t.Fatal("NewTreeParser must return a core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"*.openapi.yaml"}) {
		t.Errorf("Filenames() = %v, want [*.openapi.yaml]", got)
	}
	tree, err := tp.ParseTree([]byte("spec"))
	if err != nil {
		t.Fatal(err)
	}
	if tree.Envelope["seen"] != "spec" || len(tree.Children) != 1 {
		t.Errorf("ParseTree not delegated: %+v", tree)
	}
}

func TestTreeParserParseFallsBackToRoot(t *testing.T) {
	p := formatkit.NewTreeParser("openapi", stubParseTree)
	env, nodes, err := p.Parse([]byte("spec"))
	if err != nil {
		t.Fatal(err)
	}
	if env["seen"] != "spec" {
		t.Errorf("Parse fallback env = %v, want the tree root's envelope", env)
	}
	if len(nodes) != 1 || nodes[0].Type != "root-node" {
		t.Errorf("Parse fallback nodes = %v, want the tree root's nodes", nodes)
	}
}
