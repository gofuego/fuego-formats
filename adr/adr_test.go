package adr_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/adr"
	"github.com/gofuego/fuego-formats/formatkit"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

func TestParserClaimsDefaults(t *testing.T) {
	fp, ok := adr.Parser().(core.FilenameParser)
	if !ok {
		t.Fatal("adr.Parser() must implement core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, adr.DefaultPatterns) {
		t.Errorf("Filenames() = %v, want %v", got, adr.DefaultPatterns)
	}
	if !reflect.DeepEqual(adr.DefaultPatterns, []string{"*.adr.md"}) {
		t.Errorf("DefaultPatterns = %v, want [*.adr.md]", adr.DefaultPatterns)
	}
	if fp.Type() != "adr" {
		t.Errorf("Type() = %q, want adr", fp.Type())
	}
}

func TestWithPatternsOverride(t *testing.T) {
	fp := adr.Parser(formatkit.WithPatterns("*.decision.md")).(core.FilenameParser)
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"*.decision.md"}) {
		t.Errorf("Filenames() = %v, want [*.decision.md]", got)
	}
}

func TestSectionNodeType(t *testing.T) {
	cases := map[string]string{
		"context":            adr.NodeContext,
		"decision":           adr.NodeDecision,
		"consequences":       adr.NodeConsequences,
		"preamble":           adr.NodePreamble,
		"options considered": "adr-options-considered",
		"???":                "adr-section",
	}
	for heading, want := range cases {
		if got := adr.SectionNodeType(heading); got != want {
			t.Errorf("SectionNodeType(%q) = %q, want %q", heading, got, want)
		}
	}
}

// The envelope normalization is the convention contract: list fields become
// []string / []int (deliberately not []any — consumer hooks assert these
// types and both are cache-registered), status lowercases, dates format.
func TestEnvelopeNormalization(t *testing.T) {
	raw := []byte(`---
title: Use PostgreSQL
status: Accepted
author: fabio
tags: [database, infrastructure]
supersedes: 3
date_proposed: 2026-06-11
---
## Context

We need a relational database.

## Decision

We will use **PostgreSQL** for all persistent storage.

## Consequences

- Strong ecosystem
`)
	env, nodes, err := adr.Parser().Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if env["status"] != "accepted" {
		t.Errorf("status = %q, want accepted", env["status"])
	}
	if authors, ok := env["author"].([]string); !ok || !reflect.DeepEqual(authors, []string{"fabio"}) {
		t.Errorf("author = %#v, want []string{fabio}", env["author"])
	}
	if tags, ok := env["tags"].([]string); !ok || !reflect.DeepEqual(tags, []string{"database", "infrastructure"}) {
		t.Errorf("tags = %#v", env["tags"])
	}
	if sup, ok := env["supersedes"].([]int); !ok || !reflect.DeepEqual(sup, []int{3}) {
		t.Errorf("supersedes = %#v, want []int{3}", env["supersedes"])
	}
	if env["date_proposed"] != "2026-06-11" {
		t.Errorf("date_proposed = %v", env["date_proposed"])
	}
	if _, ok := env["layout"]; ok {
		t.Errorf("the parser must not emit a layout key, got %v", env["layout"])
	}

	wantTypes := []string{adr.NodeContext, adr.NodeDecision, adr.NodeConsequences}
	if len(nodes) != 3 {
		t.Fatalf("len(nodes) = %d, want 3", len(nodes))
	}
	for i, wt := range wantTypes {
		if nodes[i].Type != wt {
			t.Errorf("nodes[%d].Type = %q, want %q", i, nodes[i].Type, wt)
		}
		if !nodes[i].Raw {
			t.Errorf("nodes[%d].Raw = false, want true", i)
		}
	}
	if !strings.Contains(nodes[1].Content, "<strong>PostgreSQL</strong>") {
		t.Errorf("decision content missing rendered bold: %s", nodes[1].Content)
	}
}

func TestPreambleAndFreeFormSections(t *testing.T) {
	raw := []byte(`---
title: T
status: tbd
---
Before any heading.

## Context

ctx

## Options Considered

opts
`)
	_, nodes, err := adr.Parser().Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 || nodes[0].Type != adr.NodePreamble {
		t.Fatalf("nodes = %+v", nodes)
	}
	if nodes[2].Type != "adr-options-considered" {
		t.Errorf("free-form section type = %q", nodes[2].Type)
	}
}

func TestCrossLinkRewrite(t *testing.T) {
	raw := []byte(`---
title: Some decision
status: Accepted
---
## Context

This builds on [001](001-universal-ast.adr.md) and conflicts with
[002](002-other.adr.md#decision). See also https://example.com/x.adr.md.

## Decision

Decided.

## Consequences

Done.
`)
	_, nodes, err := adr.Parser().Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx := nodes[0].Content

	if !strings.Contains(ctx, `href="decisions/001-universal-ast.adr/"`) {
		t.Errorf("cross-link not rewritten: %s", ctx)
	}
	if !strings.Contains(ctx, `href="decisions/002-other.adr/#decision"`) {
		t.Errorf("fragment cross-link not rewritten: %s", ctx)
	}
	if strings.Contains(ctx, `href="001-universal-ast.adr.md"`) {
		t.Errorf("raw .adr.md link still present: %s", ctx)
	}
	if !strings.Contains(ctx, "https://example.com/x.adr.md") {
		t.Errorf("absolute URL should not be rewritten: %s", ctx)
	}
}

func TestExtractADRNumber(t *testing.T) {
	tests := []struct {
		filename string
		want     int
	}{
		{"0012-use-postgres.adr.md", 12},
		{"001-initial.adr.md", 1},
		{"42-something.adr.md", 42},
		{"no-number.adr.md", -1},
		{"path/to/0005-nested.adr.md", 5},
	}
	for _, tt := range tests {
		if got := adr.ExtractADRNumber(tt.filename); got != tt.want {
			t.Errorf("ExtractADRNumber(%q) = %d, want %d", tt.filename, got, tt.want)
		}
	}
}

// ValidateSections checks the prefixed node types but reports plain section
// names — the human-readable form consumer warnings print.
func TestValidateSections(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		types       []string
		wantMissing []string
	}{
		{"accepted with all sections", "accepted", []string{adr.NodeContext, adr.NodeDecision, adr.NodeConsequences}, nil},
		{"accepted missing consequences", "accepted", []string{adr.NodeContext, adr.NodeDecision}, []string{"consequences"}},
		{"accepted missing all", "accepted", []string{}, []string{"context", "decision", "consequences"}},
		{"tbd missing sections is OK", "tbd", []string{}, nil},
		{"proposed missing sections is OK", "proposed", []string{adr.NodeContext}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodes []core.Node
			for _, typ := range tt.types {
				nodes = append(nodes, core.Node{Type: typ})
			}
			if got := adr.ValidateSections(tt.status, nodes); !reflect.DeepEqual(got, tt.wantMissing) {
				t.Errorf("missing = %v, want %v", got, tt.wantMissing)
			}
		})
	}
}

func TestValidStatuses(t *testing.T) {
	for _, s := range []string{"tbd", "proposed", "accepted", "deprecated", "superseded"} {
		if !adr.ValidStatuses[s] {
			t.Errorf("status %q should be valid", s)
		}
	}
	if adr.ValidStatuses["draft"] {
		t.Error("draft is not a convention status")
	}
}

// dump is the golden node-dump: the parser's full output for a fixture input,
// simultaneously the regression test and the shipped contract example.
type dump struct {
	Envelope core.Envelope `json:"envelope"`
	Nodes    []core.Node   `json:"nodes"`
}

// TestGoldenDump covers a full accepted ADR (lists, dates, supersession,
// affects, preamble, cross-links, a free-form section) and a superseded one.
// Regenerate with: go test ./adr -update
func TestGoldenDump(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.adr.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.adr.md fixtures found")
	}

	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".adr.md")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			env, nodes, err := adr.Parser().Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(dump{Envelope: env, Nodes: nodes}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			golden := filepath.Join("testdata", name+".golden.json")
			if *update {
				if err := os.WriteFile(golden, got, 0644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("reading golden (run with -update): %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("golden mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}
		})
	}
}
