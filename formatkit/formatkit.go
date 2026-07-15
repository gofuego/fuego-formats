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

import "github.com/gofuego/fuego/core"

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
