package adr_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofuego/fuego/engine"
	"github.com/gofuego/fuego/parsers/markdown"

	"github.com/gofuego/fuego-formats/adr"
)

// TestCoexistsWithMarkdownParser is the acceptance-criteria stage: a site
// registers adr.Parser() next to the engine's markdown parser, and
// specificity dispatch routes *.adr.md here while plain .md stays with
// markdown — the exact overlap ADR-018 was built for. No fuego-adr
// dependency anywhere.
func TestCoexistsWithMarkdownParser(t *testing.T) {
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

	write("content/0001-use-postgres.adr.md", `---
title: Use PostgreSQL
status: Accepted
tags: [database]
---
## Context

We need a database.

## Decision

We will use **PostgreSQL**.

## Consequences

Fine.
`)
	write("content/notes.md", "# Notes\n\nJust **markdown** notes.\n")
	write("theme/base.html", `<!DOCTYPE html>
<html><head><title>{{.Page.Envelope.title}}</title></head>
<body>{{template "content" .}}</body></html>
{{define "content"}}{{.Page.Content}}{{end}}`)

	out := filepath.Join(dir, "out")
	eng := engine.New()
	eng.Register(adr.Parser())
	eng.Register(markdown.Parser())
	if err := eng.Build(context.Background(), engine.BuildOptions{
		ContentDir: filepath.Join(dir, "content"),
		ThemeDir:   filepath.Join(dir, "theme"),
		OutputDir:  out,
		SiteName:   "ADR Demo",
	}); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// The ADR routed and rendered through this parser: its sections are
	// pre-rendered raw HTML nodes.
	adrPage, err := os.ReadFile(filepath.Join(out, "0001-use-postgres.adr", "index.html"))
	if err != nil {
		t.Fatalf("ADR page missing: %v", err)
	}
	if !strings.Contains(string(adrPage), "<strong>PostgreSQL</strong>") {
		t.Errorf("ADR sections not rendered: %s", adrPage)
	}

	// The plain markdown file stayed with the markdown parser.
	notes, err := os.ReadFile(filepath.Join(out, "notes", "index.html"))
	if err != nil {
		t.Fatalf("markdown page missing: %v", err)
	}
	if !strings.Contains(string(notes), "<strong>markdown</strong>") {
		t.Errorf("markdown not rendered: %s", notes)
	}

	// The manifest proves the dispatch split: each page carries its parser's
	// type.
	var manifest struct {
		Pages []struct {
			SourcePath string `json:"source_path"`
			Type       string `json:"type"`
		} `json:"pages"`
	}
	raw, err := os.ReadFile(filepath.Join(out, "site-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	types := map[string]string{}
	for _, p := range manifest.Pages {
		if p.SourcePath != "" {
			types[p.SourcePath] = p.Type
		}
	}
	if types["0001-use-postgres.adr.md"] != "adr" {
		t.Errorf("ADR page type = %q, want adr (got map %v)", types["0001-use-postgres.adr.md"], types)
	}
	if got := types["notes.md"]; got != markdown.Type {
		t.Errorf("notes.md type = %q, want %q", got, markdown.Type)
	}
}
