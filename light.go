// Package light holds what the repository's root holds for its program:
// the logo, where the README shows it from.
package light

import _ "embed"

// Logo is light's logo, an SVG picture.
//
//go:embed logo.svg
var Logo []byte
