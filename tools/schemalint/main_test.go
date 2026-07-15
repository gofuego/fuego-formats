package main

import (
	"testing"
	"testing/fstest"
)

const completeSchema = `# Foo schema

## Claims
x

## Envelope keys
x

## Node types
x

## Tree shape
x

## Slug derivation
x

## Stability
x
`

func TestLintAcceptsCompleteSchema(t *testing.T) {
	fsys := fstest.MapFS{
		"foo/schema.md": {Data: []byte(completeSchema)},
		"docs/schema-template.md": {Data: []byte("ignored")},
		"tools/schemalint/main.go": {Data: []byte("ignored")},
	}
	found, problems, err := lint(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Fatalf("found = %d, want 1 (docs/ and tools/ must be skipped)", found)
	}
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
}

func TestLintFlagsMissingSection(t *testing.T) {
	missing := `# Bar

## Claims
x

## Node types
x
`
	fsys := fstest.MapFS{"bar/schema.md": {Data: []byte(missing)}}
	found, problems, err := lint(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Fatalf("found = %d, want 1", found)
	}
	// Missing: Envelope keys, Tree shape, Slug derivation, Stability.
	if len(problems) != 4 {
		t.Fatalf("problems = %d (%v), want 4", len(problems), problems)
	}
}

func TestContainsHeadingIsLineExact(t *testing.T) {
	if containsHeading("see the ## Claims section inline", "## Claims") {
		t.Error("a heading embedded in a sentence must not count")
	}
	if !containsHeading("## Claims  ", "## Claims") {
		t.Error("trailing whitespace should be tolerated")
	}
}
