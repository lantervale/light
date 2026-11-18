package dom

import (
	"net/url"
	"strings"
	"testing"
)

func TestSpecifiersAreAddressesOrNamesOfTheImportMap(t *testing.T) {
	page, _ := url.Parse("https://site.test/app/page.html")
	var m importMap
	refused, err := m.add(`{
		"imports": {
			"react": "/vendor/react.js",
			"lodash/": "https://cdn.test/lodash@4/",
			"lodash/special": "./special.js",
			"./local.js": "./mapped.js",
			"https://cdn.test/old.js": "https://cdn.test/new.js",
			"blocked": null,
			"bare-value": "nowhere",
			"slashless/": "/no-slash",
			"": "/empty.js"
		},
		"scopes": {
			"/app/legacy/": { "react": "/vendor/react-16.js" },
			"/app/legacy/deep/": { "react": "/vendor/react-15.js" },
			"/app/one.js": { "react": "/vendor/react-one.js" }
		}
	}`, page)
	if err != nil {
		t.Fatal(err)
	}
	if len(refused) != 3 {
		t.Errorf("the map refused %q, want the two names with a wrong address and the empty one", refused)
	}
	for _, c := range []struct{ specifier, from, want string }{
		{"./a.js", "https://site.test/app/main.js", "https://site.test/app/a.js"},
		{"../a.js", "https://site.test/app/main.js", "https://site.test/a.js"},
		{"/a.js?v=1#top", "https://site.test/app/main.js", "https://site.test/a.js?v=1#top"},
		{"https://other.test/a.js", "https://site.test/app/main.js", "https://other.test/a.js"},
		{"react", "https://site.test/app/main.js", "https://site.test/vendor/react.js"},
		{"react", "https://site.test/app/legacy/main.js", "https://site.test/vendor/react-16.js"},
		{"react", "https://site.test/app/legacy/deep/main.js", "https://site.test/vendor/react-15.js"},
		{"react", "https://site.test/app/one.js", "https://site.test/vendor/react-one.js"},
		{"react", "https://site.test/app/one.js.map", "https://site.test/vendor/react.js"},
		{"lodash/fp/map.js", "https://site.test/app/main.js", "https://cdn.test/lodash@4/fp/map.js"},
		{"lodash/special", "https://site.test/app/main.js", "https://site.test/app/special.js"},
		{"./local.js", "https://site.test/app/main.js", "https://site.test/app/mapped.js"},
		{"./local.js", "https://site.test/main.js", "https://site.test/local.js"},
		{"https://cdn.test/old.js", "https://site.test/app/main.js", "https://cdn.test/new.js"},
	} {
		from, _ := url.Parse(c.from)
		if got, failed := m.resolve(c.specifier, from); failed != nil || got != c.want {
			t.Errorf("%q in %s is %q, %v; want %q", c.specifier, c.from, got, failed, c.want)
		}
	}
	for specifier, why := range map[string]string{
		"vue": "Relative references must start with", "reactive": "Relative references", "blocked": "no address",
		"bare-value": "no address", "slashless/x.js": "no address", "lodash/../../escape.js": "no address",
		"": "Relative references",
	} {
		from, _ := url.Parse("https://site.test/app/main.js")
		if got, failed := m.resolve(specifier, from); failed == nil || !strings.Contains(failed.Error(), why) {
			t.Errorf("%q is %q, %v; want an error that says %q", specifier, got, failed, why)
		}
	}
}

func TestALaterImportMapAddsNamesAndChangesNone(t *testing.T) {
	page, _ := url.Parse("https://site.test/")
	var m importMap
	if _, err := m.add(`{"imports": {"a": "/first/a.js"}, "scopes": {"/x/": {"a": "/first/x/a.js"}}}`, page); err != nil {
		t.Fatal(err)
	}
	if _, err := m.add(`{"imports": {"a": "/second/a.js", "b": "/second/b.js"},
		"scopes": {"/x/": {"a": "/second/x/a.js", "b": "/second/x/b.js"}}}`, page); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ specifier, from, want string }{
		{"a", "https://site.test/m.js", "https://site.test/first/a.js"},
		{"b", "https://site.test/m.js", "https://site.test/second/b.js"},
		{"a", "https://site.test/x/m.js", "https://site.test/first/x/a.js"},
		{"b", "https://site.test/x/m.js", "https://site.test/second/x/b.js"},
	} {
		from, _ := url.Parse(c.from)
		if got, err := m.resolve(c.specifier, from); err != nil || got != c.want {
			t.Errorf("%q in %s is %q, %v; want %q", c.specifier, c.from, got, err, c.want)
		}
	}
	if _, err := m.add(`{"imports": [`, page); err == nil {
		t.Error("an import map that is no JSON was read")
	}
	if _, err := (&importMap{}).resolve("a", page); err == nil {
		t.Error("a name resolved without an import map")
	}
}
