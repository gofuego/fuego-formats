module github.com/gofuego/fuego-formats/docker

go 1.25.0

require (
	github.com/gofuego/fuego v0.5.0
	github.com/gofuego/fuego-formats/formatkit v0.2.0
)

require (
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// Local replace until formatkit/v0.2.0 (NewTreeParser) is tagged at the next
// develop->main merge; remove once the tag exists.
replace github.com/gofuego/fuego-formats/formatkit => ../formatkit
