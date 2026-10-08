// Package web embeds the browser UI into the binary.
package web

import "embed"

// FS holds the UI files, served at /.
//
//go:embed index.html favicon.svg css js vendor
var FS embed.FS
