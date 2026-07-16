// Package formatkit carries the claims/options boilerplate every fuego-formats
// module shares, so each format implements the filename-claim convention
// identically instead of copy-drifting.
//
// It is plumbing, not a framework. A format module defines its default claim
// patterns and a raw parse function, then calls [NewParser] to obtain a
// core.FilenameParser whose Filenames() reports the resolved patterns. A user
// overrides the defaults per registration with [WithPatterns]:
//
//	func Parser(opts ...formatkit.Option) core.Parser {
//		return formatkit.NewParser("mermaid", parseMermaid,
//			formatkit.WithDefaultPatterns("*.mmd"), opts...)
//	}
//
//	// user side:
//	eng.Register(mermaid.Parser(formatkit.WithPatterns("*.mermaid")))
//
// Claims match base names only — no path scoping, no content sniffing (the
// fuego-formats convention).
package formatkit

import (
	"strings"

	"github.com/gofuego/fuego/core"
)

// Option configures a parser's claims. Options apply in order, so a
// WithDefaultPatterns supplied by a format module is overridden by a
// WithPatterns a user passes after it.
type Option func(*config)

// config is the resolved claims/options state. It is unexported: modules and
// users interact with it only through Options.
type config struct {
	patterns []string
}

// WithDefaultPatterns sets the format's built-in claim patterns. Format modules
// call this inside their Parser constructor; it is the baseline a user may
// replace. Passing it more than once replaces the prior value.
func WithDefaultPatterns(patterns ...string) Option {
	return func(c *config) { c.patterns = append([]string(nil), patterns...) }
}

// WithPatterns overrides the claim patterns entirely — the escape hatch for a
// brownfield repo whose files don't match a format's defaults (e.g. specs named
// *.api.yaml instead of *.openapi.yaml). Users pass it when registering a
// format's parser.
func WithPatterns(patterns ...string) Option {
	return WithDefaultPatterns(patterns...)
}

// resolve applies opts over an empty config and returns the result.
func resolve(opts []Option) config {
	var c config
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// ParseFunc is a parser's core work: raw file bytes in, envelope and nodes out.
// It matches the shape of core.Parser.Parse minus the receiver.
type ParseFunc func(raw []byte) (core.Envelope, []core.Node, error)

// NewParser builds a core.FilenameParser for a format. typeName is the parser's
// Type() (the format slug); parse does the work; opts supply the claim patterns
// (typically a WithDefaultPatterns from the module followed by any user
// options).
//
// The returned parser reports its resolved patterns from Filenames(), so the
// engine dispatches to it by base-name glob.
func NewParser(typeName string, parse ParseFunc, opts ...Option) core.Parser {
	cfg := resolve(opts)
	return &parser{
		typeName: typeName,
		patterns: cfg.patterns,
		parse:    parse,
	}
}

// parser is the concrete core.FilenameParser NewParser returns.
type parser struct {
	typeName string
	patterns []string
	parse    ParseFunc
}

func (p *parser) Type() string { return p.typeName }

// Filenames reports the resolved claim patterns. It returns a copy so a caller
// cannot mutate the parser's claims.
func (p *parser) Filenames() []string {
	return append([]string(nil), p.patterns...)
}

func (p *parser) Parse(raw []byte) (core.Envelope, []core.Node, error) {
	return p.parse(raw)
}

// TreeParseFunc is a tree parser's core work: raw file bytes in, a page tree
// out. It matches the shape of core.TreeParser.ParseTree minus the receiver.
type TreeParseFunc func(raw []byte) (*core.PageTree, error)

// NewTreeParser builds a core.TreeParser (that is also a core.FilenameParser)
// for a format whose artifacts expand into a tree of pages — an OpenAPI spec,
// a database schema. typeName and opts work exactly as in [NewParser].
//
// The engine detects the TreeParser interface by assertion and calls ParseTree;
// the Parse method is a safety fallback returning the tree root's envelope and
// nodes, so the parser still behaves sensibly if driven as a plain Parser.
func NewTreeParser(typeName string, parseTree TreeParseFunc, opts ...Option) core.Parser {
	cfg := resolve(opts)
	return &treeParser{
		typeName:  typeName,
		patterns:  cfg.patterns,
		parseTree: parseTree,
	}
}

// treeParser is the concrete parser NewTreeParser returns.
type treeParser struct {
	typeName  string
	patterns  []string
	parseTree TreeParseFunc
}

func (p *treeParser) Type() string { return p.typeName }

// Filenames reports the resolved claim patterns as a copy, like parser's.
func (p *treeParser) Filenames() []string {
	return append([]string(nil), p.patterns...)
}

func (p *treeParser) ParseTree(raw []byte) (*core.PageTree, error) {
	return p.parseTree(raw)
}

// Parse satisfies core.Parser; the engine calls ParseTree instead once it
// detects the TreeParser interface, so this is only a fallback for callers
// driving the parser directly.
func (p *treeParser) Parse(raw []byte) (core.Envelope, []core.Node, error) {
	tree, err := p.parseTree(raw)
	if err != nil {
		return nil, nil, err
	}
	return tree.Envelope, tree.Nodes, nil
}

// Slugify is the shared slug convention of the fuego-formats modules:
// lowercase, camelCase boundaries become hyphens ("listInvoices" →
// "list-invoices"), and every run of characters outside [a-z0-9] collapses to
// a single hyphen ("/pets/{petId}" → "pets-pet-id"), with no leading or
// trailing hyphens. Each module's schema.md still owns its slug rules as a
// stability promise; sharing the implementation keeps the rules from
// copy-drifting apart.
func Slugify(s string) string {
	var b strings.Builder
	lastHyphen := true // suppress a leading hyphen
	prevLowerOrDigit := false
	for _, r := range s {
		isUpper := r >= 'A' && r <= 'Z'
		if isUpper && prevLowerOrDigit && !lastHyphen {
			b.WriteByte('-')
		}
		lower := r
		if isUpper {
			lower = r + ('a' - 'A')
		}
		switch {
		case lower >= 'a' && lower <= 'z', lower >= '0' && lower <= '9':
			b.WriteRune(lower)
			lastHyphen = false
			prevLowerOrDigit = !isUpper || lower >= '0' && lower <= '9'
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
			prevLowerOrDigit = false
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
