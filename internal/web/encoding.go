package web

import (
	"strings"

	"golang.org/x/net/html/charset"
)

// EncodingOf is the name of the encoding a page came in, as the DOM
// spells it (UTF-8, windows-1252, Shift_JIS): what its Content-Type
// says, else what its HTML does, else what its bytes look like. HTML
// that says x-user-defined is read as windows-1252.
func EncodingOf(body []byte, contentType string) string {
	_, name, certain := charset.DetermineEncoding(body, contentType)
	if name == "x-user-defined" && !certain {
		name = "windows-1252"
	}
	return encodingName(name)
}

// encodingName spells the name of an encoding as the Encoding standard
// does: upper case, but for the few it writes otherwise.
func encodingName(name string) string {
	switch {
	case name == "":
		return "UTF-8"
	case name == "big5":
		return "Big5"
	case name == "shift_jis":
		return "Shift_JIS"
	case name == "gb18030", name == "macintosh", name == "replacement", strings.HasPrefix(name, "windows-"),
		strings.HasPrefix(name, "x-"):
		return name
	}
	return strings.ToUpper(name)
}
