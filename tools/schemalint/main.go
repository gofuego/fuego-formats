// Command schemalint verifies that every format module's schema.md contains all
// required section headings. It is deliberately dumb and reliable: it walks a
// root directory, finds every <format>/schema.md, and checks that each required
// "## Heading" appears literally. The required set mirrors docs/schema-template.md.
//
// Usage:
//
//	go run . <root-dir>   # defaults to "." when omitted
//
// Exit code 1 on any missing section or if no schema.md is found.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// requiredSections are the headings every schema.md must contain, spelled
// exactly. Keep in sync with docs/schema-template.md.
var requiredSections = []string{
	"## Claims",
	"## Envelope keys",
	"## Node types",
	"## Tree shape",
	"## Slug derivation",
	"## Stability",
}

// skipDirs are directories the walk never descends into.
var skipDirs = map[string]bool{
	".git":   true,
	"docs":   true, // holds the template, not a module schema
	"tools":  true,
	"vendor": true,
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	found, problems, err := lint(os.DirFS(root))
	if err != nil {
		fmt.Fprintln(os.Stderr, "schemalint:", err)
		os.Exit(1)
	}

	if found == 0 {
		fmt.Fprintf(os.Stderr, "schemalint: no schema.md found under %s\n", root)
		os.Exit(1)
	}

	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "schemalint:", p)
		}
		os.Exit(1)
	}

	fmt.Printf("schemalint: %d schema.md file(s) OK\n", found)
}

// lint walks fsys, checks every schema.md, and returns the count checked plus a
// list of human-readable problems.
func lint(fsys fs.FS) (found int, problems []string, err error) {
	walkErr := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != "." && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "schema.md" {
			return nil
		}

		found++
		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			return readErr
		}
		content := string(data)
		for _, section := range requiredSections {
			if !containsHeading(content, section) {
				problems = append(problems, fmt.Sprintf("%s: missing required section %q", path, section))
			}
		}
		return nil
	})
	return found, problems, walkErr
}

// containsHeading reports whether content has a line that is exactly the given
// heading (after trimming trailing whitespace), so a substring match inside a
// paragraph never counts.
func containsHeading(content, heading string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimRight(line, " \t\r") == heading {
			return true
		}
	}
	return false
}
