package docker_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofuego/fuego/engine"

	"github.com/gofuego/fuego-formats/docker"
)

// TestStandaloneDemoSite is the acceptance-criteria stage: a site registers
// just docker.Parser() — no fuego-devops dependency — and every claimed
// naming form becomes a routed page on a real fuego v0.5.0 build. The
// api.dockerfile file is the ADR-018 landmine proof: declared patterns are
// the complete claim set, so extension-named files parse only because
// *.dockerfile is a default claim.
func TestStandaloneDemoSite(t *testing.T) {
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

	// The two upstream naming forms live in their own directories, as in a
	// real repo (side by side they would collide at /Dockerfile/ once the
	// mirror tier strips the .prod extension).
	write("content/api/Dockerfile", "FROM golang:1.22 AS builder\nRUN go build\n")
	write("content/worker/Dockerfile.prod", "FROM alpine:3.19\nCMD [\"/app\"]\n")
	write("content/api.dockerfile", "---\ntitle: \"api Dockerfile\"\n---\nFROM node:18\nRUN npm ci\n")
	write("theme/base.html", `<!DOCTYPE html>
<html><head><title>{{.Page.Envelope.title}}</title></head>
<body>{{template "content" .}}</body></html>
{{define "content"}}{{.Page.Content}}{{end}}`)

	out := filepath.Join(dir, "out")
	eng := engine.New()
	eng.Register(docker.Parser())
	if err := eng.Build(context.Background(), engine.BuildOptions{
		ContentDir: filepath.Join(dir, "content"),
		ThemeDir:   filepath.Join(dir, "theme"),
		OutputDir:  out,
		SiteName:   "Docker Demo",
	}); err != nil {
		t.Fatalf("build failed: %v", err)
	}

	// The scanner-emitted naming form routes like any extension: /api/.
	if _, err := os.Stat(filepath.Join(out, "api", "index.html")); err != nil {
		t.Errorf("api.dockerfile was not claimed — the *.dockerfile pattern is load-bearing: %v", err)
	}

	// All three naming forms became docker pages.
	var manifest struct {
		Pages []struct {
			URL        string `json:"url"`
			Type       string `json:"type"`
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
	sources := map[string]bool{}
	for _, p := range manifest.Pages {
		if p.Type == "docker" {
			sources[p.SourcePath] = true
		}
	}
	for _, want := range []string{"api/Dockerfile", "worker/Dockerfile.prod", "api.dockerfile"} {
		if !sources[want] {
			t.Errorf("no docker page for %s in the manifest (got %v)", want, sources)
		}
	}
}
