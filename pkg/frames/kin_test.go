package frames_test

import (
	"net/url"
	"testing"

	"github.com/lantervale/light/pkg/frames"
)

func TestTheDocumentsOfOneOriginAndOnePlaceAreKin(t *testing.T) {
	at := func(address string) *url.URL {
		u, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	top := at("https://site.test/page")
	scripts := frames.Sandbox{On: true, Scripts: true}
	both := frames.Sandbox{On: true, Scripts: true, SameOrigin: true}
	key := func(origin string, sandbox frames.Sandbox, chain ...string) string {
		var documents []*url.URL
		for _, address := range chain {
			if address == "" {
				documents = append(documents, nil)
				continue
			}
			documents = append(documents, at(address))
		}
		got, kin := frames.Kin(top, documents, origin, sandbox)
		if !kin {
			if got != "" {
				t.Errorf("a document that is kin to none has the key %q", got)
			}
			return "none"
		}
		return got
	}
	page := key("https://site.test", frames.Sandbox{})
	for _, c := range []struct {
		name string
		got  string
		kin  bool
	}{
		{"the page is kin to itself", page, true},
		{"a frame of the page's origin", key("https://site.test", frames.Sandbox{}, "https://site.test/a"), true},
		{"one two frames down", key("https://site.test", frames.Sandbox{}, "https://site.test/a", "https://site.test/b"),
			true},
		{"one written in its iframe, at the address of the page", key("https://site.test", frames.Sandbox{},
			"https://site.test/page"), true},
		{"one of another port", key("https://site.test:8443", frames.Sandbox{}, "https://site.test:8443/a"), false},
		{"one of another scheme", key("http://site.test", frames.Sandbox{}, "http://site.test/a"), false},
		{"one of another host of the site", key("https://www.site.test", frames.Sandbox{}, "https://www.site.test/a"), false},
		{"one of another site", key("https://other.test", frames.Sandbox{}, "https://other.test/a"), false},
		{"one of the page's origin under another site",
			key("https://site.test", frames.Sandbox{}, "https://other.test/a", "https://site.test/b"), false},
		{"one of the page's origin under a document with no origin",
			key("https://site.test", frames.Sandbox{}, "", "https://site.test/b"), false},
		{"one with no origin", key("null", frames.Sandbox{}, "https://site.test/a"), false},
		{"one whose origin is not known", key("", frames.Sandbox{}, "https://site.test/a"), false},
		{"one in a sandbox that gives it no origin", key("https://site.test", scripts, "https://site.test/a"), false},
		{"one in a sandbox that keeps its origin", key("https://site.test", both, "https://site.test/a"), true},
		{"one in a sandbox that runs no script",
			key("https://site.test", frames.Sandbox{On: true, SameOrigin: true}, "https://site.test/a"), false},
	} {
		if (c.got == page) != c.kin {
			t.Errorf("%s: its key is %q, the page's %q", c.name, c.got, page)
		}
	}
	two := key("https://other.test", frames.Sandbox{}, "https://other.test/a")
	again := key("https://other.test", frames.Sandbox{}, "https://site.test/x", "https://other.test/b")
	if two == "none" || again != two {
		t.Errorf("two frames of another site have the keys %q and %q", two, again)
	}
	under := key("https://site.test", frames.Sandbox{}, "https://other.test/a", "https://site.test/b")
	other := key("https://site.test", frames.Sandbox{}, "https://other.test/c", "https://site.test/d")
	if under == "none" || under != other {
		t.Errorf("two frames of the page's origin under another site are not kin to each other: %q", under)
	}
}
