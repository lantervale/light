package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"strings"
	"testing/fstest"
)

// icons are the explorer's own glyphs, drawn for it: 16 point SVG
// pictures its buttons, tabs and address field show.
func icons() fstest.MapFS {
	files := fstest.MapFS{}
	for name, path := range map[string]string{
		"back":    `<path d="M7 3.5 2.5 8 7 12.5M2.5 8h11"/>`,
		"forward": `<path d="M9 3.5 13.5 8 9 12.5M13.5 8h-11"/>`,
		"reload":  `<path d="M13 8a5 5 0 1 1-1.46-3.54M12 1.6v3.2H8.8"/>`,
		"stop":    `<path d="M4 4l8 8M12 4l-8 8"/>`,
		"home":    `<path d="M2.5 8 8 2.5 13.5 8M4 6.8V13h8V6.8"/>`,
		"lock":    `<path d="M8 2 3 4v4c0 3 2.2 5 5 6 2.8-1 5-3 5-6V4zM5.8 8.2l1.6 1.6 2.9-3"/>`,
		"open":    `<circle cx="8" cy="8" r="5.5"/><path d="M8 7.5v3.5M8 5v.2"/>`,
		"plus":    `<path d="M8 3v10M3 8h10"/>`,
		"close":   `<path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/>`,
		"page":    `<circle cx="8" cy="8" r="5.5"/><path d="M8 7.5v3.5M8 5v.2"/>`,
		"tabs": `<path d="M9 10.5H3.5a1 1 0 0 1-1-1v-5a1 1 0 0 1 1-1h8a1 1 0 0 1 1 1v2"/>` +
			`<circle cx="11" cy="10.5" r="2"/><path d="M12.5 12l1.5 1.5"/>`,
	} {
		files[name+".svg"] = &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" ` +
			`height="16" viewBox="0 0 16 16" fill="none" stroke="#efecf4" stroke-width="1.3" ` +
			`stroke-linecap="round" stroke-linejoin="round">` + path + `</svg>`)}
	}
	return files
}

// favicon is a site's icon as a picture the explorer can draw, nil for
// one it can't: a PNG, JPEG or GIF as it is, an SVG by its name or its
// type, and of an ICO file its largest picture stored as a PNG. An ICO
// holding only bitmaps has none.
func favicon(name, media string, data []byte) []byte {
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	if media == "image/svg+xml" || strings.HasSuffix(strings.ToLower(name), ".svg") {
		return data
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return data
	}
	return largestPNG(data)
}

// largestPNG is the widest picture of an ICO file stored as a PNG.
func largestPNG(ico []byte) []byte {
	const header, entry = 6, 16
	if len(ico) < header || binary.LittleEndian.Uint16(ico) != 0 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil
	}
	var best []byte
	widest := 0
	for i := range int(binary.LittleEndian.Uint16(ico[4:])) {
		at := header + i*entry
		if at+entry > len(ico) {
			break
		}
		width := int(ico[at])
		if width == 0 {
			width = 256
		}
		size, from := int(binary.LittleEndian.Uint32(ico[at+8:])), int(binary.LittleEndian.Uint32(ico[at+12:]))
		if from < 0 || size <= 0 || from+size > len(ico) || width <= widest {
			continue
		}
		if pic := ico[from : from+size]; bytes.HasPrefix(pic, []byte("\x89PNG\r\n\x1a\n")) {
			best, widest = pic, width
		}
	}
	return best
}
