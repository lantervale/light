package dom

import (
	"bytes"

	"golang.org/x/net/html/charset"
)

// Bytes cross between a script and light through a SharedArrayBuffer
// the script makes: an op is given it as the []byte it is, to read or
// to fill before it returns (pkg/script).

// utf8Into writes a text as UTF-8 into the buffer a script shares, and
// says how many bytes it took.
func utf8Into(a arguments) (any, error) {
	into, _ := a[1].([]byte)
	return copy(into, a.text(0)), nil
}

// decoded is the text of the bytes a script shares: UTF-8 when it names
// no encoding, as a fetch reads text, else what Decode makes of them.
func decoded(a arguments) (any, error) {
	raw, _ := a[0].([]byte)
	if label := a.text(1); label != "" {
		return Decode(raw, label), nil
	}
	return decodeAs(bytes.TrimPrefix(raw, marks[0].mark), utf8Name), nil
}

const utf8Name = "utf-8"

// marks are the byte order marks, and the encoding each tells of.
var marks = []struct { //nolint:gochecknoglobals // a constant table
	mark []byte
	name string
}{{[]byte{0xEF, 0xBB, 0xBF}, utf8Name}, {[]byte{0xFE, 0xFF}, "utf-16be"}, {[]byte{0xFF, 0xFE}, "utf-16le"}}

// Decode is the text of raw as an XMLHttpRequest reads it: in the
// encoding its byte order mark tells of, else the one label names, else
// UTF-8, without the mark; what is no text of that encoding is U+FFFD.
func Decode(raw []byte, label string) string {
	for _, m := range marks {
		if bytes.HasPrefix(raw, m.mark) {
			return decodeAs(raw[len(m.mark):], m.name)
		}
	}
	return decodeAs(raw, label)
}

func decodeAs(raw []byte, label string) string {
	e, _ := charset.Lookup(label)
	if e == nil {
		e, _ = charset.Lookup(utf8Name)
	}
	text, err := e.NewDecoder().Bytes(raw)
	if err != nil {
		return string(bytes.ToValidUTF8(raw, []byte("�")))
	}
	return string(text)
}

// answered fills the buffer a script shares with the body of the answer
// it is being told of.
func (r *Runtime) answered(a arguments) (any, error) {
	into, _ := a[0].([]byte)
	return copy(into, r.answering), nil
}
