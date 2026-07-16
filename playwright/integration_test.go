package playwright_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofuego/fuego/engine"

	"github.com/gofuego/fuego-formats/playwright"
)

// demoSite writes a small site whose content includes the checkout spec and a
// tags taxonomy, then returns the site dir. It is the acceptance-criteria
// stage: the tag envelope keys must drive a real "browse tests by tag"
// taxonomy on a real engine build.
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

	spec, err := os.ReadFile(filepath.Join("testdata", "checkout.spec.ts"))
	if err != nil {
		t.Fatal(err)
	}
	write("content/checkout.spec.ts", string(spec))

	write("config.yaml", `site:
  name: "Playwright Demo"

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

func build(t *testing.T, dir, out string, incremental bool) {
	t.Helper()
	eng := engine.New()
	eng.Register(playwright.Parser())
	if err := eng.Build(context.Background(), engine.BuildOptions{
		ConfigPath:  filepath.Join(dir, "config.yaml"),
		ContentDir:  filepath.Join(dir, "content"),
		ThemeDir:    filepath.Join(dir, "theme"),
		OutputDir:   out,
		SiteName:    "Playwright Demo",
		Incremental: incremental,
		CacheDir:    filepath.Join(dir, ".fuego"),
	}); err != nil {
		t.Fatalf("build failed: %v", err)
	}
}

func TestDemoSiteBuild(t *testing.T) {
	dir := demoSite(t)
	out := filepath.Join(dir, "out")
	build(t, dir, out, false)

	// The spec expanded into a routed section: the mirror routing tier strips
	// only the final extension, so checkout.spec.ts roots at /checkout.spec/.
	for _, page := range []string{
		"checkout.spec/index.html",
		"checkout.spec/checkout/index.html",
		"checkout.spec/checkout/guest-can-pay-with-card/index.html",
		"checkout.spec/checkout/discounts/rejects-an-expired-coupon/index.html",
		"checkout.spec/top-level-smoke/index.html",
	} {
		if _, err := os.Stat(filepath.Join(out, page)); err != nil {
			t.Errorf("missing expected page %s: %v", page, err)
		}
	}

	// Taxonomy visibility — the issue's acceptance criterion: test pages
	// carry envelope tags, so term pages exist and render. Terms are
	// lowercased in URLs (@Validation → by-tag/validation); assert the
	// lowercase form explicitly, since a case-insensitive filesystem would
	// happily "find" a wrong-case path.
	for term, wantHeading := range map[string]string{
		"smoke":      "Tag: smoke",
		"checkout":   "Tag: checkout",
		"validation": "Tag: validation",
	} {
		raw, err := os.ReadFile(filepath.Join(out, "by-tag", term, "index.html"))
		if err != nil {
			t.Fatalf("taxonomy term page %s missing — envelope tags not scanned: %v", term, err)
		}
		if !strings.Contains(string(raw), wantHeading) {
			t.Errorf("term page %s content wrong:\n%s", term, raw)
		}
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
		if strings.HasPrefix(p.URL, "/checkout.spec/") || p.URL == "/checkout.spec" {
			specPages++
			if p.SourcePath != "checkout.spec.ts" {
				t.Errorf("page %s source_path = %q, want checkout.spec.ts", p.URL, p.SourcePath)
			}
		}
	}
	// Root + 3 suites + 8 tests.
	if specPages != 12 {
		t.Errorf("expected the spec's 12 pages in the manifest, found %d", specPages)
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

	build(t, dir, clean, false)
	build(t, dir, incr, true) // cold cache
	build(t, dir, incr, true) // warm cache: the spec's whole tree restores from cache

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
