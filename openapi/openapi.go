// Package openapi is a fuego-formats TreeParser for OpenAPI 3.x specs.
//
// One spec file becomes a whole section of the site: a routed root page (the
// API index) plus a tree of real pages — one per tag, one per operation under
// each of its tags, and one per component schema. Children carry their own
// envelopes (method, path, tags), so site taxonomies, collections, and
// pagination see them natively; the manifest lists every page against the
// spec file, so fuego-studio treats each as editable-as-the-spec.
//
// Usage:
//
//	eng := engine.New()
//	eng.Register(openapi.Parser())
//
// Override the default claims for a brownfield repo:
//
//	eng.Register(openapi.Parser(formatkit.WithPatterns("*.api.yaml")))
//
// The emitted node types, envelope keys, tree shape, and slug rules are the
// module's contract — see schema.md.
package openapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
)

// Type is the parser's Type() — the format slug and every page's type.
const Type = "openapi"

// Node types emitted by the parser. All are prefixed with the format slug so a
// theme's renderer templates never collide with another format's.
const (
	// NodeInfo is the root page's overview node: the spec's info block.
	// Content is the API description; attributes carry title and version.
	NodeInfo = "openapi-info"
	// NodeServer is one server entry on the root page (attrs: url, description).
	NodeServer = "openapi-server"
	// NodeTagRef is a link node on the root page pointing at a tag page
	// (attrs: name, slug, description). slug is the child's slug path relative
	// to the root page's URL.
	NodeTagRef = "openapi-tag-ref"
	// NodeSchemaRef is a link node on the root page pointing at a schema page
	// (attrs: name, slug).
	NodeSchemaRef = "openapi-schema-ref"
	// NodeOperationRef is a link node on the root and tag pages pointing at an
	// operation page (attrs: method, path, slug, summary, deprecated).
	NodeOperationRef = "openapi-operation-ref"
	// NodeOperation is an operation page's main node. Content is the operation
	// description; attributes carry method, path, summary, operation_id,
	// deprecated.
	NodeOperation = "openapi-operation"
	// NodeParameter is one parameter of an operation (attrs: name, in,
	// required, type; content is the parameter description).
	NodeParameter = "openapi-parameter"
	// NodeRequestBody is an operation's request body (attrs: required,
	// content_types; content is the body description).
	NodeRequestBody = "openapi-request-body"
	// NodeResponse is one response of an operation, emitted in ascending
	// status order (attrs: status, content_types; content is the description).
	NodeResponse = "openapi-response"
	// NodeSchema is a schema page's main node (attrs: name, type; content is
	// the schema description).
	NodeSchema = "openapi-schema"
	// NodeProperty is one property of an object schema, in sorted name order
	// (attrs: name, type, required; content is the property description).
	NodeProperty = "openapi-property"
)

// Default layout names set in the envelopes. A theme provides
// theme/layouts/openapi.html (and the per-kind variants) to style the pages;
// absent that, the engine falls back to the base template silently.
const (
	Layout          = "openapi"
	LayoutTag       = "openapi-tag"
	LayoutOperation = "openapi-operation"
	LayoutSchema    = "openapi-schema"
)

// DefaultPatterns are the built-in filename claims: the compound suffixes plus
// the well-known literal spec names. Claims match base names only.
var DefaultPatterns = []string{
	"*.openapi.yaml", "*.openapi.json",
	"openapi.yaml", "openapi.json",
	"swagger.yaml", "swagger.json",
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

// parseTree loads an OpenAPI 3.x document and expands it into the page tree
// documented in schema.md. Load errors (bad YAML/JSON, unresolvable refs)
// return an error, which the engine turns into a LocalFatal for this file —
// the rest of the site still builds. The document is deliberately NOT run
// through full spec validation: an imperfect spec still renders.
func parseTree(raw []byte) (*core.PageTree, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(raw)
	if err != nil {
		return nil, fmt.Errorf("openapi: loading spec: %w", err)
	}

	root := &core.PageTree{
		Envelope: rootEnvelope(doc),
		Children: map[string]*core.PageTree{},
	}
	root.Nodes = append(root.Nodes, infoNode(doc))
	for _, srv := range doc.Servers {
		root.Nodes = append(root.Nodes, core.Node{
			Type: NodeServer,
			Attributes: map[string]any{
				"url":         srv.URL,
				"description": srv.Description,
			},
		})
	}

	if err := addOperationPages(doc, root); err != nil {
		return nil, err
	}
	addSchemaPages(doc, root)

	return root, nil
}

func rootEnvelope(doc *openapi3.T) core.Envelope {
	env := core.Envelope{"layout": Layout}
	if doc.Info != nil {
		if doc.Info.Title != "" {
			env["title"] = doc.Info.Title
		}
		if doc.Info.Version != "" {
			env["version"] = doc.Info.Version
		}
		if s := summaryLine(doc.Info.Description); s != "" {
			env["summary"] = s
		}
	}
	return env
}

func infoNode(doc *openapi3.T) core.Node {
	attrs := map[string]any{}
	desc := ""
	if doc.Info != nil {
		attrs["title"] = doc.Info.Title
		attrs["version"] = doc.Info.Version
		desc = doc.Info.Description
	}
	return core.Node{Type: NodeInfo, Content: desc, Attributes: attrs}
}

// operation is one operation flattened out of the paths map, pre-slugged.
type operation struct {
	method string
	path   string
	op     *openapi3.Operation
	slug   string
}

// addOperationPages walks the paths in sorted order and inserts one page per
// (tag, operation) pair under tags/<tag>/<op>, and untagged operations under
// operations/<op>. Tag pages carry ref nodes to their operations; the root
// gets ref nodes to every tag page and to untagged operations.
func addOperationPages(doc *openapi3.T, root *core.PageTree) error {
	ops := collectOperations(doc)

	// Declared tag order first (spec authors order tags deliberately), then
	// any tags only referenced by operations, alphabetically.
	tagDesc := map[string]string{}
	var tagOrder []string
	seen := map[string]bool{}
	for _, t := range doc.Tags {
		tagOrder = append(tagOrder, t.Name)
		tagDesc[t.Name] = t.Description
		seen[t.Name] = true
	}
	var extra []string
	for _, o := range ops {
		for _, t := range o.op.Tags {
			if !seen[t] {
				seen[t] = true
				extra = append(extra, t)
			}
		}
	}
	sort.Strings(extra)
	tagOrder = append(tagOrder, extra...)

	for _, tag := range tagOrder {
		tagSlug := slugify(tag)
		tagPath := "tags/" + tagSlug
		tagPage := &core.PageTree{
			Envelope: core.Envelope{"title": tag, "layout": LayoutTag},
			Children: map[string]*core.PageTree{},
		}
		if d := tagDesc[tag]; d != "" {
			tagPage.Envelope["description"] = d
		}
		for _, o := range ops {
			if !hasTag(o.op.Tags, tag) {
				continue
			}
			slugPath := tagPath + "/" + o.slug
			if _, dup := tagPage.Children[o.slug]; dup {
				return fmt.Errorf("openapi: duplicate operation slug %q under tag %q (%s %s)", o.slug, tag, o.method, o.path)
			}
			tagPage.Children[o.slug] = operationPage(o)
			tagPage.Nodes = append(tagPage.Nodes, operationRef(o, slugPath))
		}
		if _, dup := root.Children[tagPath]; dup {
			return fmt.Errorf("openapi: duplicate tag slug %q", tagSlug)
		}
		root.Children[tagPath] = tagPage
		root.Nodes = append(root.Nodes, core.Node{
			Type: NodeTagRef,
			Attributes: map[string]any{
				"name":        tag,
				"slug":        tagPath,
				"description": tagDesc[tag],
			},
		})
	}

	for _, o := range ops {
		if len(o.op.Tags) > 0 {
			continue
		}
		slugPath := "operations/" + o.slug
		if _, dup := root.Children[slugPath]; dup {
			return fmt.Errorf("openapi: duplicate operation slug %q (%s %s)", o.slug, o.method, o.path)
		}
		root.Children[slugPath] = operationPage(o)
		root.Nodes = append(root.Nodes, operationRef(o, slugPath))
	}
	return nil
}

// collectOperations flattens doc.Paths into a deterministic list: paths in
// sorted order, methods in sorted order within a path.
func collectOperations(doc *openapi3.T) []operation {
	var out []operation
	if doc.Paths == nil {
		return out
	}
	paths := doc.Paths.Map()
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)
	for _, p := range pathKeys {
		item := paths[p]
		byMethod := item.Operations()
		methods := make([]string, 0, len(byMethod))
		for m := range byMethod {
			methods = append(methods, m)
		}
		sort.Strings(methods)
		for _, m := range methods {
			op := byMethod[m]
			out = append(out, operation{
				method: m,
				path:   p,
				op:     op,
				slug:   operationSlug(m, p, op),
			})
		}
	}
	return out
}

// operationSlug is a stability promise (see schema.md): the slugified
// operationId when one is set, else the slugified method + path.
func operationSlug(method, path string, op *openapi3.Operation) string {
	if op.OperationID != "" {
		return slugify(op.OperationID)
	}
	return slugify(method + " " + path)
}

func operationPage(o operation) *core.PageTree {
	env := core.Envelope{
		"title":  operationTitle(o),
		"layout": LayoutOperation,
		"method": o.method,
		"path":   o.path,
	}
	if len(o.op.Tags) > 0 {
		tags := make([]any, len(o.op.Tags))
		for i, t := range o.op.Tags {
			tags[i] = t
		}
		env["tags"] = tags
	}
	if o.op.OperationID != "" {
		env["operation_id"] = o.op.OperationID
	}
	if o.op.Deprecated {
		env["deprecated"] = true
	}

	nodes := []core.Node{{
		Type:    NodeOperation,
		Content: o.op.Description,
		Attributes: map[string]any{
			"method":       o.method,
			"path":         o.path,
			"summary":      o.op.Summary,
			"operation_id": o.op.OperationID,
			"deprecated":   o.op.Deprecated,
		},
	}}
	for _, pref := range o.op.Parameters {
		if pref.Value == nil {
			continue
		}
		p := pref.Value
		nodes = append(nodes, core.Node{
			Type:    NodeParameter,
			Content: p.Description,
			Attributes: map[string]any{
				"name":     p.Name,
				"in":       p.In,
				"required": p.Required,
				"type":     schemaTypeString(p.Schema),
			},
		})
	}
	if o.op.RequestBody != nil && o.op.RequestBody.Value != nil {
		rb := o.op.RequestBody.Value
		nodes = append(nodes, core.Node{
			Type:    NodeRequestBody,
			Content: rb.Description,
			Attributes: map[string]any{
				"required":      rb.Required,
				"content_types": contentTypes(rb.Content),
			},
		})
	}
	if o.op.Responses != nil {
		resps := o.op.Responses.Map()
		statuses := make([]string, 0, len(resps))
		for s := range resps {
			statuses = append(statuses, s)
		}
		sort.Strings(statuses)
		for _, s := range statuses {
			r := resps[s]
			if r.Value == nil {
				continue
			}
			desc := ""
			if r.Value.Description != nil {
				desc = *r.Value.Description
			}
			nodes = append(nodes, core.Node{
				Type:    NodeResponse,
				Content: desc,
				Attributes: map[string]any{
					"status":        s,
					"content_types": contentTypes(r.Value.Content),
				},
			})
		}
	}
	return &core.PageTree{Envelope: env, Nodes: nodes}
}

func operationTitle(o operation) string {
	if o.op.Summary != "" {
		return o.op.Summary
	}
	if o.op.OperationID != "" {
		return o.op.OperationID
	}
	return o.method + " " + o.path
}

func operationRef(o operation, slugPath string) core.Node {
	return core.Node{
		Type: NodeOperationRef,
		Attributes: map[string]any{
			"method":     o.method,
			"path":       o.path,
			"slug":       slugPath,
			"summary":    o.op.Summary,
			"deprecated": o.op.Deprecated,
		},
	}
}

// addSchemaPages inserts one page per component schema under
// schemas/<name>, in sorted name order, plus ref nodes on the root.
func addSchemaPages(doc *openapi3.T, root *core.PageTree) {
	if doc.Components == nil || len(doc.Components.Schemas) == 0 {
		return
	}
	names := make([]string, 0, len(doc.Components.Schemas))
	for n := range doc.Components.Schemas {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		ref := doc.Components.Schemas[name]
		slugPath := "schemas/" + slugify(name)
		root.Children[slugPath] = schemaPage(name, ref)
		root.Nodes = append(root.Nodes, core.Node{
			Type:       NodeSchemaRef,
			Attributes: map[string]any{"name": name, "slug": slugPath},
		})
	}
}

func schemaPage(name string, ref *openapi3.SchemaRef) *core.PageTree {
	env := core.Envelope{"title": name, "layout": LayoutSchema}
	nodes := []core.Node{}
	if ref.Value != nil {
		s := ref.Value
		env["type"] = schemaTypeString(ref)
		nodes = append(nodes, core.Node{
			Type:    NodeSchema,
			Content: s.Description,
			Attributes: map[string]any{
				"name": name,
				"type": schemaTypeString(ref),
			},
		})
		required := map[string]bool{}
		for _, r := range s.Required {
			required[r] = true
		}
		props := make([]string, 0, len(s.Properties))
		for p := range s.Properties {
			props = append(props, p)
		}
		sort.Strings(props)
		for _, p := range props {
			pv := s.Properties[p]
			desc := ""
			if pv.Value != nil {
				desc = pv.Value.Description
			}
			nodes = append(nodes, core.Node{
				Type:    NodeProperty,
				Content: desc,
				Attributes: map[string]any{
					"name":     p,
					"type":     schemaTypeString(pv),
					"required": required[p],
				},
			})
		}
	}
	return &core.PageTree{Envelope: env, Nodes: nodes}
}

// schemaTypeString renders a schema reference as a short human-readable type:
// a referenced component's name, "array of X", or the primitive type with its
// format ("string (date-time)").
func schemaTypeString(ref *openapi3.SchemaRef) string {
	if ref == nil {
		return ""
	}
	if name := refName(ref.Ref); name != "" {
		return name
	}
	s := ref.Value
	if s == nil {
		return ""
	}
	if s.Type != nil && s.Type.Is(openapi3.TypeArray) {
		if inner := schemaTypeString(s.Items); inner != "" {
			return "array of " + inner
		}
		return "array"
	}
	t := ""
	if s.Type != nil && len(*s.Type) > 0 {
		t = strings.Join(*s.Type, "|")
	}
	if t != "" && s.Format != "" {
		return t + " (" + s.Format + ")"
	}
	return t
}

// refName extracts the component name from a $ref like
// "#/components/schemas/Pet".
func refName(ref string) string {
	if ref == "" {
		return ""
	}
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func contentTypes(c openapi3.Content) []any {
	types := make([]string, 0, len(c))
	for ct := range c {
		types = append(types, ct)
	}
	sort.Strings(types)
	out := make([]any, len(types))
	for i, t := range types {
		out[i] = t
	}
	return out
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// summaryLine returns the first non-empty line of a description, for the
// envelope summary key.
func summaryLine(desc string) string {
	for _, line := range strings.Split(desc, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			return l
		}
	}
	return ""
}

// slugify lowercases s, turns camelCase boundaries into hyphens
// ("listInvoices" → "list-invoices"), and collapses every run of characters
// outside [a-z0-9] into a single hyphen — path parameters lose their braces
// ("/pets/{petId}" → "pets-pet-id"). The result is a slug-derivation
// stability promise; see schema.md.
func slugify(s string) string {
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
