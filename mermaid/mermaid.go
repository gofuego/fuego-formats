// Package mermaid is a fuego-formats parser for Mermaid diagram files (*.mmd).
//
// It is the trivial-tier exemplar of the fuego-formats conventions: one file
// becomes one page carrying a single raw node whose content is the diagram
// source wrapped in a <pre class="mermaid"> block, so a theme that loads
// mermaid.js renders it client-side. No server-side diagram rendering happens.
//
// Usage:
//
//	eng := engine.New()
//	eng.Register(mermaid.Parser())
//
// Override the default *.mmd claim for a brownfield repo:
//
//	eng.Register(mermaid.Parser(formatkit.WithPatterns("*.mermaid")))
//
// The emitted node type and envelope keys are the module's contract — see
// schema.md.
package mermaid

import (
	"html"
	"strings"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
)

// Type is the parser's Type() — the format slug.
const Type = "mermaid"

// Node types emitted by the parser. All are prefixed with the format slug so a
// theme's renderer templates never collide with another format's.
const (
	// NodeDiagram is the single node emitted per .mmd file: a raw node whose
	// content is the diagram source wrapped for client-side mermaid.js.
	NodeDiagram = "mermaid-diagram"
)

// Layout is the default layout name set in the envelope. A theme provides
// theme/layouts/mermaid.html to style diagram pages; absent that, the engine
// falls back to the base template silently.
const Layout = "mermaid"

// DefaultPattern is the built-in filename claim: base names ending in .mmd.
const DefaultPattern = "*.mmd"

// Option re-exports formatkit.Option so callers configure the parser without
// importing formatkit directly for the common case.
type Option = formatkit.Option

// Parser returns a Fuego parser claiming *.mmd by default. Pass
// formatkit.WithPatterns(...) to override the claim.
func Parser(opts ...Option) core.Parser {
	all := append([]Option{formatkit.WithDefaultPatterns(DefaultPattern)}, opts...)
	return formatkit.NewParser(Type, parse, all...)
}

// parse turns a .mmd file into one page: a title-and-layout envelope plus a
// single raw NodeDiagram node.
func parse(raw []byte) (core.Envelope, []core.Node, error) {
	src := string(raw)

	env := core.Envelope{"layout": Layout}
	if title := extractTitle(raw); title != "" {
		env["title"] = title
	}

	node := core.Node{
		Type:    NodeDiagram,
		Raw:     true,
		Content: wrap(src),
		Attributes: map[string]any{
			"source": strings.TrimRight(src, "\n"),
		},
	}
	return env, []core.Node{node}, nil
}

// wrap renders the diagram source into a <pre class="mermaid"> block. The
// mermaid.js client library scans for exactly this class and replaces the block
// with the rendered SVG. The source is HTML-escaped so diagram text containing
// '<' or '&' survives into the DOM as text for mermaid to read.
func wrap(src string) string {
	trimmed := strings.TrimRight(src, "\n")
	return `<pre class="mermaid">` + "\n" + html.EscapeString(trimmed) + "\n" + `</pre>`
}

// extractTitle derives the page title from the diagram's own metadata:
//
//   - a Mermaid YAML frontmatter block (--- ... ---) with a top-level "title:"
//     key, per Mermaid's config-frontmatter feature; or
//   - a "title <text>" directive line, used by pie/gantt/xychart diagrams.
//
// If neither is present the title is left unset — the parser cannot see the
// filename (Parse receives only the file bytes), so a filename-derived title is
// the engine's/theme's concern, not this parser's. This choice is documented in
// schema.md.
func extractTitle(raw []byte) string {
	if t := titleFromFrontmatter(raw); t != "" {
		return t
	}
	return titleFromDirective(raw)
}

// titleFromFrontmatter reads a leading "--- ... ---" block and returns the value
// of a top-level "title:" line, if any. It intentionally does not depend on a
// YAML library: the block is scanned line by line for a "title:" key.
func titleFromFrontmatter(raw []byte) string {
	s := string(raw)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return ""
	}
	lines := strings.Split(s, "\n")
	// lines[0] is the opening "---"; scan until the closing "---".
	for _, line := range lines[1:] {
		trimmed := strings.TrimRight(line, "\r")
		if strings.TrimSpace(trimmed) == "---" {
			return ""
		}
		if v, ok := parseTitleKey(trimmed); ok {
			return v
		}
	}
	return ""
}

// titleFromDirective scans for a "title <text>" line anywhere in the diagram —
// the form pie/gantt/xychart use. The first such line wins.
func titleFromDirective(raw []byte) string {
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if rest, ok := strings.CutPrefix(trimmed, "title "); ok {
			if t := strings.TrimSpace(rest); t != "" {
				return unquote(t)
			}
		}
	}
	return ""
}

// parseTitleKey parses a "title: value" line, returning the unquoted value.
func parseTitleKey(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "title:")
	if !ok {
		return "", false
	}
	v := strings.TrimSpace(rest)
	if v == "" {
		return "", false
	}
	return unquote(v), true
}

// unquote strips a single matching pair of surrounding quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
