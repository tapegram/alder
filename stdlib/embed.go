// Package stdlib embeds the Alder standard library signature files.
package stdlib

import "embed"

//go:embed *.alder
var FS embed.FS
