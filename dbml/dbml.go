// Package dbml is a fuego-formats TreeParser for DBML (Database Markup
// Language) schemas.
//
// One .dbml file becomes a whole section of the site: a routed root page (the
// schema index, carrying the project block, enums, and table groups) plus one
// real page per table, with columns, indexes, and relationships as nodes.
// Children carry their own envelopes, so site taxonomies, collections, and
// pagination see them natively; the manifest lists every page against the
// schema file, so fuego-studio treats each as editable-as-the-schema.
//
// The scanner is hand-rolled and dependency-free — this module is the
// exemplar for block-DSL formats. It is deliberately strict at the top level
// (an unrecognized statement is a parse error, which the engine records as a
// LocalFatal for this file only) and lenient inside blocks (unknown column
// settings are ignored), so a schema written for a newer DBML dialect
// degrades predictably. Node attributes carry the stable identifiers (table
// and column names) future cross-artifact linking hooks consume.
//
// Usage:
//
//	eng := engine.New()
//	eng.Register(dbml.Parser())
//
// Override the default *.dbml claim for a brownfield repo:
//
//	eng.Register(dbml.Parser(formatkit.WithPatterns("*.database")))
//
// The emitted node types, envelope keys, tree shape, and slug rules are the
// module's contract — see schema.md.
package dbml

import (
	"fmt"
	"strings"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
)

// Type is the parser's Type() — the format slug and every page's type.
const Type = "dbml"

// Node types emitted by the parser. All are prefixed with the format slug so a
// theme's renderer templates never collide with another format's.
const (
	// NodeProject is the root page's overview node, emitted when the schema
	// has a Project block. Content is the project note; attributes carry
	// name and database_type.
	NodeProject = "dbml-project"
	// NodeTableRef is a link node on the root page pointing at a table page
	// (attrs: name, alias, slug, summary). slug is the child's slug path
	// relative to the root page's URL.
	NodeTableRef = "dbml-table-ref"
	// NodeEnum is one enum on the root page. Content is the enum note;
	// attributes carry name and values ([]any of the value names).
	NodeEnum = "dbml-enum"
	// NodeTableGroup is one table group on the root page. Content is the
	// group note; attributes carry name and tables ([]any of table names).
	NodeTableGroup = "dbml-table-group"
	// NodeTable is a table page's main node. Content is the table note;
	// attributes carry name and alias.
	NodeTable = "dbml-table"
	// NodeColumn is one column of a table, in declaration order. Content is
	// the column note; attributes carry name, type, pk, not_null, unique,
	// increment, and default.
	NodeColumn = "dbml-column"
	// NodeIndex is one entry of a table's indexes block, in declaration
	// order. Content is the index note; attributes carry columns ([]any),
	// name, type, pk, and unique.
	NodeIndex = "dbml-index"
	// NodeRef is one relationship involving the page's table, emitted on the
	// pages of both endpoint tables (once for a self-reference). Attributes
	// carry name, from_table, from_column, to_table, to_column, relation,
	// on_delete, and on_update; aliases are resolved to table names.
	NodeRef = "dbml-ref"
)

// Default layout names set in the envelopes. A theme provides
// theme/layouts/dbml.html (and the table variant) to style the pages; absent
// that, the engine falls back to the base template silently.
const (
	Layout      = "dbml"
	LayoutTable = "dbml-table"
)

// DefaultPatterns is the built-in filename claim. Claims match base names
// only — DBML has its own extension, so no compound suffix is needed.
var DefaultPatterns = []string{"*.dbml"}

// Option re-exports formatkit.Option so callers configure the parser without
// importing formatkit directly for the common case.
type Option = formatkit.Option

// Parser returns a Fuego TreeParser claiming the DefaultPatterns. Pass
// formatkit.WithPatterns(...) to override the claims.
func Parser(opts ...Option) core.Parser {
	all := append([]Option{formatkit.WithDefaultPatterns(DefaultPatterns...)}, opts...)
	return formatkit.NewTreeParser(Type, parseTree, all...)
}

// parseTree scans a DBML schema and expands it into the page tree documented
// in schema.md. Scan errors (an unrecognized top-level statement, a block
// left open, a malformed ref) return an error, which the engine turns into a
// LocalFatal for this file — the rest of the site still builds.
func parseTree(raw []byte) (*core.PageTree, error) {
	sch, err := parse(raw)
	if err != nil {
		return nil, err
	}
	return emit(sch)
}

// --- the parsed model -------------------------------------------------------

type schema struct {
	project *project
	tables  []*table
	enums   []*enum
	groups  []*tableGroup
	refs    []*ref
}

type project struct {
	name         string
	databaseType string
	note         string
}

type table struct {
	name    string
	alias   string
	note    string
	columns []*column
	indexes []*index
}

type column struct {
	name      string
	typ       string
	pk        bool
	notNull   bool
	unique    bool
	increment bool
	def       string
	note      string
}

type index struct {
	columns []string
	name    string
	typ     string
	pk      bool
	unique  bool
	note    string
}

type enum struct {
	name   string
	values []string
	note   string
}

type tableGroup struct {
	name   string
	tables []string
	note   string
}

type ref struct {
	name       string
	fromTable  string
	fromColumn string
	toTable    string
	toColumn   string
	relation   string
	onDelete   string
	onUpdate   string
}

// --- scanning ----------------------------------------------------------------

// scanner walks the file line by line. Line numbers are 1-based and refer to
// the last line returned by next.
type scanner struct {
	lines []string
	i     int
}

func (s *scanner) next() (string, bool) {
	if s.i >= len(s.lines) {
		return "", false
	}
	line := strings.TrimSuffix(s.lines[s.i], "\r")
	s.i++
	return line, true
}

func (s *scanner) lineNo() int { return s.i }

func errf(ln int, format string, args ...any) error {
	return fmt.Errorf("dbml: line %d: %s", ln, fmt.Sprintf(format, args...))
}

func parse(raw []byte) (*schema, error) {
	s := &scanner{lines: strings.Split(string(raw), "\n")}
	sch := &schema{}
	for {
		line, ok := s.next()
		if !ok {
			return sch, nil
		}
		t := strings.TrimSpace(stripComment(line))
		if t == "" {
			continue
		}
		kw, rest := keyword(t)
		var err error
		switch strings.ToLower(kw) {
		case "project":
			err = parseProject(sch, rest, s)
		case "table":
			err = parseTable(sch, rest, s)
		case "enum":
			err = parseEnum(sch, rest, s)
		case "tablegroup":
			err = parseTableGroup(sch, rest, s)
		case "ref":
			err = parseRef(sch, rest, s)
		default:
			return nil, errf(s.lineNo(), "unrecognized top-level statement %q", firstField(t))
		}
		if err != nil {
			return nil, err
		}
	}
}

// keyword splits the leading run of letters off a statement: "TableGroup x {"
// → ("TableGroup", "x {"), "Ref: a.b > c.d" → ("Ref", ": a.b > c.d").
func keyword(t string) (string, string) {
	i := 0
	for i < len(t) && (t[i] >= 'a' && t[i] <= 'z' || t[i] >= 'A' && t[i] <= 'Z') {
		i++
	}
	return t[:i], strings.TrimSpace(t[i:])
}

func firstField(t string) string {
	if f := strings.Fields(t); len(f) > 0 {
		return f[0]
	}
	return t
}

// blockHeader parses `<name> [as <alias>] [<settings>] {` — the shared shape
// of every top-level block header. Header settings (e.g. headercolor) are
// ignored.
func blockHeader(kind, rest string, ln int) (name, alias string, err error) {
	body, ok := strings.CutSuffix(strings.TrimSpace(rest), "{")
	if !ok {
		return "", "", errf(ln, "%s header must end with '{'", kind)
	}
	if i := indexTopLevel(body, '['); i >= 0 {
		body = body[:i]
	}
	tokens := tokenize(body)
	if len(tokens) == 0 {
		return "", "", errf(ln, "%s is missing a name", kind)
	}
	name = unquote(tokens[0])
	switch {
	case len(tokens) == 1:
		return name, "", nil
	case len(tokens) == 3 && strings.EqualFold(tokens[1], "as"):
		return name, unquote(tokens[2]), nil
	default:
		return "", "", errf(ln, "malformed %s header %q", kind, strings.TrimSpace(rest))
	}
}

func parseProject(sch *schema, rest string, s *scanner) error {
	ln := s.lineNo()
	if sch.project != nil {
		return errf(ln, "duplicate Project block")
	}
	name, _, err := blockHeader("Project", rest, ln)
	if err != nil {
		return err
	}
	p := &project{name: name}
	for {
		line, ok := s.next()
		if !ok {
			return errf(ln, "Project block is never closed")
		}
		t := strings.TrimSpace(stripComment(line))
		switch {
		case t == "":
		case t == "}":
			sch.project = p
			return nil
		case hasKeyPrefix(t, "note"):
			if p.note, err = parseNoteValue(valueAfterKey(t, "note"), s); err != nil {
				return err
			}
		case hasKeyPrefix(t, "database_type"):
			p.databaseType = unquote(valueAfterKey(t, "database_type"))
		default:
			// Other project metadata (language, etc.) is ignored.
		}
	}
}

func parseTable(sch *schema, rest string, s *scanner) error {
	ln := s.lineNo()
	name, alias, err := blockHeader("Table", rest, ln)
	if err != nil {
		return err
	}
	tb := &table{name: name, alias: alias}
	for {
		line, ok := s.next()
		if !ok {
			return errf(ln, "Table %q is never closed", name)
		}
		t := strings.TrimSpace(stripComment(line))
		switch {
		case t == "":
		case t == "}":
			sch.tables = append(sch.tables, tb)
			return nil
		case hasKeyPrefix(t, "note"):
			if tb.note, err = parseNoteValue(valueAfterKey(t, "note"), s); err != nil {
				return err
			}
		case isIndexesOpen(t):
			if err := parseIndexes(tb, name, s); err != nil {
				return err
			}
		case strings.HasSuffix(t, "{"):
			return errf(s.lineNo(), "unsupported block inside Table %q: %q", name, firstField(t))
		default:
			col, err := parseColumn(sch, tb, t, s.lineNo())
			if err != nil {
				return err
			}
			tb.columns = append(tb.columns, col)
		}
	}
}

func isIndexesOpen(t string) bool {
	rest, ok := strings.CutPrefix(strings.ToLower(t), "indexes")
	return ok && strings.TrimSpace(rest) == "{"
}

// parseColumn parses `<name> <type> [<settings>]`. The column name may be
// double-quoted; the type may contain parentheses with spaces
// (`numeric(10, 2)`). An inline `ref:` setting appends to the schema's refs.
func parseColumn(sch *schema, tb *table, t string, ln int) (*column, error) {
	main, settings, err := splitSettings(t, ln)
	if err != nil {
		return nil, err
	}
	tokens := tokenize(main)
	if len(tokens) < 2 {
		return nil, errf(ln, "column %q is missing a type", main)
	}
	col := &column{name: unquote(tokens[0]), typ: strings.Join(tokens[1:], " ")}
	for _, item := range splitTopLevel(settings, ',') {
		item = strings.TrimSpace(item)
		switch lower := strings.ToLower(item); {
		case lower == "":
		case lower == "pk" || lower == "primary key":
			col.pk = true
		case lower == "not null":
			col.notNull = true
		case lower == "null":
		case lower == "unique":
			col.unique = true
		case lower == "increment":
			col.increment = true
		case hasKeyPrefix(item, "default"):
			col.def = unquote(valueAfterKey(item, "default"))
		case hasKeyPrefix(item, "note"):
			col.note = unquote(valueAfterKey(item, "note"))
		case hasKeyPrefix(item, "ref"):
			r, err := parseInlineRef(tb.name, col.name, valueAfterKey(item, "ref"), ln)
			if err != nil {
				return nil, err
			}
			sch.refs = append(sch.refs, r)
		default:
			// Unknown settings are ignored — lenient inside blocks.
		}
	}
	return col, nil
}

func parseIndexes(tb *table, tableName string, s *scanner) error {
	open := s.lineNo()
	for {
		line, ok := s.next()
		if !ok {
			return errf(open, "indexes block of Table %q is never closed", tableName)
		}
		t := strings.TrimSpace(stripComment(line))
		switch {
		case t == "":
		case t == "}":
			return nil
		default:
			main, settings, err := splitSettings(t, s.lineNo())
			if err != nil {
				return err
			}
			idx := &index{}
			main = strings.TrimSpace(main)
			if strings.HasPrefix(main, "(") && strings.HasSuffix(main, ")") {
				for _, c := range splitTopLevel(main[1:len(main)-1], ',') {
					idx.columns = append(idx.columns, unquote(strings.TrimSpace(c)))
				}
			} else {
				idx.columns = []string{unquote(main)}
			}
			for _, item := range splitTopLevel(settings, ',') {
				item = strings.TrimSpace(item)
				switch lower := strings.ToLower(item); {
				case lower == "":
				case lower == "pk":
					idx.pk = true
				case lower == "unique":
					idx.unique = true
				case hasKeyPrefix(item, "name"):
					idx.name = unquote(valueAfterKey(item, "name"))
				case hasKeyPrefix(item, "type"):
					idx.typ = unquote(valueAfterKey(item, "type"))
				case hasKeyPrefix(item, "note"):
					idx.note = unquote(valueAfterKey(item, "note"))
				}
			}
			tb.indexes = append(tb.indexes, idx)
		}
	}
}

func parseEnum(sch *schema, rest string, s *scanner) error {
	ln := s.lineNo()
	name, _, err := blockHeader("Enum", rest, ln)
	if err != nil {
		return err
	}
	e := &enum{name: name}
	for {
		line, ok := s.next()
		if !ok {
			return errf(ln, "Enum %q is never closed", name)
		}
		t := strings.TrimSpace(stripComment(line))
		switch {
		case t == "":
		case t == "}":
			sch.enums = append(sch.enums, e)
			return nil
		case hasKeyPrefix(t, "note"):
			if e.note, err = parseNoteValue(valueAfterKey(t, "note"), s); err != nil {
				return err
			}
		default:
			// Value settings (per-value notes, colors) are dropped in v1.
			main, _, err := splitSettings(t, s.lineNo())
			if err != nil {
				return err
			}
			tokens := tokenize(main)
			if len(tokens) != 1 {
				return errf(s.lineNo(), "malformed enum value %q in Enum %q", t, name)
			}
			e.values = append(e.values, unquote(tokens[0]))
		}
	}
}

func parseTableGroup(sch *schema, rest string, s *scanner) error {
	ln := s.lineNo()
	name, _, err := blockHeader("TableGroup", rest, ln)
	if err != nil {
		return err
	}
	g := &tableGroup{name: name}
	for {
		line, ok := s.next()
		if !ok {
			return errf(ln, "TableGroup %q is never closed", name)
		}
		t := strings.TrimSpace(stripComment(line))
		switch {
		case t == "":
		case t == "}":
			sch.groups = append(sch.groups, g)
			return nil
		case hasKeyPrefix(t, "note"):
			if g.note, err = parseNoteValue(valueAfterKey(t, "note"), s); err != nil {
				return err
			}
		default:
			for _, tok := range tokenize(t) {
				g.tables = append(g.tables, unquote(tok))
			}
		}
	}
}

// parseRef handles the single-line form (`Ref[ name]: a.b > c.d [settings]`)
// and the block form (`Ref[ name] { ... }` with one relationship per line).
func parseRef(sch *schema, rest string, s *scanner) error {
	ln := s.lineNo()
	if body, ok := strings.CutSuffix(rest, "{"); ok && !strings.Contains(rest, ":") {
		name := unquote(strings.TrimSpace(body))
		open := ln
		for {
			line, more := s.next()
			if !more {
				return errf(open, "Ref block is never closed")
			}
			t := strings.TrimSpace(stripComment(line))
			switch {
			case t == "":
			case t == "}":
				return nil
			default:
				r, err := parseRefBody(name, t, s.lineNo())
				if err != nil {
					return err
				}
				sch.refs = append(sch.refs, r)
			}
		}
	}
	name, body, ok := strings.Cut(rest, ":")
	if !ok {
		return errf(ln, "malformed Ref: expected ':' or '{' after Ref")
	}
	r, err := parseRefBody(unquote(strings.TrimSpace(name)), strings.TrimSpace(body), ln)
	if err != nil {
		return err
	}
	sch.refs = append(sch.refs, r)
	return nil
}

func parseRefBody(name, body string, ln int) (*ref, error) {
	main, settings, err := splitSettings(body, ln)
	if err != nil {
		return nil, err
	}
	opIdx, op := findRelOp(main)
	if opIdx < 0 {
		return nil, errf(ln, "ref %q is missing a relationship operator (<, >, -, <>)", main)
	}
	r := &ref{name: name, relation: relationName(op)}
	if r.fromTable, r.fromColumn, err = parseEndpoint(strings.TrimSpace(main[:opIdx]), ln); err != nil {
		return nil, err
	}
	if r.toTable, r.toColumn, err = parseEndpoint(strings.TrimSpace(main[opIdx+len(op):]), ln); err != nil {
		return nil, err
	}
	for _, item := range splitTopLevel(settings, ',') {
		item = strings.TrimSpace(item)
		switch {
		case hasKeyPrefix(item, "delete"):
			r.onDelete = unquote(valueAfterKey(item, "delete"))
		case hasKeyPrefix(item, "update"):
			r.onUpdate = unquote(valueAfterKey(item, "update"))
		}
	}
	return r, nil
}

func parseInlineRef(tableName, colName, body string, ln int) (*ref, error) {
	body = strings.TrimSpace(body)
	opIdx, op := findRelOp(body)
	if opIdx != 0 {
		return nil, errf(ln, "inline ref %q must start with a relationship operator (<, >, -, <>)", body)
	}
	toTable, toColumn, err := parseEndpoint(strings.TrimSpace(body[len(op):]), ln)
	if err != nil {
		return nil, err
	}
	return &ref{
		fromTable:  tableName,
		fromColumn: colName,
		toTable:    toTable,
		toColumn:   toColumn,
		relation:   relationName(op),
	}, nil
}

// parseEndpoint splits `table.column`, `schema.table.column` (the column is
// the last segment), or a composite `table.(a, b)` whose columns join as
// "a, b".
func parseEndpoint(s string, ln int) (table, column string, err error) {
	if i := indexTopLevel(s, '('); i >= 0 {
		if !strings.HasSuffix(s, ")") {
			return "", "", errf(ln, "malformed composite ref endpoint %q", s)
		}
		var cols []string
		for _, c := range splitTopLevel(s[i+1:len(s)-1], ',') {
			cols = append(cols, unquote(strings.TrimSpace(c)))
		}
		return unquote(strings.TrimSuffix(strings.TrimSpace(s[:i]), ".")), strings.Join(cols, ", "), nil
	}
	i := lastIndexTopLevel(s, '.')
	if i < 0 {
		return "", "", errf(ln, "ref endpoint %q must be table.column", s)
	}
	return unquote(strings.TrimSpace(s[:i])), unquote(strings.TrimSpace(s[i+1:])), nil
}

// findRelOp locates the relationship operator outside quotes and parentheses.
// "<>" is checked before "<" and ">".
func findRelOp(s string) (int, string) {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
		case '<':
			if depth == 0 {
				if i+1 < len(s) && s[i+1] == '>' {
					return i, "<>"
				}
				return i, "<"
			}
		case '>', '-':
			if depth == 0 {
				return i, string(c)
			}
		}
	}
	return -1, ""
}

// relationName spells the operator out relative to the left endpoint.
func relationName(op string) string {
	switch op {
	case ">":
		return "many-to-one"
	case "<":
		return "one-to-many"
	case "<>":
		return "many-to-many"
	default:
		return "one-to-one"
	}
}

// --- low-level line helpers ---------------------------------------------------

// stripComment cuts a trailing // comment, respecting single, double, and
// backtick quotes.
func stripComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
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
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return line[:i]
			}
		}
	}
	return line
}

// hasKeyPrefix reports whether t is a `key: value` statement for key
// (case-insensitive).
func hasKeyPrefix(t, key string) bool {
	if len(t) <= len(key) || !strings.EqualFold(t[:len(key)], key) {
		return false
	}
	rest := strings.TrimSpace(t[len(key):])
	return rest != "" && rest[0] == ':'
}

// valueAfterKey returns the value part of a `key: value` statement.
func valueAfterKey(t, key string) string {
	rest := strings.TrimSpace(t[len(key):])
	return strings.TrimSpace(strings.TrimPrefix(rest, ":"))
}

// parseNoteValue reads a note value: 'single line', a same-line '''block''',
// or a ''' block continuing on subsequent raw lines until the closing '''.
// Block lines are whitespace-trimmed and joined with newlines.
func parseNoteValue(v string, s *scanner) (string, error) {
	open := s.lineNo()
	if rest, ok := strings.CutPrefix(v, "'''"); ok {
		if inner, closed := strings.CutSuffix(rest, "'''"); closed && rest != "" {
			return strings.TrimSpace(inner), nil
		}
		var lines []string
		if t := strings.TrimSpace(rest); t != "" {
			lines = append(lines, t)
		}
		for {
			line, more := s.next()
			if !more {
				return "", errf(open, "note is never closed (missing ''')")
			}
			if body, _, closed := strings.Cut(line, "'''"); closed {
				if t := strings.TrimSpace(body); t != "" {
					lines = append(lines, t)
				}
				return strings.Join(lines, "\n"), nil
			}
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return unquote(v), nil
}

// splitSettings splits a line into its main part and the contents of a
// trailing [settings] block, if any.
func splitSettings(t string, ln int) (main, settings string, err error) {
	i := indexTopLevel(t, '[')
	if i < 0 {
		return strings.TrimSpace(t), "", nil
	}
	rest := strings.TrimSpace(t[i:])
	if !strings.HasSuffix(rest, "]") {
		return "", "", errf(ln, "unterminated settings block in %q", t)
	}
	return strings.TrimSpace(t[:i]), rest[1 : len(rest)-1], nil
}

// indexTopLevel finds the first index of ch outside quotes and parentheses.
func indexTopLevel(s string, ch byte) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
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
		case '(':
			depth++
		case ')':
			depth--
		case ch:
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// lastIndexTopLevel finds the last index of ch outside quotes and parentheses.
func lastIndexTopLevel(s string, ch byte) int {
	last := -1
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
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
		case '(':
			depth++
		case ')':
			depth--
		case ch:
			if depth == 0 {
				last = i
			}
		}
	}
	return last
}

// splitTopLevel splits s at sep outside quotes and parentheses.
func splitTopLevel(s string, sep byte) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var parts []string
	start := 0
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
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
		case '(':
			depth++
		case ')':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// tokenize splits s on whitespace, keeping quoted spans (single, double, or
// backtick) as one token including their quotes; unquote strips them.
func tokenize(s string) []string {
	var tokens []string
	var b strings.Builder
	var quote byte
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			b.WriteByte(c)
		case ' ', '\t':
			flush()
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return tokens
}

// unquote strips one matching pair of surrounding single, double, or backtick
// quotes and unescapes backslash escapes inside single quotes.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}
	q := s[0]
	if (q != '\'' && q != '"' && q != '`') || s[len(s)-1] != q {
		return s
	}
	inner := s[1 : len(s)-1]
	if q == '\'' {
		inner = strings.ReplaceAll(inner, `\'`, `'`)
	}
	return inner
}

// --- emitting the page tree ---------------------------------------------------

func emit(sch *schema) (*core.PageTree, error) {
	root := &core.PageTree{
		Envelope: core.Envelope{"layout": Layout},
		Children: map[string]*core.PageTree{},
	}
	if p := sch.project; p != nil {
		if p.name != "" {
			root.Envelope["title"] = p.name
		}
		if p.databaseType != "" {
			root.Envelope["database_type"] = p.databaseType
		}
		if s := summaryLine(p.note); s != "" {
			root.Envelope["summary"] = s
		}
		root.Nodes = append(root.Nodes, core.Node{
			Type:    NodeProject,
			Content: p.note,
			Attributes: map[string]any{
				"name":          p.name,
				"database_type": p.databaseType,
			},
		})
	}

	// Aliases resolve to table names everywhere refs are emitted, so node
	// attributes always carry the stable identifiers.
	aliases := map[string]string{}
	for _, t := range sch.tables {
		if t.alias != "" {
			aliases[t.alias] = t.name
		}
	}
	resolve := func(name string) string {
		if canonical, ok := aliases[name]; ok {
			return canonical
		}
		return name
	}
	for _, r := range sch.refs {
		r.fromTable = resolve(r.fromTable)
		r.toTable = resolve(r.toTable)
	}

	for _, t := range sch.tables {
		slugPath := "tables/" + formatkit.Slugify(t.name)
		if _, dup := root.Children[slugPath]; dup {
			return nil, fmt.Errorf("dbml: duplicate table slug %q (table %q)", slugPath, t.name)
		}
		root.Children[slugPath] = tablePage(t, sch.refs)
		root.Nodes = append(root.Nodes, core.Node{
			Type: NodeTableRef,
			Attributes: map[string]any{
				"name":    t.name,
				"alias":   t.alias,
				"slug":    slugPath,
				"summary": summaryLine(t.note),
			},
		})
	}

	for _, e := range sch.enums {
		values := make([]any, len(e.values))
		for i, v := range e.values {
			values[i] = v
		}
		root.Nodes = append(root.Nodes, core.Node{
			Type:    NodeEnum,
			Content: e.note,
			Attributes: map[string]any{
				"name":   e.name,
				"values": values,
			},
		})
	}

	for _, g := range sch.groups {
		tables := make([]any, len(g.tables))
		for i, n := range g.tables {
			tables[i] = resolve(n)
		}
		root.Nodes = append(root.Nodes, core.Node{
			Type:    NodeTableGroup,
			Content: g.note,
			Attributes: map[string]any{
				"name":   g.name,
				"tables": tables,
			},
		})
	}

	return root, nil
}

func tablePage(t *table, refs []*ref) *core.PageTree {
	nodes := []core.Node{{
		Type:    NodeTable,
		Content: t.note,
		Attributes: map[string]any{
			"name":  t.name,
			"alias": t.alias,
		},
	}}
	for _, c := range t.columns {
		nodes = append(nodes, core.Node{
			Type:    NodeColumn,
			Content: c.note,
			Attributes: map[string]any{
				"name":      c.name,
				"type":      c.typ,
				"pk":        c.pk,
				"not_null":  c.notNull,
				"unique":    c.unique,
				"increment": c.increment,
				"default":   c.def,
			},
		})
	}
	for _, idx := range t.indexes {
		columns := make([]any, len(idx.columns))
		for i, c := range idx.columns {
			columns[i] = c
		}
		nodes = append(nodes, core.Node{
			Type:    NodeIndex,
			Content: idx.note,
			Attributes: map[string]any{
				"columns": columns,
				"name":    idx.name,
				"type":    idx.typ,
				"pk":      idx.pk,
				"unique":  idx.unique,
			},
		})
	}
	for _, r := range refs {
		if r.fromTable != t.name && r.toTable != t.name {
			continue
		}
		nodes = append(nodes, core.Node{
			Type: NodeRef,
			Attributes: map[string]any{
				"name":        r.name,
				"from_table":  r.fromTable,
				"from_column": r.fromColumn,
				"to_table":    r.toTable,
				"to_column":   r.toColumn,
				"relation":    r.relation,
				"on_delete":   r.onDelete,
				"on_update":   r.onUpdate,
			},
		})
	}
	return &core.PageTree{
		Envelope: core.Envelope{"title": t.name, "layout": LayoutTable},
		Nodes:    nodes,
	}
}

// summaryLine returns the first non-empty line of a note, for the envelope
// summary key.
func summaryLine(note string) string {
	for _, line := range strings.Split(note, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			return l
		}
	}
	return ""
}
