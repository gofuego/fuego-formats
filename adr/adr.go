// Package adr is a fuego-formats parser for Architecture Decision Records —
// Markdown files with YAML frontmatter following the fuego-adr convention,
// claimed by the *.adr.md compound suffix.
//
// One ADR becomes one page: the parser splits the YAML frontmatter, normalizes
// the convention's metadata fields (list fields to []string / []int, status to
// lowercase, dates to YYYY-MM-DD strings), splits the body on ## headings, and
// emits one raw HTML node per section (goldmark, GFM). Relative cross-links
// between ADR files rewrite to the conventional decision route.
//
// The module also exports the convention helpers a consumer tool builds on —
// ExtractADRNumber, ValidateSections, ValidStatuses, RequiredSections — so
// fuego-adr's hooks and CLI (and any other tool) share one definition of the
// ADR contract. The full contract is documented in schema.md.
//
// Usage:
//
//	eng := engine.New()
//	eng.Register(adr.Parser())
//
// Under specificity dispatch the *.adr.md claim safely coexists with a
// markdown parser's bare md claim: guide.adr.md goes to this parser, plain
// notes.md to markdown. Override the claim with formatkit.WithPatterns(...).
package adr

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofuego/fuego/core"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/gofuego/fuego-formats/formatkit"
)

// Type is the parser's Type() — the format slug and every page's type.
const Type = "adr"

// Node types for the conventional sections. Every ## heading becomes one node
// whose type is SectionNodeType(heading) — these constants cover the sections
// the convention names; free-form headings get "adr-" plus their slug
// ("## Options Considered" → "adr-options-considered"). Content is the
// section's Markdown rendered to HTML, marked Raw.
const (
	// NodePreamble is content before the first ## heading.
	NodePreamble = "adr-preamble"
	// NodeContext is the ## Context section.
	NodeContext = "adr-context"
	// NodeDecision is the ## Decision section.
	NodeDecision = "adr-decision"
	// NodeConsequences is the ## Consequences section.
	NodeConsequences = "adr-consequences"
)

// SectionNodeType maps a section heading to its node type: "adr-" plus the
// slugified heading (the shared fuego-formats slug convention). A heading
// that slugifies to nothing falls back to "adr-section".
func SectionNodeType(heading string) string {
	slug := formatkit.Slugify(heading)
	if slug == "" {
		slug = "section"
	}
	return "adr-" + slug
}

// DefaultPatterns is the built-in filename claim — the ADR compound suffix.
// Claims match base names only.
var DefaultPatterns = []string{"*.adr.md"}

// RequiredSections are the section headings ValidateSections enforces for
// accepted ADRs.
var RequiredSections = []string{"context", "decision", "consequences"}

// ValidStatuses is the set of allowed status values — the convention's status
// flow: tbd → proposed → accepted → deprecated / superseded.
var ValidStatuses = map[string]bool{
	"tbd":        true,
	"proposed":   true,
	"accepted":   true,
	"deprecated": true,
	"superseded": true,
}

// Option re-exports formatkit.Option so callers configure the parser without
// importing formatkit directly for the common case.
type Option = formatkit.Option

// Parser returns a Fuego parser claiming *.adr.md. Pass
// formatkit.WithPatterns(...) to override the claim.
func Parser(opts ...Option) core.Parser {
	all := append([]Option{formatkit.WithDefaultPatterns(DefaultPatterns...)}, opts...)
	return formatkit.NewParser(Type, parse, all...)
}

var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// adrNumberRe matches the leading numeric prefix in filenames like
// "0012-use-postgres.adr.md".
var adrNumberRe = regexp.MustCompile(`^(\d+)-`)

// parse extracts frontmatter, normalizes metadata, splits the body into
// sections, and renders each section's Markdown to HTML. It deliberately
// emits no layout key — the consuming pack (fuego-adr's AfterParse hook)
// owns layout defaulting.
func parse(raw []byte) (core.Envelope, []core.Node, error) {
	env, payload, err := core.SplitFrontmatter(raw)
	if err != nil {
		return nil, nil, err
	}
	if env == nil {
		env = make(core.Envelope)
	}

	normalizeEnvelope(env)

	sections := splitSections(payload)
	var nodes []core.Node
	for _, sec := range sections {
		html, err := renderMarkdown(sec.content)
		if err != nil {
			return nil, nil, fmt.Errorf("adr: rendering section %q: %w", sec.heading, err)
		}
		nodes = append(nodes, core.Node{
			Type:    SectionNodeType(sec.heading),
			Content: html,
			Raw:     true,
		})
	}

	return env, nodes, nil
}

// section represents a parsed body section.
type section struct {
	heading string // lowercase, e.g. "context", "decision"
	content []byte // raw Markdown content under the heading
}

// splitSections splits the Markdown body on ## headings.
// Content before the first heading is emitted under the "preamble" heading.
func splitSections(body []byte) []section {
	lines := bytes.Split(body, []byte("\n"))
	var sections []section
	var current *section

	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("## ")) {
			if current != nil && len(bytes.TrimSpace(current.content)) > 0 {
				sections = append(sections, *current)
			}
			heading := strings.ToLower(strings.TrimSpace(string(trimmed[3:])))
			current = &section{heading: heading}
			continue
		}
		if current != nil {
			current.content = append(current.content, line...)
			current.content = append(current.content, '\n')
		} else if len(trimmed) > 0 {
			// Content before first heading
			current = &section{heading: "preamble"}
			current.content = append(current.content, line...)
			current.content = append(current.content, '\n')
		}
	}
	if current != nil && len(bytes.TrimSpace(current.content)) > 0 {
		sections = append(sections, *current)
	}
	return sections
}

// adrCrossLinkRe matches a rendered relative link to another ADR file —
// href="012-foo.adr.md" or href="012-foo.adr.md#decision" — but not absolute
// URLs or paths (it forbids ':' '/' '?' '#' in the filename).
var adrCrossLinkRe = regexp.MustCompile(`href="([^":/?#]+)\.adr\.md(#[^"]*)?"`)

// renderMarkdown converts Markdown bytes to HTML and rewrites relative
// cross-links between ADR files ("NNN-slug.adr.md") to the conventional
// decision page route ("decisions/NNN-slug.adr/" — the route fuego-adr's
// config defaults establish; a standalone site configures the same
// `adr: /decisions/{slug}` route pattern for the links to resolve). The
// result is base-relative, so it resolves correctly under the site's
// <base href> for any deployment base URL.
func renderMarkdown(src []byte) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return "", err
	}
	html := adrCrossLinkRe.ReplaceAllString(buf.String(), `href="decisions/$1.adr/$2"`)
	return html, nil
}

// normalizeEnvelope converts scalar-or-list fields to consistent list form
// and normalizes status to lowercase. []string and []int are deliberate (not
// []any): both are in the engine's registered cache set, and consumer hooks
// assert these exact types.
func normalizeEnvelope(env core.Envelope) {
	for _, key := range []string{"author", "approvers", "tags", "affects"} {
		if v, ok := env[key]; ok {
			env[key] = toStringSlice(v)
		}
	}
	for _, key := range []string{"supersedes", "superseded_by"} {
		if v, ok := env[key]; ok {
			env[key] = toIntSlice(v)
		}
	}

	if s, ok := env["status"].(string); ok {
		env["status"] = strings.ToLower(strings.TrimSpace(s))
	}

	// Normalize date fields: time.Time → "YYYY-MM-DD" string
	for _, key := range []string{"date_proposed", "date_accepted", "date_deprecated", "date_superseded", "deadline"} {
		if v, ok := env[key]; ok {
			env[key] = formatDate(v)
		}
	}
}

// formatDate converts a value to a date string.
// YAML parses bare dates like 2026-01-15 as time.Time.
func formatDate(v any) string {
	switch d := v.(type) {
	case time.Time:
		return d.Format("2006-01-02")
	case string:
		return d
	default:
		return fmt.Sprintf("%v", v)
	}
}

// toStringSlice normalizes a value to []string.
// Accepts: string, []any (of strings), []string.
func toStringSlice(v any) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return val
	default:
		return nil
	}
}

// toIntSlice normalizes a value to []int.
// Accepts: int, float64 (YAML numbers), []any (of numbers), []int.
func toIntSlice(v any) []int {
	switch val := v.(type) {
	case int:
		return []int{val}
	case float64:
		return []int{int(val)}
	case []any:
		out := make([]int, 0, len(val))
		for _, item := range val {
			switch n := item.(type) {
			case int:
				out = append(out, n)
			case float64:
				out = append(out, int(n))
			}
		}
		return out
	case []int:
		return val
	default:
		return nil
	}
}

// ExtractADRNumber parses the leading number from a filename like
// "0012-use-postgres.adr.md". Returns -1 if no number is found.
func ExtractADRNumber(filename string) int {
	base := filepath.Base(filename)
	m := adrNumberRe.FindStringSubmatch(base)
	if m == nil {
		return -1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return n
}

// ValidateSections checks whether a page with the given status has all
// RequiredSections. It returns the missing section names (plain headings,
// not node types — they are meant for human-readable warnings). Only
// enforced for "accepted" status.
func ValidateSections(status string, nodes []core.Node) []string {
	if status != "accepted" {
		return nil
	}

	present := make(map[string]bool)
	for _, n := range nodes {
		present[n.Type] = true
	}

	var missing []string
	for _, req := range RequiredSections {
		if !present[SectionNodeType(req)] {
			missing = append(missing, req)
		}
	}
	return missing
}
