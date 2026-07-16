package openapi_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
	"github.com/gofuego/fuego-formats/openapi"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

func TestParserClaimsDefaults(t *testing.T) {
	fp, ok := openapi.Parser().(core.FilenameParser)
	if !ok {
		t.Fatal("openapi.Parser() must implement core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, openapi.DefaultPatterns) {
		t.Errorf("Filenames() = %v, want %v", got, openapi.DefaultPatterns)
	}
	if fp.Type() != "openapi" {
		t.Errorf("Type() = %q, want openapi", fp.Type())
	}
}

func TestParserIsTreeParser(t *testing.T) {
	if _, ok := openapi.Parser().(core.TreeParser); !ok {
		t.Fatal("openapi.Parser() must implement core.TreeParser")
	}
}

func TestWithPatternsOverride(t *testing.T) {
	fp := openapi.Parser(formatkit.WithPatterns("*.api.yaml")).(core.FilenameParser)
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"*.api.yaml"}) {
		t.Errorf("Filenames() = %v, want [*.api.yaml]", got)
	}
}

func loadFixtureTree(t *testing.T, name string) *core.PageTree {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	tree, err := openapi.Parser().(core.TreeParser).ParseTree(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// The slug rules are a stability promise: operationId when present (camelCase
// → hyphenated), else method + path (braces stripped, params hyphenated).
func TestTreeShapeAndSlugs(t *testing.T) {
	tree := loadFixtureTree(t, "billing.openapi.yaml")

	var rootKeys []string
	for k := range tree.Children {
		rootKeys = append(rootKeys, k)
	}
	sort.Strings(rootKeys)
	want := []string{
		"operations/get-healthz",
		"schemas/invoice",
		"schemas/line-item",
		"schemas/new-invoice",
		"tags/invoices",
		"tags/payments",
	}
	if !reflect.DeepEqual(rootKeys, want) {
		t.Fatalf("root children = %v, want %v", rootKeys, want)
	}

	invoices := tree.Children["tags/invoices"]
	var opKeys []string
	for k := range invoices.Children {
		opKeys = append(opKeys, k)
	}
	sort.Strings(opKeys)
	wantOps := []string{
		"create-invoice",                // operationId createInvoice
		"list-invoices",                 // operationId listInvoices
		"post-invoices-invoice-id-void", // no operationId: method+path
		"refund-payment",                // multi-tag op appears here too
	}
	if !reflect.DeepEqual(opKeys, wantOps) {
		t.Fatalf("invoices tag children = %v, want %v", opKeys, wantOps)
	}

	// The multi-tag operation also lives under its other tag.
	if _, ok := tree.Children["tags/payments"].Children["refund-payment"]; !ok {
		t.Error("refund-payment missing under tags/payments")
	}
}

func TestEnvelopes(t *testing.T) {
	tree := loadFixtureTree(t, "billing.openapi.yaml")

	if tree.Envelope["title"] != "Billing API" || tree.Envelope["layout"] != openapi.Layout {
		t.Errorf("root envelope = %v", tree.Envelope)
	}
	if tree.Envelope["version"] != "2.1.0" {
		t.Errorf("root version = %v", tree.Envelope["version"])
	}
	if tree.Envelope["summary"] != "Invoicing and payments for the Acme platform." {
		t.Errorf("root summary = %v", tree.Envelope["summary"])
	}

	op := tree.Children["tags/invoices"].Children["list-invoices"]
	env := op.Envelope
	if env["title"] != "List invoices" || env["layout"] != openapi.LayoutOperation {
		t.Errorf("operation envelope = %v", env)
	}
	if env["method"] != "GET" || env["path"] != "/invoices" || env["operation_id"] != "listInvoices" {
		t.Errorf("operation envelope facts = %v", env)
	}
	// tags is []any — taxonomy-scannable and cache-eligible.
	if tags, ok := env["tags"].([]any); !ok || len(tags) != 1 || tags[0] != "Invoices" {
		t.Errorf("operation tags = %#v, want []any{Invoices}", env["tags"])
	}

	dep := tree.Children["tags/payments"].Children["refund-payment"]
	if dep.Envelope["deprecated"] != true {
		t.Errorf("deprecated op envelope = %v", dep.Envelope)
	}

	schema := tree.Children["schemas/invoice"]
	if schema.Envelope["title"] != "Invoice" || schema.Envelope["layout"] != openapi.LayoutSchema || schema.Envelope["type"] != "object" {
		t.Errorf("schema envelope = %v", schema.Envelope)
	}
}

func TestOperationNodes(t *testing.T) {
	tree := loadFixtureTree(t, "billing.openapi.yaml")
	op := tree.Children["tags/invoices"].Children["create-invoice"]

	byType := map[string][]core.Node{}
	for _, n := range op.Nodes {
		byType[n.Type] = append(byType[n.Type], n)
	}
	if len(byType[openapi.NodeOperation]) != 1 {
		t.Fatalf("want one %s node, got %v", openapi.NodeOperation, byType)
	}
	rb := byType[openapi.NodeRequestBody]
	if len(rb) != 1 || rb[0].Attributes["required"] != true {
		t.Errorf("request body node = %+v", rb)
	}
	if cts, ok := rb[0].Attributes["content_types"].([]any); !ok || len(cts) != 1 || cts[0] != "application/json" {
		t.Errorf("request body content types = %#v", rb[0].Attributes["content_types"])
	}
	resp := byType[openapi.NodeResponse]
	if len(resp) != 2 || resp[0].Attributes["status"] != "201" || resp[1].Attributes["status"] != "422" {
		t.Errorf("responses = %+v", resp)
	}
}

func TestSchemaNodes(t *testing.T) {
	tree := loadFixtureTree(t, "billing.openapi.yaml")
	page := tree.Children["schemas/invoice"]

	var props []core.Node
	for _, n := range page.Nodes {
		if n.Type == openapi.NodeProperty {
			props = append(props, n)
		}
	}
	if len(props) != 4 {
		t.Fatalf("want 4 property nodes, got %d", len(props))
	}
	// Sorted by name: id, issued_at, lines, total.
	if props[0].Attributes["name"] != "id" || props[0].Attributes["type"] != "string (uuid)" || props[0].Attributes["required"] != true {
		t.Errorf("id property = %+v", props[0].Attributes)
	}
	if props[2].Attributes["name"] != "lines" || props[2].Attributes["type"] != "array of LineItem" {
		t.Errorf("lines property = %+v", props[2].Attributes)
	}
	if props[3].Attributes["name"] != "total" || props[3].Attributes["required"] != true {
		t.Errorf("total property = %+v", props[3].Attributes)
	}
}

func TestDuplicateOperationSlugFails(t *testing.T) {
	spec := `
openapi: 3.0.3
info: {title: Dup, version: "1"}
paths:
  /a:
    get:
      operationId: sameSlug
      tags: [T]
      responses: {"200": {description: ok}}
  /b:
    get:
      operationId: sameSlug
      tags: [T]
      responses: {"200": {description: ok}}
`
	_, err := openapi.Parser().(core.TreeParser).ParseTree([]byte(spec))
	if err == nil || !strings.Contains(err.Error(), "duplicate operation slug") {
		t.Fatalf("want duplicate-slug error, got %v", err)
	}
}

func TestMalformedSpecErrors(t *testing.T) {
	_, err := openapi.Parser().(core.TreeParser).ParseTree([]byte("{not: [valid"))
	if err == nil {
		t.Fatal("want error for malformed spec")
	}
}

// dump mirrors core.PageTree with JSON tags and sorted children, so the golden
// file is both the regression fixture and the shipped contract example.
type dump struct {
	Envelope core.Envelope   `json:"envelope"`
	Nodes    []core.Node     `json:"nodes,omitempty"`
	Children map[string]dump `json:"children,omitempty"`
}

func toDump(t *core.PageTree) dump {
	d := dump{Envelope: t.Envelope, Nodes: t.Nodes}
	if len(t.Children) > 0 {
		d.Children = map[string]dump{}
		for k, c := range t.Children {
			d.Children[k] = toDump(c)
		}
	}
	return d
}

// TestGoldenDump is simultaneously the regression test and the shipped
// contract example. Regenerate with: go test ./openapi -update
func TestGoldenDump(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.openapi.yaml fixtures found")
	}

	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".openapi.yaml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := openapi.Parser().(core.TreeParser).ParseTree(raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(toDump(tree), "", "  ")
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
