package devtools

import _ "github.com/lantervale/ember/ui/svg" // the panel's pictures are SVG: ui draws those once this is imported

// The pictures a line of the console begins with.
const (
	pictureInfo  = "info"
	pictureWarn  = "warn"
	pictureError = "error"
	pictureInput = "input"
)

// pictures are the panel's glyphs, by name: what a 16 by 16 svg element
// holds, drawn in strokes of the ink pictureInk names for it, a grey
// that reads on a dark panel and on a light one otherwise.
//
//nolint:gochecknoglobals // a table
var pictures = map[string]string{
	"close":      `<path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/>`,
	"clear":      `<path d="M3 4.5h10M6.5 4.5v-2h3v2M4.5 4.5l.6 9h5.8l.6-9M6.8 7v4M9.2 7v4"/>`,
	"save":       `<path d="M8 2.5v7.5M5 7.2 8 10l3-2.8M3 13.5h10"/>`,
	"filter":     `<path d="M2.5 3.5h11L9.5 8.5v4l-3 1v-5z"/>`,
	"lock":       `<path d="M4 7.5h8v6H4zM5.8 7.5V5.2a2.2 2.2 0 0 1 4.4 0v2.3"/>`,
	"failed":     `<circle cx="8" cy="8" r="6"/><path d="M8 4.8v3.8M8 11v.2"/>`,
	"blocked":    `<circle cx="8" cy="8" r="5.5"/><path d="M4.2 4.2l7.6 7.6"/>`,
	pictureInfo:  `<circle cx="8" cy="8" r="6"/><path d="M8 7.4v3.8M8 4.9v.2"/>`,
	pictureWarn:  `<path d="M8 2.2 14.2 13H1.8zM8 6.4v3.2M8 11.3v.2"/>`,
	pictureError: `<circle cx="8" cy="8" r="6"/><path d="M8 4.8v3.8M8 11v.2"/>`,
	pictureInput: `<path d="M4 3.5 8.5 8 4 12.5M9 3.5 13.5 8 9 12.5"/>`,
	"result":     `<path d="M10 3.5 5.5 8l4.5 4.5"/>`,
	"folded":     `<path d="M6 4l4 4-4 4z"/>`,
	"open-fold":  `<path d="M4 6l4 4 4-4z"/>`,
	"up":         `<path d="M4.5 9.5 8 6l3.5 3.5"/>`,
	"down":       `<path d="M4.5 6.5 8 10l3.5-3.5"/>`,
}

// pictureInk is the ink of the pictures that have one of their own.
//
//nolint:gochecknoglobals // a table
var pictureInk = map[string]string{
	"failed": "#ffffff", "blocked": "#e5484d", pictureInfo: "#4c9ef5", pictureWarn: "#e0a100",
	pictureError: "#e5484d", pictureInput: "#4c9ef5", "lock": "#3fa45b",
}

func pictureName(name string) string { return "dt-" + name + ".svg" }

func picture(paths, ink string) []byte {
	if ink == "" {
		ink = "#8f8f9d"
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" ` +
		`stroke="` + ink + `" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`)
}
