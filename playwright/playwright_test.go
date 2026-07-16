package playwright_test

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
	"github.com/gofuego/fuego-formats/playwright"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

func TestParserClaimsDefaults(t *testing.T) {
	fp, ok := playwright.Parser().(core.FilenameParser)
	if !ok {
		t.Fatal("playwright.Parser() must implement core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, playwright.DefaultPatterns) {
		t.Errorf("Filenames() = %v, want %v", got, playwright.DefaultPatterns)
	}
	if fp.Type() != "playwright" {
		t.Errorf("Type() = %q, want playwright", fp.Type())
	}
}

func TestParserIsTreeParser(t *testing.T) {
	if _, ok := playwright.Parser().(core.TreeParser); !ok {
		t.Fatal("playwright.Parser() must implement core.TreeParser")
	}
}

func TestWithPatternsOverride(t *testing.T) {
	fp := playwright.Parser(formatkit.WithPatterns("*.e2e.ts")).(core.FilenameParser)
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"*.e2e.ts"}) {
		t.Errorf("Filenames() = %v, want [*.e2e.ts]", got)
	}
}

func parseString(t *testing.T, src string) *core.PageTree {
	t.Helper()
	tree, err := playwright.Parser().(core.TreeParser).ParseTree([]byte(src))
	if err != nil {
		t.Fatalf("the scanner must never fail, got %v", err)
	}
	return tree
}

func loadFixtureTree(t *testing.T, name string) *core.PageTree {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return parseString(t, string(raw))
}

// Children are keyed by full slug paths under the root; suites nest as path
// segments. The slug rules are a stability promise.
func TestTreeShapeAndSlugs(t *testing.T) {
	tree := loadFixtureTree(t, "checkout.spec.ts")

	var keys []string
	for k := range tree.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{
		"checkout",
		"checkout/applies-gift-card",
		"checkout/discounts",
		"checkout/discounts/rejects-an-expired-coupon",
		"checkout/discounts/stacks-seasonal-discounts",
		"checkout/guest-can-pay-with-card",
		"formats-currency",
		"legacy-flow",
		"legacy-flow/redirects-to-the-old-cart",
		"prices-load-for-region",
		"top-level-smoke",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("children = %v, want %v", keys, want)
	}
}

func TestEnvelopes(t *testing.T) {
	tree := loadFixtureTree(t, "checkout.spec.ts")

	// The root has no title: the parser cannot see the filename.
	if _, ok := tree.Envelope["title"]; ok {
		t.Errorf("root title should be unset: %v", tree.Envelope)
	}
	if tree.Envelope["layout"] != playwright.Layout {
		t.Errorf("root envelope = %v", tree.Envelope)
	}

	suite := tree.Children["checkout"]
	if suite.Envelope["title"] != "Checkout" || suite.Envelope["layout"] != playwright.LayoutSuite {
		t.Errorf("suite envelope = %v", suite.Envelope)
	}
	// Tags are []any — taxonomy-scannable and cache-eligible.
	if tags, ok := suite.Envelope["tags"].([]any); !ok || !reflect.DeepEqual(tags, []any{"checkout"}) {
		t.Errorf("suite tags = %#v", suite.Envelope["tags"])
	}

	test := tree.Children["checkout/guest-can-pay-with-card"]
	if test.Envelope["title"] != "guest can pay with card" || test.Envelope["layout"] != playwright.LayoutTest {
		t.Errorf("test envelope = %v", test.Envelope)
	}
	// Effective tags: the suite's plus the test's own.
	if tags, ok := test.Envelope["tags"].([]any); !ok || !reflect.DeepEqual(tags, []any{"checkout", "smoke"}) {
		t.Errorf("test tags = %#v", test.Envelope["tags"])
	}
}

func TestAnnotations(t *testing.T) {
	tree := loadFixtureTree(t, "checkout.spec.ts")

	skip := tree.Children["checkout/applies-gift-card"]
	if a, ok := skip.Envelope["annotations"].([]any); !ok || !reflect.DeepEqual(a, []any{"skip"}) {
		t.Errorf("test.skip declaration annotations = %#v", skip.Envelope["annotations"])
	}

	// test.slow() inside the body annotates the test.
	slow := tree.Children["checkout/discounts/rejects-an-expired-coupon"]
	if a, ok := slow.Envelope["annotations"].([]any); !ok || !reflect.DeepEqual(a, []any{"slow"}) {
		t.Errorf("in-body slow annotations = %#v", slow.Envelope["annotations"])
	}

	fixme := tree.Children["checkout/discounts/stacks-seasonal-discounts"]
	if a, ok := fixme.Envelope["annotations"].([]any); !ok || !reflect.DeepEqual(a, []any{"fixme"}) {
		t.Errorf("test.fixme annotations = %#v", fixme.Envelope["annotations"])
	}

	// describe.skip inherits down to its tests.
	inherited := tree.Children["legacy-flow/redirects-to-the-old-cart"]
	if a, ok := inherited.Envelope["annotations"].([]any); !ok || !reflect.DeepEqual(a, []any{"skip"}) {
		t.Errorf("inherited annotations = %#v", inherited.Envelope["annotations"])
	}
}

func TestNodesAndTitlepath(t *testing.T) {
	tree := loadFixtureTree(t, "checkout.spec.ts")

	page := tree.Children["checkout/discounts/rejects-an-expired-coupon"]
	if len(page.Nodes) != 1 || page.Nodes[0].Type != playwright.NodeTest {
		t.Fatalf("test page nodes = %+v", page.Nodes)
	}
	attrs := page.Nodes[0].Attributes
	// The node carries the raw title (the stable identifier); the envelope
	// carries the cleaned one.
	if attrs["title"] != "rejects an expired coupon @Validation" {
		t.Errorf("raw title = %v", attrs["title"])
	}
	wantPath := []any{"Checkout @checkout", "Discounts", "rejects an expired coupon @Validation"}
	if got, ok := attrs["titlepath"].([]any); !ok || !reflect.DeepEqual(got, wantPath) {
		t.Errorf("titlepath = %#v, want %v", attrs["titlepath"], wantPath)
	}
	if attrs["dynamic"] != false {
		t.Errorf("dynamic = %v", attrs["dynamic"])
	}

	// Suite pages carry ref nodes to their immediate children, slugs relative
	// to the root page's URL.
	suite := tree.Children["checkout"]
	var refSlugs []string
	for _, n := range suite.Nodes {
		if n.Type == playwright.NodeTestRef || n.Type == playwright.NodeSuiteRef {
			refSlugs = append(refSlugs, n.Attributes["slug"].(string))
		}
	}
	wantRefs := []string{
		"checkout/guest-can-pay-with-card",
		"checkout/applies-gift-card",
		"checkout/discounts",
	}
	if !reflect.DeepEqual(refSlugs, wantRefs) {
		t.Errorf("suite ref slugs = %v, want %v", refSlugs, wantRefs)
	}
}

// Dynamic titles degrade to best-effort text — never a parse failure.
func TestDynamicTitlesDegrade(t *testing.T) {
	tree := loadFixtureTree(t, "checkout.spec.ts")

	tpl := tree.Children["prices-load-for-region"]
	if tpl.Envelope["title"] != "prices load for ${region}" {
		t.Errorf("template-literal title = %v", tpl.Envelope["title"])
	}
	if tpl.Nodes[0].Attributes["dynamic"] != true {
		t.Errorf("template-literal test not marked dynamic")
	}

	expr := tree.Children["formats-currency"]
	if expr.Envelope["title"] != "'formats ' + currency" {
		t.Errorf("expression title = %v", expr.Envelope["title"])
	}
	if expr.Nodes[0].Attributes["dynamic"] != true {
		t.Errorf("expression test not marked dynamic")
	}
}

// Colliding slugs (repeated dynamic titles) uniquify with a numeric suffix in
// source order instead of failing.
func TestCollidingSlugsUniquify(t *testing.T) {
	src := `
import { test } from '@playwright/test';
for (const tc of cases) {
  test(tc.name, async () => {});
}
for (const tc of moreCases) {
  test(tc.name, async () => {});
}
`
	tree := parseString(t, src)
	var keys []string
	for k := range tree.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"tc-name", "tc-name-2"}) {
		t.Errorf("children = %v, want [tc-name tc-name-2]", keys)
	}
}

// Conditional test.skip(condition)/test.fixme(condition) calls are
// annotations on the enclosing block, not test declarations.
func TestConditionalSkipAnnotatesNotDeclares(t *testing.T) {
	src := `
import { test } from '@playwright/test';
test.describe('Suite', () => {
  test.skip(({ browserName }) => browserName === 'firefox');
  test('works', async ({ page }) => {
    test.fixme(process.env.CI === 'true', 'flaky on CI');
  });
});
`
	tree := parseString(t, src)
	var keys []string
	for k := range tree.Children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"suite", "suite/works"}) {
		t.Fatalf("children = %v — a conditional annotation became a page", keys)
	}
	test := tree.Children["suite/works"]
	want := []any{"skip", "fixme"} // inherited suite skip, then own fixme
	if a, ok := test.Envelope["annotations"].([]any); !ok || !reflect.DeepEqual(a, want) {
		t.Errorf("annotations = %#v, want %v", test.Envelope["annotations"], want)
	}
}

// The scanner never fails — even on input that isn't Playwright, or isn't
// even JavaScript.
func TestNeverFails(t *testing.T) {
	for name, src := range map[string]string{
		"empty":            "",
		"not js":           "Table users {\n  id integer\n}\n",
		"unbalanced":       "test.describe('x', () => {\n  test('y', () => {\n",
		"multiline title":  "test(`a\nvery long\ntitle`, () => {});\n",
		"comment noise":    "/* { { { */\ntest('a', () => {}); // }\n",
		"describe.ConFiG?": "test.describe.configure({ mode: 'parallel' });\n",
	} {
		tree := parseString(t, src)
		if tree == nil {
			t.Errorf("%s: nil tree", name)
		}
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
// contract example. Regenerate with: go test ./playwright -update
func TestGoldenDump(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.spec.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.spec.ts fixtures found")
	}

	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".spec.ts")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			tree, err := playwright.Parser().(core.TreeParser).ParseTree(raw)
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
