module github.com/gofuego/fuego-formats/playwright

go 1.25.0

require (
	github.com/gofuego/fuego v0.5.0
	github.com/gofuego/fuego-formats/formatkit v0.2.0
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

// Local replace until formatkit/v0.2.0 (NewTreeParser) is tagged at the next
// develop->main merge; remove once the tag exists.
replace github.com/gofuego/fuego-formats/formatkit => ../formatkit
