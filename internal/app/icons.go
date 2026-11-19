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
	// searchMark is the magnifier that stands for a search engine light
	// has no picture of.
	searchMark = "search.svg"
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
		"spinner":  `<path d="M8 2.5a5.5 5.5 0 1 1-5.5 5.5"/>`,
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
	files[searchMark] = &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" ` +
		`height="16" viewBox="0 0 16 16" fill="none" stroke="#8f8a9c" stroke-width="1.5" ` +
		`stroke-linecap="round"><circle cx="7" cy="7" r="4.5"/><path d="M10.4 10.4 14 14"/></svg>`)}
	for name, drawn := range drawings() {
		files[name+".svg"] = &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" ` +
			`height="16" viewBox="0 0 24 24" fill="` + drawn.fill + `" stroke="` + drawn.stroke + `" stroke-width="2" ` +
			`stroke-linecap="round" stroke-linejoin="round">` + drawn.path + `</svg>`)}
	}
	return files
}

// drawing is a glyph drawn on a grid of 24 and shown at 16 points: its
// path, the colour of its lines and what fills it.
type drawing struct {
	path, stroke, fill string
}

// drawings are the glyphs of the address field's star, of the menu and
// of the bookmarks: the menu's rows in the grey of its labels, the star
// filled in the blue that marks what is chosen.
// tick is the mark of a robot whose part shows.
const tick = `<circle cx="12" cy="12" r="11"/><path d="M7 12.5l3.5 3.5 6.5-7.5" stroke="#0f1b2b"/>`

func drawings() map[string]drawing {
	const (
		star   = `<path d="M12 3l2.8 5.7 6.2.9-4.5 4.4 1 6.2L12 17.3 6.5 20.2l1-6.2L3 9.6l6.2-.9z"/>`
		dim    = "#a9a6b8"
		row    = "#c9c6d6"
		accent = "#7ab0ff"
		ready  = "#46c46e"
		warn   = "#f0a23c"
		robot  = `<rect x="4" y="8" width="16" height="11" rx="3"/><path d="M12 8V5M2 13v3M22 13v3M9 13v1M15 13v1"/>` +
			`<circle cx="12" cy="4" r="1"/>`
		lens = `<circle cx="11" cy="11" r="6.5"/><path d="M20 20l-4.3-4.3"/>`
		navy = "#0f1b2b"
	)
	glyphs := map[string]drawing{
		"star":    {path: star, stroke: dim},
		"star-on": {path: star, stroke: accent, fill: accent},
		"menu":    {path: `<path d="M4 7h16M4 12h16M4 17h16"/>`, stroke: "#efecf4"},
		"check":   {path: `<path d="M5 12l5 5 9-10"/>`, stroke: "#cfe1ff"},
		"folder":  {path: `<path d="M3 7h6l2 2h10v10H3z"/>`, stroke: "#8f8c9c"},

		"fetch":             {path: `<path d="M12 4v12M6 11l6 6 6-6M5 21h14"/>`, stroke: dim},
		"tick":              {path: tick, stroke: ready, fill: ready},
		"check-dark":        {path: `<path d="M5 12l5 5 9-10"/>`, stroke: "#0f1b2b"},
		"robot":             {path: robot, stroke: "#efecf4"},
		"robot-tile":        {path: robot, stroke: "#0f1b2b"},
		"find":              {path: lens, stroke: navy},
		"robot-ready":       {path: `<circle cx="12" cy="12" r="9"/>`, stroke: ready, fill: ready},
		"robot-working":     {path: `<rect x="4" y="4" width="16" height="16" rx="3"/>`, stroke: accent, fill: accent},
		"robot-preparing":   {path: `<circle cx="12" cy="12" r="8"/>`, stroke: accent},
		"robot-downloading": {path: `<path d="M12 3v15M5 12l7 7 7-7"/>`, stroke: accent},
		"robot-missing":     {path: `<path d="M4 12h16"/>`, stroke: warn},
		"robot-failed":      {path: `<path d="M12 3l10 18H2z"/>`, stroke: warn, fill: warn},
	}
	for name, path := range map[string]string{
		"tab":       `<rect x="4" y="4" width="16" height="16" rx="3"/><path d="M12 8v8M8 12h8"/>`,
		"window":    `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 9h18"/>`,
		"star":      star,
		"history":   `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`,
		"downloads": `<path d="M12 4v11M7 11l5 5 5-5M5 20h14"/>`,
		"zoom":      `<circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4M11 8v6M8 11h6"/>`,
		"minus":     `<path d="M6 12h12"/>`,
		"plus":      `<path d="M12 6v12M6 12h12"/>`,
		"full":      `<path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"/>`,
		"save":      `<path d="M5 4h11l3 3v13H5zM8 4v5h7M8 20v-6h8v6"/>`,
		"code":      `<path d="M8 7l-5 5 5 5M16 7l5 5-5 5M13.5 5l-3 14"/>`,
		"settings":  `<path d="M4 7h10M18 7h2M4 17h2M10 17h10"/><circle cx="16" cy="7" r="2"/><circle cx="8" cy="17" r="2"/>`,
		"quit":      `<path d="M12 3v8M6.3 6.3a8 8 0 1 0 11.4 0"/>`,
		"ask":       `<path d="M4 5h16v11H10l-4 4v-4H4z"/>`,
		"find":      lens,
		"model":     robot,
		"model-on":  robot,
		"sorted":    `<path d="M3 3h8.5l9.5 9.5-8.5 8.5L3 11.5z"/><circle cx="8" cy="8" r="1"/>`,
	} {
		glyphs["row-"+name] = drawing{path: path, stroke: row}
	}
	for name, d := range glyphs {
		if d.fill == "" {
			d.fill = "none"
			glyphs[name] = d
		}
	}
	return glyphs
}

func glyph(name string) *ui.Node { return ui.Img(ui.Attr("src", name+".svg"), ui.Attr("alt", "")) }
