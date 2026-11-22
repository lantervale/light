package web //nolint:testpackage // builds a page with the package's own loader

import "testing"

func TestAFragmentIndicatesAnElementAsBrowsersFindIt(t *testing.T) {
	doc := (&Loader{Frame: &Frame{}}).Local("https://site.test/", `<p id="first">one</p>
		<a id="link" name="first">not this</a><a id="named" name="notes">notes</a><p id="two words">two</p>
		<p id="a%20b">kept escaped</p>
		<p id="é">accent</p><p id="x" name="orphan">no anchor</p>`, Links{}).Doc
	root := doc.Body().Parent()
	for address, want := range map[string]string{
		"https://site.test/#first": "first", "https://site.test/#notes": "named",
		"https://site.test/#two%20words": "two words", "https://site.test/#a%20b": "a%20b",
		"https://site.test/#%C3%A9": "é", "https://site.test/#orphan": "", "https://site.test/#First": "",
		"https://site.test/#missing": "",
	} {
		got := ""
		if n := Indicated(doc, address); n != nil {
			got = n.ID()
		}
		if got != want {
			t.Errorf("%s indicates #%s, want #%s", address, got, want)
		}
	}
	for _, address := range []string{"https://site.test/", "https://site.test/#", "https://site.test/#top",
		"https://site.test/#TOP"} {
		if Indicated(doc, address) != root {
			t.Errorf("%s does not indicate the top of the page", address)
		}
	}
	top := (&Loader{Frame: &Frame{}}).Local("https://site.test/", `<p>one</p><p id="top">named top</p>`, Links{}).Doc
	if n := Indicated(top, "https://site.test/#top"); n == nil || n.ID() != "top" {
		t.Error("#top indicates the top of a page that has an element of that id")
	}
}

func TestAPlaceOfAPageIsItsAddressWithAFragment(t *testing.T) {
	for _, c := range []struct {
		from, to string
		want     bool
	}{
		{"https://site.test/a", "https://site.test/a#b", true},
		{"https://site.test/a#b", "https://site.test/a#c", true},
		{"https://site.test/a#b", "https://site.test/a#", true},
		{"https://site.test/a#b", "https://site.test/a", false},
		{"https://site.test/a", "https://site.test/a?x=1#b", false},
		{"https://site.test/a", "https://site.test/b#b", false},
	} {
		if got := SamePage(c.from, c.to); got != c.want {
			t.Errorf("SamePage(%q, %q) is %v", c.from, c.to, got)
		}
	}
}

func TestTheElementAFragmentNamesIsTheTargetOfItsPage(t *testing.T) {
	doc := (&Loader{Frame: &Frame{}}).Local("https://site.test/", `<p id="first">one</p>
		<a id="named" name="notes">notes</a><p id="two words">two</p>`, Links{}).Doc
	for _, c := range []struct{ address, want string }{
		{"https://site.test/#first", "first"}, {"https://site.test/#notes", "named"},
		{"https://site.test/#two%20words", "two words"}, {"https://site.test/#missing", ""},
		{"https://site.test/#first", "first"}, {"https://site.test/#top", ""},
		{"https://site.test/#first", "first"}, {"https://site.test/#", ""},
		{"https://site.test/#first", "first"}, {"https://site.test/", ""},
	} {
		Reveal(doc, c.address)
		got := ""
		if n := doc.Target(); n != nil {
			got = n.ID()
		}
		if got != c.want {
			t.Errorf("at %s the target is #%s, want #%s", c.address, got, c.want)
		}
	}
}
