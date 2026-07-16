package dbml_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofuego/fuego/engine"

	"github.com/gofuego/fuego-formats/dbml"
)

// demoSite writes a small site whose content includes the inventory schema,
// then returns the site dir. It is the acceptance-criteria stage: tree
// expansion, the manifest contract, and incremental parity all get proven on
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

	schema, err := os.ReadFile(filepath.Join("testdata", "inventory.dbml"))
	if err != nil {
		t.Fatal(err)
	}
	write("content/inventory.dbml", string(schema))

	write("theme/base.html", `<!DOCTYPE html>
<html><head><title>{{.Page.Envelope.title}}</title></head>
<body>{{template "content" .}}</body></html>
{{define "content"}}{{.Page.Content}}{{end}}`)

	return dir
}

func build(t *testing.T, dir, out string, incremental bool) error {
	t.Helper()
	eng := engine.New()
	eng.Register(dbml.Parser())
	return eng.Build(context.Background(), engine.BuildOptions{
		ContentDir:  filepath.Join(dir, "content"),
		ThemeDir:    filepath.Join(dir, "theme"),
		OutputDir:   out,
		SiteName:    "DBML Demo",
		Incremental: incremental,
		CacheDir:    filepath.Join(dir, ".fuego"),
	})
}

func TestDemoSiteBuild(t *testing.T) {
	dir := demoSite(t)
	out := filepath.Join(dir, "out")

	eng := engine.New()
	eng.Register(dbml.Parser())
	if err := eng.Build(context.Background(), engine.BuildOptions{
		ContentDir: filepath.Join(dir, "content"),
		ThemeDir:   filepath.Join(dir, "theme"),
		OutputDir:  out,
		SiteName:   "DBML Demo",
	}); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// The schema expanded into a routed section: root + one page per table.
	for _, page := range []string{
		"inventory/index.html",
		"inventory/tables/warehouses/index.html",
		"inventory/tables/stock-items/index.html",
		"inventory/tables/audits/index.html",
	} {
		if _, err := os.Stat(filepath.Join(out, page)); err != nil {
			t.Errorf("missing expected page %s: %v", page, err)
		}
	}

	// Manifest contract: every page of the tree lists the schema as its
	// source_path (multi-entry per source) — editable-as-the-schema in studio.
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
	schemaPages := 0
	for _, p := range manifest.Pages {
		if strings.HasPrefix(p.URL, "/inventory/") || p.URL == "/inventory" {
			schemaPages++
			if p.SourcePath != "inventory.dbml" {
				t.Errorf("page %s source_path = %q, want inventory.dbml", p.URL, p.SourcePath)
			}
		}
	}
	if schemaPages != 4 {
		t.Errorf("expected the schema's 4 pages in the manifest, found %d", schemaPages)
	}
}

// TestMalformedFileDegradesPerEngineConvention proves the engine convention
// for malformed input: the parse error is a LocalFatal attributed to that
// file — the pipeline finishes and the reported failure names the broken
// file, not the healthy one.
func TestMalformedFileDegradesPerEngineConvention(t *testing.T) {
	dir := demoSite(t)
	if err := os.WriteFile(filepath.Join(dir, "content", "broken.dbml"), []byte("Tabel users {\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := build(t, dir, filepath.Join(dir, "out"), false)
	if err == nil {
		t.Fatal("want the build to report the malformed file")
	}
	if !strings.Contains(err.Error(), "broken.dbml") || !strings.Contains(err.Error(), "dbml:") {
		t.Errorf("error not attributed to the malformed file: %v", err)
	}
	if strings.Contains(err.Error(), "inventory.dbml") {
		t.Errorf("healthy file dragged into the failure: %v", err)
	}
}

// TestIncrementalRebuildIsByteIdentical builds the demo site clean, then twice
// incrementally with nothing changed. The engine's parity guarantee extends to
// tree parsers (fuego ADR-019): an unchanged schema restores its whole tree
// from the cache and the output stays byte-identical.
func TestIncrementalRebuildIsByteIdentical(t *testing.T) {
	dir := demoSite(t)
	clean := filepath.Join(dir, "out-clean")
	incr := filepath.Join(dir, "out-incr")

	if err := build(t, dir, clean, false); err != nil {
		t.Fatalf("clean build failed: %v", err)
	}
	if err := build(t, dir, incr, true); err != nil { // cold cache
		t.Fatalf("cold incremental build failed: %v", err)
	}
	if err := build(t, dir, incr, true); err != nil { // warm cache
		t.Fatalf("warm incremental build failed: %v", err)
	}

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
