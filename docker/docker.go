// Package docker is a fuego-formats parser for Dockerfiles.
//
// One Dockerfile becomes one page: a node per build stage (FROM), per
// instruction, and per comment. Stage and instruction attributes carry the
// identifiers relationship hooks read — the stage image and alias, the
// instruction's stage, and COPY --from references — so a consumer like
// fuego-devops builds its architecture graph from node attributes alone.
// That attribute contract is public API; see schema.md.
//
// The parser accepts optional YAML frontmatter (a scanner front-end like
// fuego-devops emits title/source_path/resource_kind that way); a plain
// Dockerfile without frontmatter parses the same.
//
// Usage:
//
//	eng := engine.New()
//	eng.Register(docker.Parser())
//
// The default claims cover the upstream naming conventions plus the
// *.dockerfile extension (required: under specificity dispatch, declared
// patterns are a parser's complete claim set, so extension-named files must
// be claimed explicitly). Override with formatkit.WithPatterns(...).
package docker

import (
	"strings"

	"github.com/gofuego/fuego/core"

	"github.com/gofuego/fuego-formats/formatkit"
)

// Type is the parser's Type() — the format slug and every page's type.
const Type = "docker"

// Node types emitted by the parser. All are prefixed with the format slug so
// a theme's renderer templates never collide with another format's.
const (
	// NodeStage is one build stage — a FROM instruction. Content is the full
	// FROM line; attributes carry image and alias.
	NodeStage = "docker-stage"
	// NodeInstruction is any non-FROM instruction. Content is the
	// instruction's arguments; attributes carry instruction (uppercased),
	// stage (the enclosing stage's alias, when named), and copyFrom (the
	// --from stage of a COPY, when present).
	NodeInstruction = "docker-instruction"
	// NodeComment is one # comment line; content is the comment text.
	NodeComment = "docker-comment"
)

// DefaultPatterns are the built-in filename claims: the upstream naming
// conventions plus the *.dockerfile extension form scanner front-ends emit.
// Claims match base names only.
var DefaultPatterns = []string{"Dockerfile", "Dockerfile.*", "*.dockerfile"}

// Option re-exports formatkit.Option so callers configure the parser without
// importing formatkit directly for the common case.
type Option = formatkit.Option

// Parser returns a Fuego parser claiming the DefaultPatterns. Pass
// formatkit.WithPatterns(...) to override the claims.
func Parser(opts ...Option) core.Parser {
	all := append([]Option{formatkit.WithDefaultPatterns(DefaultPatterns...)}, opts...)
	return formatkit.NewParser(Type, parse, all...)
}

// parse splits optional YAML frontmatter off the file, then walks the
// Dockerfile line by line. It deliberately emits no layout key: the page uses
// the site's default layout, so a consuming pack's layout semantics are
// untouched.
func parse(raw []byte) (core.Envelope, []core.Node, error) {
	env, payload, err := core.SplitFrontmatter(raw)
	if err != nil {
		return nil, nil, err
	}
	if env == nil {
		env = make(core.Envelope)
	}

	lines := strings.Split(string(payload), "\n")
	var nodes []core.Node
	var currentStage string
	var images []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			nodes = append(nodes, core.Node{
				Type:    NodeComment,
				Content: strings.TrimPrefix(trimmed, "#"),
			})
			continue
		}

		parts := strings.SplitN(trimmed, " ", 2)
		instruction := strings.ToUpper(parts[0])
		var args string
		if len(parts) > 1 {
			args = parts[1]
		}

		switch instruction {
		case "FROM":
			image, alias := parseFrom(args)
			currentStage = alias
			if image != "" {
				images = append(images, image)
			}
			nodes = append(nodes, core.Node{
				Type:    NodeStage,
				Content: trimmed,
				Attributes: map[string]any{
					"image": image,
					"alias": alias,
				},
			})
		default:
			attrs := map[string]any{
				"instruction": instruction,
			}
			if currentStage != "" {
				attrs["stage"] = currentStage
			}
			if instruction == "COPY" {
				if from := parseCopyFrom(args); from != "" {
					attrs["copyFrom"] = from
				}
			}
			nodes = append(nodes, core.Node{
				Type:       NodeInstruction,
				Content:    args,
				Attributes: attrs,
			})
		}
	}

	// A frontmatter title wins; otherwise derive one from the last stage's
	// alias, else the first base image.
	if env["title"] == nil {
		if currentStage != "" {
			env["title"] = "Dockerfile (" + currentStage + ")"
		} else if len(images) > 0 {
			env["title"] = "Dockerfile — " + images[0]
		} else {
			env["title"] = "Dockerfile"
		}
	}
	if len(images) > 0 {
		// []any, not []string — the envelope stays cache-eligible.
		list := make([]any, len(images))
		for i, img := range images {
			list[i] = img
		}
		env["images"] = list
	}
	env["resource_kind"] = "Dockerfile"

	return env, nodes, nil
}

// parseCopyFrom extracts the --from=<name> value from COPY args, if present.
func parseCopyFrom(args string) string {
	for _, field := range strings.Fields(args) {
		if strings.HasPrefix(field, "--from=") {
			return strings.TrimPrefix(field, "--from=")
		}
	}
	return ""
}

// parseFrom extracts the image and optional alias from a FROM line.
// e.g. "golang:1.22 AS builder" → ("golang:1.22", "builder")
func parseFrom(args string) (string, string) {
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return "", ""
	}
	image := parts[0]
	alias := ""
	for i, p := range parts {
		if strings.EqualFold(p, "AS") && i+1 < len(parts) {
			alias = parts[i+1]
			break
		}
	}
	return image, alias
}
