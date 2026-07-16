package openapi_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofuego/fuego/engine"

	"github.com/gofuego/fuego-formats/formatkit"
	"github.com/gofuego/fuego-formats/openapi"
)

// demoSite writes a small site whose content includes the billing spec, a
// plain .yaml file claimed by a declarative parser, and a tags taxonomy, then
// returns the site dir. It is the acceptance-criteria stage: specificity
// dispatch, taxonomy visibility, and the manifest contract all get proven on
// a real engine build.
func demoSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	write := func(path, body string) {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	spec, err := os.ReadFile(filepath.Join("testdata", "billing.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	write("content/api.openapi.yaml", string(spec))
	// A plain .yaml file: the declarative parser's bare-extension claim must
	// keep it, while the spec's *.openapi.yaml pattern outranks it.
	write("content/notes.yaml", "just: notes\n")

	write("config.yaml", `site:
  name: "OpenAPI Demo"

parsers:
  yaml:
    rules:
      - match: '^(.+)$'
        emit:
          type: yaml-line
          content: "$1"

taxonomies:
  tags:
    path: "/by-tag/{term}"
    layout: "tag"
`)
	write("theme/base.html", `<!DOCTYPE html>
<html><head><title>{{.Page.Envelope.title}}</title></head>
<body>{{template "content" .}}</body></html>
{{define "content"}}{{.Page.Content}}{{end}}`)
	write("theme/layouts/tag.html", `{{define "content"}}<h1>Tag: {{.Page.Envelope.term}}</h1>{{.Page.Content}}{{end}}`)

	return dir
}

func TestDemoSiteBuild(t *testing.T) {
	dir := demoSite(t)
	out := filepath.Join(dir, "out")

	eng := engine.New()
	eng.Register(openapi.Parser())

	if err := eng.Build(context.Background(), engine.BuildOptions{
		ConfigPath: filepath.Join(dir, "config.yaml"),
		ContentDir: filepath.Join(dir, "content"),
		ThemeDir:   filepath.Join(dir, "theme"),
		OutputDir:  out,
		SiteName:   "OpenAPI Demo",
	}); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// The spec expanded into a routed section: root + tag + operation pages.
	for _, page := range []string{
		"api.openapi/index.html",
		"api.openapi/tags/invoices/index.html",
		"api.openapi/tags/invoices/list-invoices/index.html",
		"api.openapi/schemas/invoice/index.html",
		"api.openapi/operations/get-healthz/index.html",
	} {
		if _, err := os.Stat(filepath.Join(out, page)); err != nil {
			t.Errorf("missing expected page %s: %v", page, err)
		}
	}

	// Specificity dispatch: notes.yaml went to the declarative yaml parser
	// (it becomes a routed page, not an asset copy and not a spec).
	notes, err := os.ReadFile(filepath.Join(out, "notes", "index.html"))
	if err != nil {
		t.Fatalf("notes.yaml not routed by the declarative parser: %v", err)
	}
	if !strings.Contains(string(notes), "just: notes") {
		t.Errorf("declarative parser output wrong: %s", notes)
	}

	// Taxonomy visibility: operation pages carry envelope tags, so the
	// Invoices term page must exist and link/render its member operations.
	term, err := os.ReadFile(filepath.Join(out, "by-tag", "invoices", "index.html"))
	if err != nil {
		t.Fatalf("taxonomy term page missing — operation envelopes not scanned: %v", err)
	}
	if !strings.Contains(string(term), "Tag: invoices") {
		t.Errorf("taxonomy term page content wrong:\n%s", term)
	}

	// Manifest contract: every page of the tree lists the spec as its
	// source_path (multi-entry per source) — editable-as-the-spec in studio.
	var manifest struct {
		Pages []struct {
			URL        string `json:"url"`
			SourcePath string `json:"source_path"`
		} `json:"pages"`
	}
	raw, err := os.ReadFile(filepath.Join(out, "site-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	specPages := 0
	for _, p := range manifest.Pages {
		if strings.HasPrefix(p.URL, "/api.openapi/") || p.URL == "/api.openapi" {
			specPages++
			if p.SourcePath != "api.openapi.yaml" {
				t.Errorf("page %s source_path = %q, want api.openapi.yaml", p.URL, p.SourcePath)
			}
		}
	}
	if specPages < 10 {
		t.Errorf("expected the spec's tree in the manifest, found %d spec pages", specPages)
	}
}

// TestIncrementalRebuildIsByteIdentical builds the demo site clean, then twice
// incrementally with nothing changed. The engine's parity guarantee extends to
// tree parsers (fuego ADR-019): an unchanged spec restores its whole tree from
// the cache and the output stays byte-identical.
func TestIncrementalRebuildIsByteIdentical(t *testing.T) {
	dir := demoSite(t)
	clean := filepath.Join(dir, "out-clean")
	incr := filepath.Join(dir, "out-incr")

	build := func(out string, incremental bool) {
		t.Helper()
		eng := engine.New()
		eng.Register(openapi.Parser())
		if err := eng.Build(context.Background(), engine.BuildOptions{
			ConfigPath:  filepath.Join(dir, "config.yaml"),
			ContentDir:  filepath.Join(dir, "content"),
			ThemeDir:    filepath.Join(dir, "theme"),
			OutputDir:   out,
			SiteName:    "OpenAPI Demo",
			Incremental: incremental,
			CacheDir:    filepath.Join(dir, ".fuego"),
		}); err != nil {
			t.Fatalf("build failed: %v", err)
		}
	}

	build(clean, false)
	build(incr, true) // cold cache
	build(incr, true) // warm cache: the spec's whole tree restores from cache

	err := filepath.Walk(clean, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(clean, path)
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(incr, rel))
		if err != nil {
			t.Errorf("missing in incremental output: %s", rel)
			return nil
		}
		if string(want) != string(got) {
			t.Errorf("byte mismatch after warm-cache rebuild: %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWithPatternsClaimsBrownfieldName proves the WithPatterns escape hatch on
// a real build: specs named *.api.yaml route to the parser.
func TestWithPatternsClaimsBrownfieldName(t *testing.T) {
	dir := t.TempDir()
	spec, err := os.ReadFile(filepath.Join("testdata", "billing.openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "content", "billing.api.yaml"), spec, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "theme"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := "<!DOCTYPE html><html><body>{{template \"content\" .}}</body></html>\n{{define \"content\"}}{{.Page.Content}}{{end}}"
	if err := os.WriteFile(filepath.Join(dir, "theme", "base.html"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	eng := engine.New()
	eng.Register(openapi.Parser(formatkit.WithPatterns("*.api.yaml")))
	if err := eng.Build(context.Background(), engine.BuildOptions{
		ContentDir: filepath.Join(dir, "content"),
		ThemeDir:   filepath.Join(dir, "theme"),
		OutputDir:  filepath.Join(dir, "out"),
		SiteName:   "Brownfield",
	}); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "billing.api", "tags", "invoices", "index.html")); err != nil {
		t.Errorf("*.api.yaml spec not expanded: %v", err)
	}
}
