package mermaid_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofuego/fuego/engine"

	"github.com/gofuego/fuego-formats/mermaid"
)

// TestEngineDiscoversRoutesRenders is the acceptance-criteria integration test:
// eng.Register(mermaid.Parser()) on a real fuego engine, pointed at a tiny site
// with one .mmd file, must discover, route, and render that file client-side —
// the raw diagram source reaching the output HTML for mermaid.js to render.
func TestEngineDiscoversRoutesRenders(t *testing.T) {
	dir := t.TempDir()
	content := filepath.Join(dir, "content")
	theme := filepath.Join(dir, "theme")
	out := filepath.Join(dir, "out")

	write := func(path, body string) {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A single mermaid diagram file, routed by the filesystem tier to /architecture/.
	write("content/architecture.mmd", "---\ntitle: System Architecture\n---\ngraph LR\n  Client-->Gateway\n  Gateway-->Engine\n")

	// A minimal theme: base shell plus a mermaid layout that loads the client
	// library. The layout falling back to base is the default; here we provide it
	// explicitly to prove the layout name is wired.
	write("theme/base.html", `<!DOCTYPE html><html><head><title>{{.Page.Envelope.title}}</title></head>`+
		`<body>{{template "content" .}}</body></html>`+
		"\n"+`{{define "content"}}{{.Page.Content}}{{end}}`)
	write("theme/layouts/mermaid.html", `{{define "content"}}<div id="diagram">{{.Page.Content}}</div>`+
		`<script type="module">import mermaid from "https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.esm.min.mjs";mermaid.run();</script>{{end}}`)

	eng := engine.New()
	eng.Register(mermaid.Parser())

	if err := eng.Build(context.Background(), engine.BuildOptions{
		ContentDir: content,
		ThemeDir:   theme,
		OutputDir:  out,
		SiteName:   "Diagrams",
	}); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// Discovered + routed: architecture.mmd -> /architecture/index.html.
	page := filepath.Join(out, "architecture", "index.html")
	data, err := os.ReadFile(page)
	if err != nil {
		t.Fatalf("expected routed output %s: %v", page, err)
	}
	html := string(data)

	// Rendered client-side: the <pre class="mermaid"> block and the diagram
	// source reach the HTML for mermaid.js to pick up.
	if !strings.Contains(html, `<pre class="mermaid">`) {
		t.Errorf("output missing <pre class=\"mermaid\"> block:\n%s", html)
	}
	if !strings.Contains(html, "Client--&gt;Gateway") {
		t.Errorf("output missing escaped diagram source:\n%s", html)
	}
	// The envelope title flowed through to the page.
	if !strings.Contains(html, "System Architecture") {
		t.Errorf("output missing derived title:\n%s", html)
	}
	// The mermaid layout was selected (its client-side loader is present).
	if !strings.Contains(html, "mermaid.run()") {
		t.Errorf("mermaid layout not applied (loader absent):\n%s", html)
	}
}
