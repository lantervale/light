// Package content decides how an answer is shown from its Content-Type,
// its first bytes and its address, as a browser does, and names the file
// a download is saved as.
package content

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"
)

// Kind is how an answer is shown.
type Kind int

const (
	// Download is what is neither read nor drawn: said, and saved on request.
	Download Kind = iota
	// Page is HTML, built and laid out.
	Page
	// Picture is drawn alone.
	Picture
	// Text is read as it was sent.
	Text
)

func (k Kind) String() string {
	return [...]string{"a download", "a page", "a picture", "text"}[k]
}

const (
	octetStream = "application/octet-stream"
	plainText   = "text/plain"
	icon        = "image/x-icon"
	html        = "text/html"
	jpeg        = "image/jpeg"
	png         = "image/png"
	gif         = "image/gif"
	webp        = "image/webp"
	svg         = "image/svg+xml"
	json        = "application/json"
	script      = "text/javascript"
)

// Kinds is the kind of each media type, the first row matching: a type as
// it is, a whole family as type/*, a structured suffix as */*+suffix.
// What no row matches is a Download. An icon is a Picture only when it
// holds a PNG (ICOPicture).
//
//nolint:gochecknoglobals // the table, read only
var Kinds = []struct {
	Media string
	Kind  Kind
}{
	{html, Page},
	{"application/xhtml+xml", Page},
	{png, Picture},
	{jpeg, Picture},
	{gif, Picture},
	{webp, Picture},
	{svg, Picture},
	{icon, Picture},
	{"image/vnd.microsoft.icon", Picture},
	{"text/*", Text},
	{json, Text},
	{"application/javascript", Text},
	{"application/x-javascript", Text},
	{"application/ecmascript", Text},
	{"application/xml", Text},
	{"*/*+json", Text},
	{"*/*+xml", Text},
}

// extensions are the types an address's name hints at when nothing else
// says.
//
//nolint:gochecknoglobals // the table, read only
var extensions = map[string]string{
	".html": html, ".htm": html, ".css": "text/css", ".js": script,
	".mjs": script, ".json": json, ".xml": "text/xml", ".md": "text/markdown",
	".txt": plainText, ".svg": svg, ".png": png, ".jpg": jpeg,
	".jpeg": jpeg, ".gif": gif, ".webp": webp, ".ico": icon,
}

// Of is how the answer to address is shown and the media type it is taken
// for: the one its Content-Type header declares, unless it declares none,
// or bytes as application/octet-stream that are a picture, or as
// text/plain what is no text; then body's first bytes say, and for a type
// they leave open, the name in address.
func Of(header string, body []byte, address string) (Kind, string) {
	media := declared(header)
	switch media {
	case "":
		media = sniffed(body)
		if hint, ok := extensions[extension(address)]; ok && (media == octetStream || media == plainText ||
			strings.HasSuffix(media, "xml")) {
			media = hint
		}
	case octetStream:
		if seen := sniffed(body); kindOf(seen) == Picture {
			media = seen
		}
	case plainText:
		if !strings.HasPrefix(sniffed(body), "text/") {
			media = octetStream
		}
	}
	kind := kindOf(media)
	if kind == Picture && strings.Contains(media, "icon") && ICOPicture(body) == nil {
		kind = Download
	}
	return kind, media
}

func declared(header string) string {
	media, _, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return media
}

func sniffed(body []byte) string {
	return declared(http.DetectContentType(body))
}

func extension(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return strings.ToLower(path.Ext(u.Path))
}

func kindOf(media string) Kind {
	family, _, _ := strings.Cut(media, "/")
	for _, row := range Kinds {
		suffix, structured := strings.CutPrefix(row.Media, "*/*")
		if row.Media == media || row.Media == family+"/*" || structured && strings.HasSuffix(media, suffix) {
			return row.Kind
		}
	}
	return Download
}

// ICOPicture is the widest picture of an ICO file stored as a PNG, nil
// for a file that is no ICO or holds only bitmaps.
func ICOPicture(ico []byte) []byte {
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

// Drawable is a picture as a decoder of the kinds listed takes it: of an
// ICO file its PNG, any other as it is.
func Drawable(picture []byte) []byte {
	if png := ICOPicture(picture); png != nil {
		return png
	}
	return picture
}

// Attachment reports whether a Content-Disposition header asks for the
// answer to be saved rather than shown.
func Attachment(disposition string) bool {
	kind, _, _ := strings.Cut(disposition, ";")
	return strings.EqualFold(strings.TrimSpace(kind), "attachment")
}

// unnamed is the file name of an answer nothing names.
const unnamed = "download"

// FileName is the name an answer is saved under: the one its
// Content-Disposition header suggests, else the last part of address,
// without folders, leading dots or control characters, so it names a file
// in the folder it is saved to and nothing else.
func FileName(disposition, address string) string {
	if _, params, err := mime.ParseMediaType(disposition); err == nil {
		if name := safe(params["filename"]); name != "" {
			return name
		}
	}
	if u, err := url.Parse(address); err == nil {
		if name := safe(path.Base(u.Path)); name != "" {
			return name
		}
	}
	return unnamed
}

// maxName is the longest name a file is saved under, in bytes.
const maxName = 200

// reserved are the names Windows keeps for its devices, whatever their
// extension.
//
//nolint:gochecknoglobals // the list, read only
var reserved = []string{
	"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
}

// saved are the extensions of the types a name most often lacks one for.
//
//nolint:gochecknoglobals // the table, read only
var saved = map[string]string{
	html: ".html", "text/css": ".css", script: ".js", json: ".json", plainText: ".txt",
	svg: ".svg", png: ".png", jpeg: ".jpg", gif: ".gif", webp: ".webp",
	"application/pdf": ".pdf", "application/zip": ".zip", "application/gzip": ".gz", "audio/mpeg": ".mp3",
	"video/mp4": ".mp4",
}

// SaveName is the name a download is saved under on any system:
// FileName, with the extension of media when it has none, never a name
// Windows reserves nor a character it refuses, and no longer than a file
// name may be.
func SaveName(disposition, address, media string) string {
	name := FileName(disposition, address)
	if path.Ext(name) == "" {
		kind, _, _ := mime.ParseMediaType(media)
		name += saved[kind]
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	stem, _, _ := strings.Cut(name, ".")
	for _, device := range reserved {
		if strings.EqualFold(stem, device) {
			name = "_" + name
		}
	}
	return shortened(name)
}

// shortened is name cut to maxName bytes, its extension kept.
func shortened(name string) string {
	ext := path.Ext(name)
	if len(name) <= maxName || len(ext) >= maxName {
		return name
	}
	stem := name[:maxName-len(ext)]
	return strings.ToValidUTF8(stem, "") + ext
}

func safe(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	return strings.TrimSpace(strings.TrimLeft(name, ". "))
}

// FreeName is name, or name with the first number in brackets before its
// extension that taken does not report: "map.png", "map (1).png"….
func FreeName(name string, taken func(name string) bool) string {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	free := name
	for i := 1; taken(free); i++ {
		free = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
	return free
}

// tabStop is how many columns apart the stops of a tab are.
const tabStop = 8

// Spaced is text as ui can draw it: each tab as the spaces to its next
// stop, each line ended as on Unix. Its faces have neither a tab nor a
// carriage return.
func Spaced(text string) string {
	if !strings.ContainsAny(text, "\t\r") {
		return text
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var b strings.Builder
	column := 0
	for _, r := range text {
		switch r {
		case '\t':
			n := tabStop - column%tabStop
			b.WriteString(strings.Repeat(" ", n))
			column += n
		case '\n':
			b.WriteRune(r)
			column = 0
		default:
			b.WriteRune(r)
			column++
		}
	}
	return b.String()
}
