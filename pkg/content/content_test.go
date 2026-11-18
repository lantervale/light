package content_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/content"
)

func picture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// ico is an ICO file of one picture, pic.
func ico(pic []byte) []byte {
	b := binary.LittleEndian.AppendUint16(nil, 0)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = append(b, 16, 16, 0, 0, 1, 0, 32, 0)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(pic)))
	b = binary.LittleEndian.AppendUint32(b, 22)
	return append(b, pic...)
}

func TestEveryListedTypeIsShownAsItsKind(t *testing.T) {
	seen := make(map[content.Kind]bool)
	for _, row := range content.Kinds {
		media := strings.Replace(row.Media, "*", "x-anything", 1)
		body := []byte("plain words")
		if strings.Contains(media, "icon") {
			body = ico(picture(t))
		}
		kind, got := content.Of(media+"; charset=utf-8", body, "https://site.test/file")
		if kind != row.Kind || got != media {
			t.Errorf("%s is shown as %v (%s), want %v", row.Media, kind, got, row.Kind)
		}
		seen[row.Kind] = true
	}
	for _, kind := range []content.Kind{content.Page, content.Picture, content.Text} {
		if !seen[kind] {
			t.Errorf("no type is shown as %v", kind)
		}
	}
}

func TestWhatNoRowListsIsADownload(t *testing.T) {
	for _, media := range []string{"application/octet-stream", "application/pdf", "application/zip", "audio/mpeg",
		"video/mp4", "font/woff2", "image/webp", "image/avif", "image/bmp", "application/wasm"} {
		if kind, _ := content.Of(media, []byte{0, 1, 2}, "https://site.test/file"); kind != content.Download {
			t.Errorf("%s is shown as %v, want a download", media, kind)
		}
	}
}

func TestATypeMissingOrLyingIsSniffed(t *testing.T) {
	binary := []byte{0x00, 0x01, 0xfe, 0xff, 0x00, 0x10}
	for name, c := range map[string]struct {
		header, address string
		body            []byte
		kind            content.Kind
		media           string
	}{
		"no type, a page":    {"", "https://s.test/", []byte("<!doctype html><p>hi"), content.Page, "text/html"},
		"no type, a picture": {"", "https://s.test/x", picture(t), content.Picture, "image/png"},
		"no type, words":     {"", "https://s.test/notes", []byte("words"), content.Text, "text/plain"},
		"no type, a sheet by its name": {"", "https://s.test/site.css?v=2", []byte("a { color: red }"), content.Text,
			"text/css"},
		"no type, an svg by its name": {"", "https://s.test/logo.SVG", []byte("<svg xmlns='x'/>"), content.Picture,
			"image/svg+xml"},
		"no type, bytes":           {"", "https://s.test/blob", binary, content.Download, "application/octet-stream"},
		"a header no one can read": {";;", "https://s.test/", []byte("<html><body>"), content.Page, "text/html"},
		"octet-stream that is a png": {"application/octet-stream", "https://s.test/x", picture(t), content.Picture,
			"image/png"},
		"octet-stream that is a page": {"application/octet-stream", "https://s.test/x.html", []byte("<html>"),
			content.Download, "application/octet-stream"},
		"text/plain that is bytes": {"text/plain", "https://s.test/x.txt", binary, content.Download,
			"application/octet-stream"},
		"text/plain that looks a page": {"text/plain", "https://s.test/x", []byte("<html><p>shown as text"), content.Text,
			"text/plain"},
		"a declared type wins the name": {
			"text/plain", "https://s.test/site.css", []byte("a {}"), content.Text, "text/plain",
		},
		"an ico of bitmaps alone": {"image/x-icon", "https://s.test/favicon.ico", ico([]byte("BM")), content.Download,
			"image/x-icon"},
		"upper case": {"TEXT/HTML", "https://s.test/", nil, content.Page, "text/html"},
	} {
		if kind, media := content.Of(c.header, c.body, c.address); kind != c.kind || media != c.media {
			t.Errorf("%s: %v (%s), want %v (%s)", name, kind, media, c.kind, c.media)
		}
	}
}

func TestAnIcosWidestPNGIsItsPicture(t *testing.T) {
	pic := picture(t)
	if got := content.ICOPicture(ico(pic)); !bytes.Equal(got, pic) {
		t.Fatalf("got %d bytes of the %d", len(got), len(pic))
	}
	for _, data := range [][]byte{nil, []byte("not an ico"), ico([]byte("BM")), ico(pic)[:20]} {
		if content.ICOPicture(data) != nil {
			t.Errorf("%q has a picture", data)
		}
	}
	if !bytes.Equal(content.Drawable(ico(pic)), pic) || !bytes.Equal(content.Drawable(pic), pic) {
		t.Error("Drawable is not an ico's PNG, or changes a picture that is none")
	}
}

func TestAnAttachmentIsSavedUnderASafeName(t *testing.T) {
	for name, c := range map[string]struct{ disposition, address, want string }{
		"its file name":         {`attachment; filename="report.pdf"`, "https://s.test/get?id=3", "report.pdf"},
		"no folders":            {`attachment; filename="../../etc/passwd"`, "https://s.test/x", "passwd"},
		"nor a windows path":    {`attachment; filename="C:\\Users\\me\\a.txt"`, "https://s.test/x", "a.txt"},
		"no leading dots":       {`attachment; filename="...bashrc"`, "https://s.test/x", "bashrc"},
		"no control characters": {"attachment; filename=\"a\tb\x01.txt\"", "https://s.test/x", "ab.txt"},
		"an extended name":      {`attachment; filename*=UTF-8''cl%C3%A9.txt`, "https://s.test/x", "clé.txt"},
		"the address's name":    {"attachment", "https://s.test/files/archive.zip?dl=1", "archive.zip"},
		"an escaped address":    {"", "https://s.test/files/my%20file.bin", "my file.bin"},
		"nothing to name it by": {"", "https://s.test/", "download"},
		"only dots":             {`attachment; filename=".."`, "https://s.test/", "download"},
	} {
		if got := content.FileName(c.disposition, c.address); got != c.want {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
	if !content.Attachment(`Attachment; filename="a"`) || content.Attachment("inline") || content.Attachment("") {
		t.Error("Attachment misreads a disposition")
	}
}

func TestASavedFileTakesAFreeName(t *testing.T) {
	taken := map[string]bool{"map.png": true, "map (1).png": true, "notes": true, "a.tar.gz": true}
	has := func(name string) bool { return taken[name] }
	for name, want := range map[string]string{
		"new.png": "new.png", "map.png": "map (2).png", "notes": "notes (1)", "a.tar.gz": "a.tar (1).gz",
	} {
		if got := content.FreeName(name, has); got != want {
			t.Errorf("%s is saved as %q, want %q", name, got, want)
		}
	}
}

func TestASavedNameIsReadFromEveryFormAndMadeSafeForAnySystem(t *testing.T) {
	long := strings.Repeat("n", 300)
	for name, c := range map[string]struct{ disposition, address, media, want string }{
		"the UTF-8 form wins the plain one": {
			`attachment; filename="plan.pdf"; filename*=UTF-8''pl%C3%A0n%20final.pdf`, "https://s.test/x", "application/pdf",
			"plàn final.pdf",
		},
		"no extension takes the type's":     {"", "https://s.test/files/report", "application/pdf", "report.pdf"},
		"a typed name keeps its extension":  {"", "https://s.test/a.tar.gz", "application/gzip", "a.tar.gz"},
		"an unknown type adds nothing":      {"", "https://s.test/blob", "application/x-thing", "blob"},
		"nothing names it":                  {"", "https://s.test/", "text/html", "download.html"},
		"a reserved Windows name":           {`attachment; filename="CON.txt"`, "https://s.test/x", "text/plain", "_CON.txt"},
		"a reserved name without extension": {"", "https://s.test/nul", "", "_nul"},
		"characters no system takes": {`attachment; filename="a<b>:c|d?.txt"`, "https://s.test/x", "",
			"a_b__c_d_.txt"},
		"a name too long keeps its end": {"", "https://s.test/" + long + ".zip", "",
			strings.Repeat("n", 196) + ".zip"},
	} {
		if got := content.SaveName(c.disposition, c.address, c.media); got != c.want {
			t.Errorf("%s: saved as %q, want %q", name, got, c.want)
		}
	}
}

func TestSpacedTextHasItsTabsAsSpacesToTheNextStopAndUnixLines(t *testing.T) {
	for text, want := range map[string]string{
		"plain":          "plain",
		"\ta":            "        a",
		"ab\tc":          "ab      c",
		"12345678\tx":    "12345678        x",
		"a\r\n\tb\rc\td": "a\n        b\nc       d",
	} {
		if got := content.Spaced(text); got != want {
			t.Errorf("%q reads %q, not %q", text, got, want)
		}
	}
}
