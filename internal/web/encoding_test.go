package web_test

import (
	"testing"

	"github.com/lantervale/light/internal/web"
)

func TestAPageSaysTheEncodingItCameIn(t *testing.T) {
	for _, c := range []struct{ body, contentType, want string }{
		{"<p>plain", "text/html", "windows-1252"},
		{"<p>été", "text/html", "UTF-8"},
		{"<p>plain", "text/html; charset=latin1", "windows-1252"},
		{`<meta charset="cp866"><p>`, "text/html", "IBM866"},
		{`<meta charset="sjis"><p>`, "", "Shift_JIS"},
		{`<meta charset="big5"><p>`, "", "Big5"},
		{`<meta charset="koi8"><p>`, "", "KOI8-R"},
		{`<meta charset="gb2312"><p>`, "", "GBK"},
		{`<meta charset="gb18030"><p>`, "", "gb18030"},
		{`<meta charset="utf-16"><p>`, "", "UTF-8"},
		{`<meta charset="x-user-defined"><p>`, "", "windows-1252"},
		{"<p>", "text/html; charset=x-user-defined", "x-user-defined"},
		{`<meta charset="iso-8859-8-i"><p>`, "", "ISO-8859-8-I"},
		{`<meta charset="mac"><p>`, "text/html; charset=euc-kr", "EUC-KR"},
		{"\xff\xfe<\x00p\x00>\x00", "text/html; charset=utf-8", "UTF-16LE"},
	} {
		if got := web.EncodingOf([]byte(c.body), c.contentType); got != c.want {
			t.Errorf("%q as %q is in %s, want %s", c.body, c.contentType, got, c.want)
		}
	}
}
