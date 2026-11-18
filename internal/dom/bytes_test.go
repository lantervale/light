package dom_test

import (
	"testing"

	"github.com/lantervale/light/internal/dom"
)

func TestTheTextOfBytes(t *testing.T) {
	for _, c := range []struct{ raw, label, want string }{
		{"caf\xc3\xa9", "utf-8", "café"},
		{"caf\xe9", "iso-8859-1", "café"},
		{"caf\xe9", "ISO-8859-1", "café"},
		{"caf\xe9", "windows-1251", "cafй"},
		{"caf\xe9", "utf-8", "caf�"},
		{"caf\xe9", "no-such-charset", "caf�"},
		{"a\xff\xffb", "utf-8", "a��b"},
		{"a\xe2\x82b", "utf-8", "a�b"},
		{"\xef\xbb\xbfcaf\xc3\xa9", "iso-8859-1", "café"},
		{"\xff\xfeh\x00i\x00", "utf-8", "hi"},
		{"\xfe\xff\x00h\x00i", "iso-8859-1", "hi"},
		{"\x93\xfa\x96\x7b", "shift_jis", "日本"},
		{"a\x00b", "utf-8", "a\x00b"},
		{"", "utf-8", ""},
	} {
		if got := dom.Decode([]byte(c.raw), c.label); got != c.want {
			t.Errorf("%q as %s is %q, want %q", c.raw, c.label, got, c.want)
		}
	}
}
