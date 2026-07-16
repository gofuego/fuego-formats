// Package playwright is a fuego-formats TreeParser for Playwright spec files.
//
// It is explicitly NOT a JavaScript parser. A shallow structural scanner
// reads describe/test block structure — titles, @tags in titles, and
// skip/fixme/slow annotations — and expands one spec file into a routed
// section: a file root page plus a tree of real pages, one per suite and one
// per test. Test and suite envelopes carry their effective tags ([]any,
// taxonomy-scannable), so a site taxonomy gives "browse tests by tag" for
// free.
//
// The scanner never fails: dynamic or computed titles (template literals,
// expressions), multi-line trickery, and unbalanced structure all degrade to
// best-effort raw text and best-effort structure — a spec file that runs
// under Playwright always produces a tree. The v1 scope boundary is
// documented in schema.md.
//
// Usage:
//
//	eng := engine.New()
//	eng.Register(playwright.Parser())
//
// The default claims are upstream's own compound-suffix convention
// (*.test.js, *.test.ts, *.spec.js, *.spec.ts); under specificity dispatch
// they outrank a site-wide bare js/ts parser. Override with
// formatkit.WithPatterns(...).
package playwright

import (
	"fmt"
	"strings"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
)

// Type is the parser's Type() — the format slug and every page's type.
const Type = "playwright"

// Node types emitted by the parser. All are prefixed with the format slug so
// a theme's renderer templates never collide with another format's.
const (
	// NodeSuiteRef is a link node on the root and suite pages pointing at a
	// child suite page (attrs: title, slug, tags, annotations). slug is the
	// child's slug path relative to the root page's URL.
	NodeSuiteRef = "playwright-suite-ref"
	// NodeTestRef is a link node on the root and suite pages pointing at a
	// child test page (attrs: title, slug, tags, annotations).
	NodeTestRef = "playwright-test-ref"
	// NodeSuite is a suite page's main node. Attributes carry the raw title
	// as written (the stable identifier), titlepath, tags, annotations, and
	// dynamic.
	NodeSuite = "playwright-suite"
	// NodeTest is a test page's main node. Attributes carry the raw title as
	// written, titlepath (the Playwright test identity: every ancestor
	// suite's raw title plus the test's own), tags, annotations, and dynamic.
	NodeTest = "playwright-test"
)

// Default layout names set in the envelopes. A theme provides
// theme/layouts/playwright.html (and the per-kind variants) to style the
// pages; absent that, the engine falls back to the base template silently.
const (
	Layout      = "playwright"
	LayoutSuite = "playwright-suite"
	LayoutTest  = "playwright-test"
)

// DefaultPatterns are the built-in filename claims — upstream Playwright's
// own compound-suffix convention. Claims match base names only.
var DefaultPatterns = []string{
	"*.test.js", "*.test.ts",
	"*.spec.js", "*.spec.ts",
}

// Option re-exports formatkit.Option so callers configure the parser without
// importing formatkit directly for the common case.
type Option = formatkit.Option

// Parser returns a Fuego TreeParser claiming the DefaultPatterns. Pass
// formatkit.WithPatterns(...) to override the claims.
func Parser(opts ...Option) core.Parser {
	all := append([]Option{formatkit.WithDefaultPatterns(DefaultPatterns...)}, opts...)
	return formatkit.NewTreeParser(Type, parseTree, all...)
}

// parseTree scans a spec file and expands it into the page tree documented in
// schema.md. It never returns an error: unparseable structure degrades to
// best-effort text and shape instead (see the package comment).
func parseTree(raw []byte) (*core.PageTree, error) {
	root := scan(raw)
	tree := &core.PageTree{
		Envelope: core.Envelope{"layout": Layout},
		Children: map[string]*core.PageTree{},
	}
	emitChildren(tree, tree, root, "", nil, nil, nil)
	return tree, nil
}

// --- the scanned model --------------------------------------------------------

// block is one describe suite or one test.
type block struct {
	kind        string // "suite" or "test"
	rawTitle    string // the first argument as written, quotes stripped
	title       string // rawTitle with @tags removed and whitespace collapsed
	tags        []string
	annotations []string
	dynamic     bool // the title was not a plain string literal
	children    []*block
	openDepth   int // brace depth inside the block's body
}

// --- scanning ------------------------------------------------------------------

// scan walks the file line by line, tracking brace depth to nest blocks. It
// returns a pseudo-suite holding the top-level blocks.
func scan(raw []byte) *block {
	root := &block{kind: "suite"}
	stack := []*block{root}
	depth := 0
	inComment := false // inside /* ... */
	inTemplate := false

	nearestSuite := func() *block {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].kind == "suite" {
				return stack[i]
			}
		}
		return root
	}

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSuffix(line, "\r")

		var decl *block
		if !inComment && !inTemplate {
			trimmed := strings.TrimSpace(line)
			if kind, mods, rest, ok := matchDeclaration(trimmed); ok {
				title, dynamic := extractTitle(rest)
				if kind == "test" && dynamic && len(mods) == 1 && mods[0] != "only" {
					// test.skip(condition, ...)/test.fixme(condition, ...)
					// without a title string is Playwright's conditional
					// form — an annotation on the enclosing block, not a
					// test declaration.
					top := stack[len(stack)-1]
					top.annotations = appendUnique(top.annotations, mods[0])
				} else {
					decl = &block{kind: kind, rawTitle: title, dynamic: dynamic}
					decl.title, decl.tags = extractTags(title)
					decl.annotations = declarationAnnotations(mods)
				}
			} else if a, isAnnotation := matchBodyAnnotation(trimmed); isAnnotation {
				// A bare test.slow(...) call annotates the enclosing block.
				top := stack[len(stack)-1]
				top.annotations = appendUnique(top.annotations, a)
			}
		}

		var delta int
		delta, inComment, inTemplate = braceDelta(line, inComment, inTemplate)
		depth += delta

		if decl != nil {
			nearestSuite().children = append(nearestSuite().children, decl)
			if delta > 0 {
				decl.openDepth = depth
				stack = append(stack, decl)
			}
			// A block whose braces balance on its own line (a one-line test)
			// is already complete.
		}

		for len(stack) > 1 && depth < stack[len(stack)-1].openDepth {
			stack = stack[:len(stack)-1]
		}
	}
	return root
}

// matchDeclaration reports whether a line opens a suite or test declaration:
// test(...), test.only/skip/fixme(...), test.describe(...) with optional
// only/skip/fixme/serial/parallel modifiers, or a bare describe(...). It
// returns the kind, the modifier chain, and the text after the opening
// parenthesis.
func matchDeclaration(t string) (kind string, mods []string, rest string, ok bool) {
	chain, rest, found := identChain(t)
	if !found || len(chain) == 0 {
		return "", nil, "", false
	}
	switch chain[0] {
	case "test":
		if len(chain) == 1 {
			return "test", nil, rest, true
		}
		if chain[1] == "describe" {
			return classifySuite(chain[2:], rest)
		}
		if len(chain) == 2 && isTestModifier(chain[1]) {
			return "test", chain[1:], rest, true
		}
	case "describe":
		return classifySuite(chain[1:], rest)
	}
	return "", nil, "", false
}

func classifySuite(mods []string, rest string) (string, []string, string, bool) {
	for _, m := range mods {
		switch m {
		case "only", "skip", "fixme", "serial", "parallel":
		default:
			// describe.configure(...) and anything unrecognized is not a
			// suite declaration.
			return "", nil, "", false
		}
	}
	return "suite", mods, rest, true
}

func isTestModifier(m string) bool {
	return m == "only" || m == "skip" || m == "fixme"
}

// matchBodyAnnotation matches a bare test.slow(...) call — always an
// annotation, never a declaration. (Conditional test.skip(...)/test.fixme(...)
// annotations are recognized on the matchDeclaration path instead, because
// those chains are also valid declaration forms.)
func matchBodyAnnotation(t string) (string, bool) {
	chain, _, found := identChain(t)
	if found && len(chain) == 2 && chain[0] == "test" && chain[1] == "slow" {
		return "slow", true
	}
	return "", false
}

// identChain splits a leading dotted identifier chain followed by an opening
// parenthesis: "test.describe.only('x', ..." → ([test describe only], "'x', ...").
func identChain(t string) (chain []string, rest string, ok bool) {
	i := 0
	for {
		start := i
		for i < len(t) && (isIdentByte(t[i])) {
			i++
		}
		if i == start {
			return nil, "", false
		}
		chain = append(chain, t[start:i])
		if i < len(t) && t[i] == '.' {
			i++
			continue
		}
		break
	}
	if i < len(t) && t[i] == '(' {
		return chain, t[i+1:], true
	}
	return nil, "", false
}

func isIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '$'
}

// extractTitle reads the declaration's first argument. A plain string literal
// is the title; anything else — a template literal with placeholders, an
// expression, a string the expression continues past — degrades to the raw
// argument text with dynamic=true, never a failure.
func extractTitle(rest string) (string, bool) {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", true
	}
	switch rest[0] {
	case '\'', '"', '`':
		quote := rest[0]
		for i := 1; i < len(rest); i++ {
			if rest[i] == '\\' {
				i++
				continue
			}
			if rest[i] == quote {
				title := rest[1:i]
				// If the argument continues past the literal ("a" + x), the
				// title is really an expression.
				after := strings.TrimSpace(rest[i+1:])
				if after != "" && after[0] != ',' && after[0] != ')' {
					return rawArgument(rest), true
				}
				return title, quote == '`' && strings.Contains(title, "${")
			}
		}
		// Unterminated on this line (a multi-line literal): best effort.
		return strings.TrimSpace(rest[1:]), true
	default:
		return rawArgument(rest), true
	}
}

// rawArgument captures the first argument as raw text: everything up to the
// first comma outside quotes and brackets, or the line's end.
func rawArgument(rest string) string {
	depth := 0
	var quote byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if c == ')' && depth == 0 {
				return strings.TrimSpace(rest[:i])
			}
			depth--
		case ',':
			if depth == 0 {
				return strings.TrimSpace(rest[:i])
			}
		}
	}
	return strings.TrimSpace(rest)
}

// extractTags pulls @tag tokens out of a title: "pays with card @smoke" →
// ("pays with card", [smoke]). If removing the tags empties the title, the
// raw title is kept.
func extractTags(title string) (string, []string) {
	var tags []string
	var kept []string
	for _, f := range strings.Fields(title) {
		if len(f) > 1 && f[0] == '@' {
			tags = append(tags, f[1:])
			continue
		}
		kept = append(kept, f)
	}
	cleaned := strings.Join(kept, " ")
	if cleaned == "" {
		cleaned = strings.TrimSpace(title)
	}
	return cleaned, tags
}

func declarationAnnotations(mods []string) []string {
	var out []string
	for _, m := range mods {
		switch m {
		case "only", "skip", "fixme":
			out = append(out, m)
		}
	}
	return out
}

// braceDelta counts the net {/} on a line, ignoring string literals and
// comments. inComment and inTemplate carry /* */ and template-literal state
// across lines; single- and double-quoted strings never span lines.
func braceDelta(line string, inComment, inTemplate bool) (int, bool, bool) {
	delta := 0
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inComment:
			if c == '*' && i+1 < len(line) && line[i+1] == '/' {
				inComment = false
				i++
			}
		case inTemplate:
			if c == '\\' {
				i++
			} else if c == '`' {
				inTemplate = false
			}
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		default:
			switch c {
			case '\'', '"':
				quote = c
			case '`':
				inTemplate = true
			case '/':
				if i+1 < len(line) {
					if line[i+1] == '/' {
						return delta, false, false
					}
					if line[i+1] == '*' {
						inComment = true
						i++
					}
				}
			case '{':
				delta++
			case '}':
				delta--
			}
		}
	}
	return delta, inComment, inTemplate
}

func appendUnique(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

// --- emitting the page tree -----------------------------------------------------

// emitChildren adds b's children to parentPage (ref nodes) and to the tree
// root's Children (real pages), threading the inherited state down:
// slugPrefix is the parent's slug path, titlepath/tags/annotations are the
// ancestors' accumulated values.
func emitChildren(root, parentPage *core.PageTree, b *block, slugPrefix string, titlepath, tags, annotations []string) {
	taken := map[string]bool{}
	for _, child := range b.children {
		slug := formatkit.Slugify(child.title)
		if slug == "" {
			slug = child.kind
		}
		// Colliding slugs (dynamic titles, repeated raw text) uniquify with a
		// numeric suffix in source order — degradation, never a failure.
		if taken[slug] {
			for n := 2; ; n++ {
				if s := fmt.Sprintf("%s-%d", slug, n); !taken[s] {
					slug = s
					break
				}
			}
		}
		taken[slug] = true
		slugPath := slug
		if slugPrefix != "" {
			slugPath = slugPrefix + "/" + slug
		}

		childTitlepath := append(append([]string(nil), titlepath...), child.rawTitle)
		childTags := mergeUnique(tags, child.tags)
		childAnnotations := mergeUnique(annotations, child.annotations)

		env := core.Envelope{"title": child.title}
		attrs := map[string]any{
			"title":       child.rawTitle,
			"titlepath":   toAny(childTitlepath),
			"tags":        toAny(childTags),
			"annotations": toAny(childAnnotations),
			"dynamic":     child.dynamic,
		}
		if len(childTags) > 0 {
			env["tags"] = toAny(childTags)
		}
		if len(childAnnotations) > 0 {
			env["annotations"] = toAny(childAnnotations)
		}

		refType := NodeTestRef
		if child.kind == "suite" {
			refType = NodeSuiteRef
		}
		parentPage.Nodes = append(parentPage.Nodes, core.Node{
			Type: refType,
			Attributes: map[string]any{
				"title":       child.title,
				"slug":        slugPath,
				"tags":        toAny(childTags),
				"annotations": toAny(childAnnotations),
			},
		})

		page := &core.PageTree{Envelope: env}
		if child.kind == "suite" {
			env["layout"] = LayoutSuite
			page.Nodes = []core.Node{{Type: NodeSuite, Attributes: attrs}}
			root.Children[slugPath] = page
			emitChildren(root, page, child, slugPath, childTitlepath, childTags, childAnnotations)
		} else {
			env["layout"] = LayoutTest
			page.Nodes = []core.Node{{Type: NodeTest, Attributes: attrs}}
			root.Children[slugPath] = page
		}
	}
}

func mergeUnique(inherited, own []string) []string {
	out := append([]string(nil), inherited...)
	for _, v := range own {
		out = appendUnique(out, v)
	}
	return out
}

func toAny(list []string) []any {
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out
}
