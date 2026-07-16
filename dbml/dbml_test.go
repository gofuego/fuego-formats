package dbml_test

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

	"github.com/gofuego/fuego-formats/dbml"
	"github.com/gofuego/fuego-formats/formatkit"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

func TestParserClaimsDefaults(t *testing.T) {
	fp, ok := dbml.Parser().(core.FilenameParser)
	if !ok {
		t.Fatal("dbml.Parser() must implement core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, dbml.DefaultPatterns) {
		t.Errorf("Filenames() = %v, want %v", got, dbml.DefaultPatterns)
	}
	if fp.Type() != "dbml" {
		t.Errorf("Type() = %q, want dbml", fp.Type())
	}
}

func TestParserIsTreeParser(t *testing.T) {
	if _, ok := dbml.Parser().(core.TreeParser); !ok {
		t.Fatal("dbml.Parser() must implement core.TreeParser")
	}
}

func TestWithPatternsOverride(t *testing.T) {
	fp := dbml.Parser(formatkit.WithPatterns("*.database")).(core.FilenameParser)
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"*.database"}) {
		t.Errorf("Filenames() = %v, want [*.database]", got)
	}
}

func loadFixtureTree(t *testing.T, name string) *core.PageTree {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	tree, err := dbml.Parser().(core.TreeParser).ParseTree(raw)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// The slug rules are a stability promise: tables/ plus the slugified table
// name (underscores become hyphens).
func TestTreeShapeAndSlugs(t *testing.T) {
	tree := loadFixtureTree(t, "inventory.dbml")

	var rootKeys []string
	for k := range tree.Children {
		rootKeys = append(rootKeys, k)
	}
	sort.Strings(rootKeys)
	want := []string{
		"tables/audits",
		"tables/stock-items",
		"tables/warehouses",
	}
	if !reflect.DeepEqual(rootKeys, want) {
		t.Fatalf("root children = %v, want %v", rootKeys, want)
	}
	for _, k := range want {
		if len(tree.Children[k].Children) != 0 {
			t.Errorf("table page %s must be a leaf", k)
		}
	}
}

func TestEnvelopes(t *testing.T) {
	tree := loadFixtureTree(t, "inventory.dbml")

	if tree.Envelope["title"] != "inventory" || tree.Envelope["layout"] != dbml.Layout {
		t.Errorf("root envelope = %v", tree.Envelope)
	}
	if tree.Envelope["database_type"] != "PostgreSQL" {
		t.Errorf("root database_type = %v", tree.Envelope["database_type"])
	}
	if tree.Envelope["summary"] != "Warehouse inventory tracking." {
		t.Errorf("root summary = %v", tree.Envelope["summary"])
	}

	page := tree.Children["tables/warehouses"]
	if page.Envelope["title"] != "warehouses" || page.Envelope["layout"] != dbml.LayoutTable {
		t.Errorf("table envelope = %v", page.Envelope)
	}
}

func nodesByType(page *core.PageTree) map[string][]core.Node {
	byType := map[string][]core.Node{}
	for _, n := range page.Nodes {
		byType[n.Type] = append(byType[n.Type], n)
	}
	return byType
}

func TestRootNodes(t *testing.T) {
	tree := loadFixtureTree(t, "inventory.dbml")
	byType := nodesByType(tree)

	proj := byType[dbml.NodeProject]
	if len(proj) != 1 || proj[0].Attributes["name"] != "inventory" || proj[0].Attributes["database_type"] != "PostgreSQL" {
		t.Errorf("project node = %+v", proj)
	}
	if !strings.Contains(proj[0].Content, "second line") {
		t.Errorf("project note content = %q", proj[0].Content)
	}

	refs := byType[dbml.NodeTableRef]
	if len(refs) != 3 {
		t.Fatalf("want 3 table-ref nodes, got %d", len(refs))
	}
	// Declaration order, with alias and slug carried.
	if refs[0].Attributes["name"] != "warehouses" || refs[0].Attributes["slug"] != "tables/warehouses" {
		t.Errorf("first table-ref = %+v", refs[0].Attributes)
	}
	if refs[0].Attributes["summary"] != "One row per physical warehouse." {
		t.Errorf("table-ref summary = %+v", refs[0].Attributes)
	}
	if refs[1].Attributes["name"] != "stock_items" || refs[1].Attributes["alias"] != "SI" || refs[1].Attributes["slug"] != "tables/stock-items" {
		t.Errorf("aliased table-ref = %+v", refs[1].Attributes)
	}

	enums := byType[dbml.NodeEnum]
	if len(enums) != 2 || enums[0].Attributes["name"] != "item_status" {
		t.Fatalf("enum nodes = %+v", enums)
	}
	// values is []any — JSON-shaped; quoted values and per-value settings
	// reduce to the value names.
	wantValues := []any{"in_stock", "reserved", "written off"}
	if got, ok := enums[0].Attributes["values"].([]any); !ok || !reflect.DeepEqual(got, wantValues) {
		t.Errorf("enum values = %#v, want %v", enums[0].Attributes["values"], wantValues)
	}

	groups := byType[dbml.NodeTableGroup]
	if len(groups) != 1 || groups[0].Attributes["name"] != "logistics" {
		t.Fatalf("table-group nodes = %+v", groups)
	}
	if got, ok := groups[0].Attributes["tables"].([]any); !ok || !reflect.DeepEqual(got, []any{"warehouses", "stock_items"}) {
		t.Errorf("table-group tables = %#v", groups[0].Attributes["tables"])
	}
}

func TestColumnAndIndexNodes(t *testing.T) {
	tree := loadFixtureTree(t, "inventory.dbml")
	byType := nodesByType(tree.Children["tables/warehouses"])

	table := byType[dbml.NodeTable]
	if len(table) != 1 || table[0].Attributes["name"] != "warehouses" || table[0].Content != "One row per physical warehouse." {
		t.Errorf("table node = %+v", table)
	}

	cols := byType[dbml.NodeColumn]
	if len(cols) != 4 {
		t.Fatalf("want 4 column nodes, got %d", len(cols))
	}
	id := cols[0].Attributes
	if id["name"] != "id" || id["type"] != "integer" || id["pk"] != true || id["increment"] != true || id["not_null"] != false {
		t.Errorf("id column = %+v", id)
	}
	code := cols[1]
	if code.Attributes["unique"] != true || code.Attributes["not_null"] != true || code.Content != "Short human code" {
		t.Errorf("code column = %+v", code)
	}
	if cols[3].Attributes["default"] != "now()" {
		t.Errorf("opened_at default = %+v", cols[3].Attributes)
	}

	idxs := byType[dbml.NodeIndex]
	if len(idxs) != 2 {
		t.Fatalf("want 2 index nodes, got %d", len(idxs))
	}
	first := idxs[0].Attributes
	if got, ok := first["columns"].([]any); !ok || !reflect.DeepEqual(got, []any{"region", "code"}) {
		t.Errorf("composite index columns = %#v", first["columns"])
	}
	if first["unique"] != true || first["name"] != "idx_region_code" {
		t.Errorf("composite index = %+v", first)
	}
	if idxs[1].Attributes["type"] != "btree" {
		t.Errorf("btree index = %+v", idxs[1].Attributes)
	}
}

// Refs land on both endpoint tables' pages, with aliases resolved to table
// names — the stable identifiers cross-artifact linking reads.
func TestRefNodes(t *testing.T) {
	tree := loadFixtureTree(t, "inventory.dbml")

	stock := nodesByType(tree.Children["tables/stock-items"])[dbml.NodeRef]
	if len(stock) != 2 {
		t.Fatalf("stock_items ref nodes = %+v", stock)
	}
	inline := stock[0].Attributes
	if inline["from_table"] != "stock_items" || inline["from_column"] != "warehouse_id" ||
		inline["to_table"] != "warehouses" || inline["to_column"] != "id" || inline["relation"] != "many-to-one" {
		t.Errorf("inline ref = %+v", inline)
	}
	named := stock[1].Attributes
	if named["name"] != "audit_item" || named["to_table"] != "stock_items" {
		t.Errorf("alias not resolved in named ref = %+v", named)
	}
	if named["on_delete"] != "cascade" || named["on_update"] != "no action" {
		t.Errorf("ref settings = %+v", named)
	}

	audits := nodesByType(tree.Children["tables/audits"])[dbml.NodeRef]
	if len(audits) != 1 || audits[0].Attributes["from_table"] != "audits" {
		t.Errorf("audits ref nodes = %+v", audits)
	}
	warehouses := nodesByType(tree.Children["tables/warehouses"])[dbml.NodeRef]
	if len(warehouses) != 1 || warehouses[0].Attributes["to_table"] != "warehouses" {
		t.Errorf("warehouses ref nodes = %+v", warehouses)
	}
}

func TestDuplicateTableSlugFails(t *testing.T) {
	src := `
Table user_profiles {
  id integer [pk]
}
Table "user profiles" {
  id integer [pk]
}
`
	_, err := dbml.Parser().(core.TreeParser).ParseTree([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "duplicate table slug") {
		t.Fatalf("want duplicate-slug error, got %v", err)
	}
}

// Malformed input is a parse error (the engine records a LocalFatal for the
// file), never a partial tree.
func TestMalformedInputErrors(t *testing.T) {
	cases := map[string]string{
		"unknown top-level": "Tabel users {\n  id integer\n}\n",
		"missing name":      "Table {\n  id integer\n}\n",
		"unclosed table":    "Table users {\n  id integer\n",
		"missing type":      "Table users {\n  id\n}\n",
		"unclosed note":     "Table users {\n  Note: '''\n  never closed\n}\n",
		"nested block":      "Table users {\n  audit {\n  }\n}\n",
		"ref without op":    "Ref: users.id orders.user_id\n",
		"bad ref endpoint":  "Ref: users > orders.user_id\n",
	}
	for name, src := range cases {
		if _, err := dbml.Parser().(core.TreeParser).ParseTree([]byte(src)); err == nil {
			t.Errorf("%s: want parse error, got nil", name)
		} else if !strings.HasPrefix(err.Error(), "dbml: ") {
			t.Errorf("%s: error not attributed to the format: %v", name, err)
		}
	}
}

// A schema without a Project block still parses; the root has no title (the
// parser cannot see the filename), which is the engine's/theme's concern.
func TestNoProjectBlock(t *testing.T) {
	tree, err := dbml.Parser().(core.TreeParser).ParseTree([]byte("Table users {\n  id integer [pk]\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tree.Envelope["title"]; ok {
		t.Errorf("root title should be unset without a Project block: %v", tree.Envelope)
	}
	if len(tree.Nodes) != 1 || tree.Nodes[0].Type != dbml.NodeTableRef {
		t.Errorf("root nodes = %+v", tree.Nodes)
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
// contract example. Regenerate with: go test ./dbml -update
func TestGoldenDump(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.dbml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.dbml fixtures found")
	}

	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".dbml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := dbml.Parser().(core.TreeParser).ParseTree(raw)
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
