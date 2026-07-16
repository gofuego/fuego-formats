package docker_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/docker"
	"github.com/gofuego/fuego-formats/formatkit"
)

var update = flag.Bool("update", false, "regenerate golden fixtures")

// The *.dockerfile pattern is load-bearing: under specificity dispatch
// (fuego ADR-018) declared patterns are the complete claim set, so
// scanner-emitted <name>.dockerfile files must be claimed explicitly.
func TestParserClaimsDefaults(t *testing.T) {
	fp, ok := docker.Parser().(core.FilenameParser)
	if !ok {
		t.Fatal("docker.Parser() must implement core.FilenameParser")
	}
	if got := fp.Filenames(); !reflect.DeepEqual(got, docker.DefaultPatterns) {
		t.Errorf("Filenames() = %v, want %v", got, docker.DefaultPatterns)
	}
	want := []string{"Dockerfile", "Dockerfile.*", "*.dockerfile"}
	if !reflect.DeepEqual(docker.DefaultPatterns, want) {
		t.Errorf("DefaultPatterns = %v, want %v", docker.DefaultPatterns, want)
	}
	if fp.Type() != "docker" {
		t.Errorf("Type() = %q, want docker", fp.Type())
	}
}

func TestWithPatternsOverride(t *testing.T) {
	fp := docker.Parser(formatkit.WithPatterns("Containerfile")).(core.FilenameParser)
	if got := fp.Filenames(); !reflect.DeepEqual(got, []string{"Containerfile"}) {
		t.Errorf("Filenames() = %v, want [Containerfile]", got)
	}
}

func TestStagesAndInstructions(t *testing.T) {
	raw := []byte(`FROM golang:1.22 AS builder
RUN go build -o /app
FROM alpine:3.19
COPY --from=builder /app /app
EXPOSE 8080
CMD ["/app"]
`)
	env, nodes, err := docker.Parser().Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if env["resource_kind"] != "Dockerfile" {
		t.Errorf("resource_kind = %v", env["resource_kind"])
	}
	// images is []any — cache-eligible.
	images, ok := env["images"].([]any)
	if !ok || !reflect.DeepEqual(images, []any{"golang:1.22", "alpine:3.19"}) {
		t.Errorf("images = %#v", env["images"])
	}
	// The last stage is unnamed, so the title falls back to the first image.
	if env["title"] != "Dockerfile — golang:1.22" {
		t.Errorf("title = %v", env["title"])
	}

	if len(nodes) != 6 {
		t.Fatalf("want 6 nodes (2 stages + 4 instructions), got %d", len(nodes))
	}
	if nodes[0].Type != docker.NodeStage || nodes[0].Attributes["image"] != "golang:1.22" || nodes[0].Attributes["alias"] != "builder" {
		t.Errorf("first stage node = %+v", nodes[0])
	}
	run := nodes[1]
	if run.Type != docker.NodeInstruction || run.Attributes["instruction"] != "RUN" || run.Attributes["stage"] != "builder" {
		t.Errorf("RUN node = %+v", run)
	}
	copyNode := nodes[3]
	if copyNode.Attributes["copyFrom"] != "builder" {
		t.Errorf("COPY node = %+v", copyNode)
	}
}

func TestCommentsAndTitleFromStage(t *testing.T) {
	raw := []byte(`# Build stage
FROM golang:1.22 AS builder
RUN go build
`)
	env, nodes, err := docker.Parser().Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 || nodes[0].Type != docker.NodeComment || nodes[0].Content != " Build stage" {
		t.Errorf("nodes = %+v", nodes)
	}
	if env["title"] != "Dockerfile (builder)" {
		t.Errorf("title = %v", env["title"])
	}
}

// Frontmatter (as emitted by a scanner front-end) wins over the derived title;
// a plain Dockerfile parses identically without it.
func TestFrontmatterTitleWins(t *testing.T) {
	raw := []byte(`---
title: Custom Title
---
FROM node:18
RUN npm install
`)
	env, nodes, err := docker.Parser().Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if env["title"] != "Custom Title" {
		t.Errorf("title = %v", env["title"])
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
}

func TestNoLayoutKey(t *testing.T) {
	env, _, err := docker.Parser().Parse([]byte("FROM scratch\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := env["layout"]; ok {
		t.Errorf("the parser must not emit a layout key (consumer layout semantics), got %v", env["layout"])
	}
}

// dump is the golden node-dump: the parser's full output for a fixture input,
// simultaneously the regression test and the shipped contract example.
type dump struct {
	Envelope core.Envelope `json:"envelope"`
	Nodes    []core.Node   `json:"nodes"`
}

// TestGoldenDump covers a plain multi-stage Dockerfile and a scanner-shaped
// *.dockerfile with frontmatter. Regenerate with: go test ./docker -update
func TestGoldenDump(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata/*.dockerfile fixtures found")
	}

	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".dockerfile")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			env, nodes, err := docker.Parser().Parse(raw)
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
