package app

import (
	"strings"
	"testing/fstest"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light"
)

// logo is the name light's logo is drawn by, and darkLogo the name of
// the one a dark page shows: its navy and its white swapped.
const (
	logo     = "logo.svg"
	darkLogo = "logo-dark.svg"
)

// icons are light's own pictures: its logo, light and dark, and the
// glyphs drawn for it, 16 point SVG pictures its buttons, tabs and
// address field show.
func icons() fstest.MapFS {
	swapped := strings.NewReplacer("#0e2a3f", "#fff", "#fff", "#0e2a3f").Replace(string(light.Logo))
	files := fstest.MapFS{logo: &fstest.MapFile{Data: light.Logo}, darkLogo: &fstest.MapFile{Data: []byte(swapped)}}
	for name, path := range map[string]string{
		"back":     `<path d="M7 3.5 2.5 8 7 12.5M2.5 8h11"/>`,
		"forward":  `<path d="M9 3.5 13.5 8 9 12.5M13.5 8h-11"/>`,
		"reload":   `<path d="M13 8a5 5 0 1 1-1.46-3.54M12 1.6v3.2H8.8"/>`,
		"stop":     `<path d="M4 4l8 8M12 4l-8 8"/>`,
		"home":     `<path d="M2.5 8 8 2.5 13.5 8M4 6.8V13h8V6.8"/>`,
		"download": `<path d="M8 2.5v7.5M4.8 7 8 10.2 11.2 7M3 13h10"/>`,
		"lock":     `<path d="M8 2 3 4v4c0 3 2.2 5 5 6 2.8-1 5-3 5-6V4zM5.8 8.2l1.6 1.6 2.9-3"/>`,
		"open":     `<circle cx="8" cy="8" r="5.5"/><path d="M8 7.5v3.5M8 5v.2"/>`,
		"plus":     `<path d="M8 3v10M3 8h10"/>`,
		"close":    `<path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/>`,
		"tabs": `<path d="M9 10.5H3.5a1 1 0 0 1-1-1v-5a1 1 0 0 1 1-1h8a1 1 0 0 1 1 1v2"/>` +
			`<circle cx="11" cy="10.5" r="2"/><path d="M12.5 12l1.5 1.5"/>`,
	} {
		files[name+".svg"] = &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" ` +
			`height="16" viewBox="0 0 16 16" fill="none" stroke="#efecf4" stroke-width="1.3" ` +
			`stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`)}
	}
	return files
}

func glyph(name string) *ui.Node { return ui.Img(ui.Attr("src", name+".svg"), ui.Attr("alt", "")) }
