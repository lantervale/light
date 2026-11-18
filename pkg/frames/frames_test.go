package frames_test

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/frames"
)

func at(t *testing.T, address string) *url.URL {
	t.Helper()
	u, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func chain(t *testing.T, addresses ...string) []frames.Ancestor {
	t.Helper()
	var out []frames.Ancestor
	for _, a := range addresses {
		if a == "opaque" {
			out = append(out, frames.Ancestor{Opaque: true})
			continue
		}
		out = append(out, frames.Ancestor{URL: at(t, a)})
	}
	return out
}

func TestATabTakesSoManyFramesAndSoDeep(t *testing.T) {
	for _, c := range []struct {
		depth, count int
		want         error
	}{
		{1, 0, nil}, {frames.MaxDepth, frames.MaxFrames - 1, nil},
		{frames.MaxDepth + 1, 0, frames.ErrTooDeep}, {1, frames.MaxFrames, frames.ErrTooMany},
	} {
		if got := frames.Admit(c.depth, c.count); !errors.Is(got, c.want) {
			t.Errorf("at depth %d with %d frames: %v, want %v", c.depth, c.count, got, c.want)
		}
	}
}

func TestWhatAnIframeShows(t *testing.T) {
	for _, c := range []struct {
		src    string
		srcdoc bool
		want   frames.Kind
	}{
		{"", false, frames.Blank}, {"  ", false, frames.Blank}, {"about:blank", false, frames.Blank},
		{"ABOUT:BLANK#top", false, frames.Blank}, {"about:blank?x", false, frames.Blank},
		{"about:blankly", false, frames.Remote}, {"/page", false, frames.Remote},
		{"/page", true, frames.Inline}, {"", true, frames.Inline},
	} {
		if got := frames.Shown(c.src, c.srcdoc); got != c.want {
			t.Errorf("src %q, srcdoc %v: %v, want %v", c.src, c.srcdoc, got, c.want)
		}
	}
}

func TestAFramesAddressIsReadFromItsDocuments(t *testing.T) {
	base := at(t, "https://site.test/a/page.html")
	for ref, want := range map[string]string{
		"frame.html": "https://site.test/a/frame.html", "/top": "https://site.test/top",
		"//other.test/x": "https://other.test/x", "\thttp://plain.test/\n": "http://plain.test/",
		"javascript:alert(1)": "", "data:text/html,<p>x": "", "file:///etc/passwd": "", "light://settings": "",
		"view-source:https://site.test/": "", "blob:https://site.test/1": "", "about:srcdoc": "", "http://": "",
	} {
		got, err := frames.Resolve(base, ref)
		switch {
		case want == "" && !errors.Is(err, frames.ErrScheme):
			t.Errorf("%q: %v %v, want it refused", ref, got, err)
		case want != "" && (err != nil || got.String() != want):
			t.Errorf("%q: %v %v, want %s", ref, got, err, want)
		}
	}
}

func TestAFrameDoesNotShowAPageItIsIn(t *testing.T) {
	ancestors := []*url.URL{at(t, "https://site.test/inner?x=1"), nil, at(t, "https://site.test/")}
	for address, want := range map[string]bool{
		"https://site.test/": true, "https://site.test/#part": true, "https://site.test/inner?x=1#a": true,
		"https://site.test/inner": false, "https://site.test/inner?x=2": false, "http://site.test/": false,
	} {
		if got := frames.Recursive(at(t, address), ancestors); got != want {
			t.Errorf("%s: recursive %v, want %v", address, got, want)
		}
	}
}

func TestOriginsAndSites(t *testing.T) {
	for address, want := range map[string][2]string{
		"https://www.Shop.example.com/a":  {"https://www.shop.example.com", "https://example.com"},
		"https://shop.example.com:443/":   {"https://shop.example.com", "https://example.com"},
		"http://shop.example.com:8080/":   {"http://shop.example.com:8080", "http://example.com"},
		"http://127.0.0.1:9000/":          {"http://127.0.0.1:9000", "http://127.0.0.1"},
		"https://user.github.io/":         {"https://user.github.io", "https://user.github.io"},
		"https://[::1]:8443/":             {"https://[::1]:8443", "https://::1"},
		"light://settings":                {"null", ""},
		"data:text/html,x":                {"null", ""},
		"https://example.co.uk./deep/one": {"https://example.co.uk.", "https://example.co.uk"},
	} {
		u := at(t, address)
		if got := [2]string{frames.Origin(u), frames.Site(u)}; got != want {
			t.Errorf("%s: origin and site %v, want %v", address, got, want)
		}
	}
	if frames.Origin(nil) != "null" || frames.Site(nil) != "" {
		t.Error("no address has an origin or a site")
	}
}

func TestTheCookiesOfAFrameAreKeptUnderItsTopLevelSite(t *testing.T) {
	top := at(t, "https://news.example/today")
	for _, c := range []struct {
		name  string
		chain []*url.URL
		want  string
	}{
		{"a frame of the page's own site", []*url.URL{at(t, "https://static.news.example/box")}, ""},
		{"a frame of another site", []*url.URL{at(t, "https://video.example/1")}, "https://news.example"},
		{"the page's site in another's frame", []*url.URL{at(t, "https://news.example/in"),
			at(t, "https://ads.example/slot")}, "https://news.example"},
		{"the same host over http", []*url.URL{at(t, "http://news.example/")}, "https://news.example"},
		{"a document with no origin", []*url.URL{nil}, "https://news.example"},
		{"no frame", nil, ""},
	} {
		if got := frames.Partition(top, c.chain); got != c.want {
			t.Errorf("%s: partition %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAnHTTPSPageShowsNoFrameInTheClear(t *testing.T) {
	secure, plain := []*url.URL{at(t, "https://site.test/")}, []*url.URL{at(t, "http://site.test/")}
	deep := []*url.URL{at(t, "http://mid.test/"), nil, at(t, "https://site.test/")}
	for _, c := range []struct {
		ancestors []*url.URL
		to        string
		want      bool
	}{
		{secure, "http://other.test/", true}, {secure, "https://other.test/", false},
		{plain, "http://other.test/", false}, {deep, "http://other.test/", true},
		{secure, "http://localhost:8080/", false}, {secure, "http://127.0.0.1/", false},
		{secure, "http://app.localhost/", false}, {secure, "http://[::1]/", false},
		{secure, "http://192.168.1.1/", true},
	} {
		if got := frames.Mixed(c.ancestors, at(t, c.to)); got != c.want {
			t.Errorf("%s under %v: mixed %v, want %v", c.to, c.ancestors, got, c.want)
		}
	}
}

func TestWhereATargetLeads(t *testing.T) {
	for target, want := range map[string]frames.Target{
		"": frames.Self, "_self": frames.Self, "_SELF": frames.Self, "_blank": frames.Self,
		"_parent": frames.Parent, "_Top": frames.Top, "_top": frames.Top, "player": frames.Named,
		"_other": frames.Named,
	} {
		if got := frames.TargetOf(target); got != want {
			t.Errorf("target %q: %v, want %v", target, got, want)
		}
	}
}

func TestASandboxAllowsWhatItsTokensName(t *testing.T) {
	none := frames.ParseSandbox("", false)
	if none.On || none.Opaque() || !none.Submits() || !none.Saves() || !none.NavigatesTop(false) {
		t.Errorf("a frame with no sandbox is held back: %+v", none)
	}
	empty := frames.ParseSandbox("", true)
	if !empty.On || !empty.Opaque() || empty.Submits() || empty.Saves() || empty.NavigatesTop(true) ||
		empty.Scripts {
		t.Errorf("an empty sandbox allows something: %+v", empty)
	}
	some := frames.ParseSandbox(" Allow-Forms  allow-same-origin allow-nonsense\t"+
		"allow-top-navigation-by-user-activation", true)
	if some.Opaque() || !some.Submits() || some.Saves() || some.NavigatesTop(false) || !some.NavigatesTop(true) ||
		some.Scripts {
		t.Errorf("the tokens were not read: %+v", some)
	}
	all := frames.ParseSandbox("allow-scripts allow-same-origin allow-forms allow-popups "+
		"allow-popups-to-escape-sandbox allow-downloads allow-modals allow-top-navigation", true)
	want := frames.Sandbox{On: true, Scripts: true, SameOrigin: true, Forms: true, Popups: true, PopupsEscape: true,
		Downloads: true, Modals: true, TopNavigation: true}
	if all != want {
		t.Errorf("every token: %+v, want %+v", all, want)
	}
}

func TestAFrameInASandboxedOneIsLeftWhatBothAllow(t *testing.T) {
	parent := frames.ParseSandbox("allow-forms allow-same-origin", true)
	free := frames.ParseSandbox("", false)
	if got := free.Under(parent); got != parent {
		t.Errorf("a frame with no sandbox under a sandboxed one: %+v, want its parent's %+v", got, parent)
	}
	child := frames.ParseSandbox("allow-forms allow-downloads allow-top-navigation", true)
	got := child.Under(parent)
	if want := (frames.Sandbox{On: true, Forms: true}); got != want {
		t.Errorf("under its parent: %+v, want %+v", got, want)
	}
	if got = child.Under(free); got != child {
		t.Errorf("under a frame with no sandbox: %+v, want its own %+v", got, child)
	}
}

func TestWhetherADocumentLetsItselfBeFramed(t *testing.T) {
	const framed = "https://bank.example/login"
	same := []string{"https://bank.example/"}
	other := []string{"https://evil.example/"}
	nested := []string{"https://bank.example/widget", "https://evil.example/"}
	for _, c := range []struct {
		name      string
		header    map[string][]string
		ancestors []string
		refused   string
	}{
		{"no header", nil, other, ""},
		{"DENY", map[string][]string{"X-Frame-Options": {"DENY"}}, same, "'deny'"},
		{"deny in small letters and spaces", map[string][]string{"X-Frame-Options": {"  deny "}}, same, "'deny'"},
		{"SAMEORIGIN in its own site", map[string][]string{"X-Frame-Options": {"SAMEORIGIN"}}, same, ""},
		{"SAMEORIGIN in another", map[string][]string{"X-Frame-Options": {"SAMEORIGIN"}}, other, "'sameorigin'"},
		{"SAMEORIGIN with another site above its parent", map[string][]string{"X-Frame-Options": {"SAMEORIGIN"}},
			nested, "'sameorigin'"},
		{"SAMEORIGIN on another port", map[string][]string{"X-Frame-Options": {"SAMEORIGIN"}},
			[]string{"https://bank.example:8443/"}, "'sameorigin'"},
		{"SAMEORIGIN over another scheme", map[string][]string{"X-Frame-Options": {"SAMEORIGIN"}},
			[]string{"http://bank.example/"}, "'sameorigin'"},
		{"SAMEORIGIN in a sandboxed frame", map[string][]string{"X-Frame-Options": {"SAMEORIGIN"}},
			[]string{"opaque", "https://bank.example/"}, "'sameorigin'"},
		{"ALLOW-FROM is read as nothing", map[string][]string{"X-Frame-Options": {"ALLOW-FROM https://a.example"}},
			other, ""},
		{"ALLOWALL", map[string][]string{"X-Frame-Options": {"ALLOWALL"}}, other, ""},
		{"an unknown value", map[string][]string{"X-Frame-Options": {"NOPE"}}, other, ""},
		{"two that disagree", map[string][]string{"X-Frame-Options": {"SAMEORIGIN, DENY"}}, same, "'sameorigin, deny'"},
		{"two headers that disagree", map[string][]string{"X-Frame-Options": {"SAMEORIGIN", "ALLOWALL"}}, same,
			"'sameorigin, allowall'"},
		{"the same value twice", map[string][]string{"X-Frame-Options": {"SAMEORIGIN", "sameorigin"}}, same, ""},
		{"a known and an unknown value", map[string][]string{"X-Frame-Options": {"SAMEORIGIN, NOPE"}}, same,
			"'sameorigin, nope'"},
		{"two unknown values", map[string][]string{"X-Frame-Options": {"NOPE, NADA"}}, other, ""},

		{"frame-ancestors 'none'", map[string][]string{"Content-Security-Policy": {"frame-ancestors 'none'"}}, same,
			`"frame-ancestors 'none'"`},
		{"frame-ancestors with no source", map[string][]string{"Content-Security-Policy": {"frame-ancestors"}}, same,
			`"frame-ancestors"`},
		{"'self' in its own site", map[string][]string{"Content-Security-Policy": {"frame-ancestors 'self'"}}, same,
			""},
		{"'self' in another", map[string][]string{"Content-Security-Policy": {"frame-ancestors 'self'"}}, other,
			`"frame-ancestors 'self'"`},
		{"'self' with another site above its parent",
			map[string][]string{"Content-Security-Policy": {"default-src 'none'; frame-ancestors 'self'"}}, nested,
			`"frame-ancestors 'self'"`},
		{"'self' and the other site, both above", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors 'self' https://evil.example"}}, nested, ""},
		{"a host", map[string][]string{"Content-Security-Policy": {"frame-ancestors https://evil.example"}}, other, ""},
		{"a host that is not the ancestor's", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors https://good.example"}}, other, `"frame-ancestors https://good.example"`},
		{"a host without a scheme takes the document's", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors evil.example"}}, other, ""},
		{"a host without a scheme, the ancestor over http", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors evil.example"}}, []string{"http://evil.example/"}, `"frame-ancestors evil.example"`},
		{"an http source names https too", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors http://evil.example"}}, other, ""},
		{"a wildcard host", map[string][]string{"Content-Security-Policy": {"frame-ancestors https://*.example"}},
			other, ""},
		{"a wildcard is not the domain itself", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors https://*.evil.example"}}, other, `"frame-ancestors https://*.evil.example"`},
		{"a port", map[string][]string{"Content-Security-Policy": {"frame-ancestors https://evil.example:8443"}},
			[]string{"https://evil.example:8443/"}, ""},
		{"another port", map[string][]string{"Content-Security-Policy": {"frame-ancestors https://evil.example"}},
			[]string{"https://evil.example:8443/"}, `"frame-ancestors https://evil.example"`},
		{"any port", map[string][]string{"Content-Security-Policy": {"frame-ancestors https://evil.example:*"}},
			[]string{"https://evil.example:8443/"}, ""},
		{"a path is not read", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors https://evil.example/only/here"}}, other, ""},
		{"a scheme", map[string][]string{"Content-Security-Policy": {"frame-ancestors https:"}}, other, ""},
		{"another scheme", map[string][]string{"Content-Security-Policy": {"frame-ancestors https:"}},
			[]string{"http://evil.example/"}, `"frame-ancestors https:"`},
		{"a star", map[string][]string{"Content-Security-Policy": {"frame-ancestors *"}}, other, ""},
		{"a star, in a sandboxed frame", map[string][]string{"Content-Security-Policy": {"frame-ancestors *"}},
			[]string{"opaque", "https://evil.example/"}, `"frame-ancestors *"`},
		{"the directive in capitals", map[string][]string{"Content-Security-Policy": {"FRAME-ANCESTORS 'NONE'"}},
			same, `"frame-ancestors 'NONE'"`},
		{"two policies, one refusing", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors 'self', frame-ancestors 'none'"}}, same, `"frame-ancestors 'none'"`},
		{"two headers, one refusing", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors *", "frame-ancestors 'self'"}}, other, `"frame-ancestors 'self'"`},
		{"the first directive of a policy counts", map[string][]string{"Content-Security-Policy": {
			"frame-ancestors 'self'; frame-ancestors 'none'"}}, same, ""},
		{"a policy that only reports", map[string][]string{"Content-Security-Policy-Report-Only": {
			"frame-ancestors 'none'"}}, other, ""},
		{"a policy without the directive leaves X-Frame-Options to decide", map[string][]string{
			"Content-Security-Policy": {"default-src 'self'"}, "X-Frame-Options": {"DENY"}}, same, "'deny'"},
		{"frame-ancestors wins over X-Frame-Options", map[string][]string{
			"Content-Security-Policy": {"frame-ancestors https://evil.example"}, "X-Frame-Options": {"DENY"}}, other,
			""},
		{"frame-ancestors refuses where X-Frame-Options would not", map[string][]string{
			"Content-Security-Policy": {"frame-ancestors 'none'"}, "X-Frame-Options": {"ALLOWALL"}}, same,
			`"frame-ancestors 'none'"`},
	} {
		header := http.Header{}
		for name, values := range c.header {
			for _, v := range values {
				header.Add(name, v)
			}
		}
		err := frames.Allowed(header, at(t, framed), chain(t, c.ancestors...))
		var refused *frames.RefusedError
		switch {
		case c.refused == "" && err != nil:
			t.Errorf("%s: refused, %v", c.name, err)
		case c.refused != "" && (!errors.As(err, &refused) || !strings.Contains(err.Error(), c.refused) ||
			!strings.Contains(err.Error(), "'"+framed+"'")):
			t.Errorf("%s: %v, want it refused naming %s", c.name, err, c.refused)
		}
	}
}

func TestARefusalReadsAsChromesConsoleDoes(t *testing.T) {
	header := http.Header{"X-Frame-Options": {"DENY"}}
	err := frames.Allowed(header, at(t, "https://bank.example/"), chain(t, "https://evil.example/"))
	if want := "Refused to display 'https://bank.example/' in a frame because it set 'X-Frame-Options' to " +
		"'deny'."; err == nil || err.Error() != want {
		t.Errorf("X-Frame-Options: %v, want %s", err, want)
	}
	header = http.Header{"Content-Security-Policy": {"frame-ancestors 'self'"}}
	err = frames.Allowed(header, at(t, "https://bank.example/"), chain(t, "https://evil.example/"))
	if want := "Refused to frame 'https://bank.example/' because an ancestor violates the following Content " +
		`Security Policy directive: "frame-ancestors 'self'".`; err == nil || err.Error() != want {
		t.Errorf("frame-ancestors: %v, want %s", err, want)
	}
}
