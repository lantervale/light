package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"weak"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/console"
	"github.com/lantervale/light/pkg/frames"
	"github.com/lantervale/light/pkg/localnet"
)

const secret = `<p>the account</p><img src="secret.png" alt="s">`

func TestADocumentThatRefusesToBeFramedIsNotShown(t *testing.T) {
	xfo := func(value string) answer { return with(html(secret), "X-Frame-Options", value) }
	csp := func(value string) answer { return with(html(secret), "Content-Security-Policy", value) }
	cases := []struct {
		name, at string
		a        answer
		refused  bool
	}{
		{"deny", "bank.test/deny/", xfo("DENY"), true},
		{"sameorigin-elsewhere", "bank.test/so/", xfo("SAMEORIGIN"), true},
		{"sameorigin-at-home", "site.test/so/", xfo("sameorigin"), false},
		{"allow-from", "bank.test/from/", xfo("ALLOW-FROM https://nowhere.test"), false},
		{"disagreeing", "site.test/both/", xfo("SAMEORIGIN, DENY"), true},
		{"none", "site.test/none/", csp("frame-ancestors 'none'"), true},
		{"self-at-home", "site.test/self/", csp("default-src *; frame-ancestors 'self'"), false},
		{"self-elsewhere", "bank.test/self/", csp("frame-ancestors 'self'"), true},
		{"host", "bank.test/host/", csp("frame-ancestors https://site.test"), false},
		{"another-host", "bank.test/other/", csp("frame-ancestors https://partner.test"), true},
		{"wildcard", "bank.test/wild/", csp("frame-ancestors https://*.test"), false},
		{"scheme", "bank.test/scheme/", csp("frame-ancestors https:"), false},
		{"another-scheme", "bank.test/plain/", csp("frame-ancestors http://site.test:8080"), true},
		{"policy-over-options", "bank.test/wins/",
			with(csp("frame-ancestors https://site.test"), "X-Frame-Options", "DENY"), false},
		{"policy-refuses-over-options", "bank.test/loses/",
			with(csp("frame-ancestors 'none'"), "X-Frame-Options", "ALLOWALL"), true},
		{"report-only", "bank.test/report/", with(html(secret), "Content-Security-Policy-Report-Only",
			"frame-ancestors 'none'"), false},
		{"two-policies", "bank.test/two/", csp("frame-ancestors *\nframe-ancestors 'self'"), true},
	}
	pages := map[string]answer{
		"site.test/hop": moved("https://bank.test/deny/"),
		"other.test/mid": html(`<iframe id="nested-sameorigin" src="https://site.test/so2/"></iframe>` +
			`<iframe id="nested-one" src="https://bank.test/one/"></iframe>` +
			`<iframe id="nested-both" src="https://bank.test/all/"></iframe>`),
		"site.test/so2/": xfo("SAMEORIGIN"),
		"bank.test/one/": csp("frame-ancestors https://other.test"),
		"bank.test/all/": csp("frame-ancestors https://other.test https://site.test"),
	}
	top := `<iframe id="redirected" src="/hop"></iframe><iframe id="mid" src="https://other.test/mid"></iframe>`
	for _, c := range cases {
		pages[c.at] = c.a
		top += `<iframe id="` + c.name + `" src="https://` + c.at + `"></iframe>`
	}
	pages["site.test/"] = html(top)
	f, transport := framingOf(t, pages)
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	check := func(id, at string, refused bool) {
		t.Helper()
		text := s.framedText(id)
		switch {
		case refused && !strings.Contains(text, "refused to be shown in a frame"):
			t.Errorf("%s: the frame shows %q, want light's notice", id, text)
		case !refused && text != "the account":
			t.Errorf("%s: the frame shows %q, want the document", id, text)
		}
		if refused && s.reason("https://"+at) != "refused to be framed" {
			t.Errorf("%s: its request is not a blocked row: %q", id, s.reason("https://"+at))
		}
		if refused && !s.consoleSays(console.Error, "'https://"+at+"'") {
			t.Errorf("%s: the console says nothing of it", id)
		}
		if !refused && s.reason("https://"+at) != "" {
			t.Errorf("%s: its request is a blocked row", id)
		}
	}
	for _, c := range cases {
		check(c.name, c.at, c.refused)
	}
	s.framedText("mid")
	check("nested-sameorigin", "site.test/so2/", true)
	check("nested-one", "bank.test/one/", true)
	check("nested-both", "bank.test/all/", false)
	if text := s.framedText("redirected"); !strings.Contains(text, "bank.test refused to be shown in a frame") {
		t.Errorf("a frame redirected to a document that refuses shows %q", text)
	}
	s.quiet()
	for _, at := range []string{"bank.test/deny/secret.png", "bank.test/so/secret.png", "site.test/none/secret.png",
		"bank.test/self/secret.png", "bank.test/one/secret.png"} {
		if f.came(at) {
			t.Errorf("%s was asked for: a document that refused to be framed was read", at)
		}
	}
	if !f.came("bank.test/host/secret.png") {
		t.Error("the picture of a document that lets itself be framed was not asked for")
	}
	for _, want := range []string{
		"Refused to display 'https://bank.test/deny/' in a frame because it set 'X-Frame-Options' to 'deny'.",
		"Refused to display 'https://bank.test/so/' in a frame because it set 'X-Frame-Options' to 'sameorigin'.",
		"Refused to frame 'https://bank.test/self/' because an ancestor violates the following Content Security " +
			`Policy directive: "frame-ancestors 'self'".`,
		"Refused to frame 'https://bank.test/one/' because an ancestor violates the following Content Security " +
			`Policy directive: "frame-ancestors https://other.test".`,
	} {
		if !s.consoleSays(console.Error, want) {
			t.Errorf("the console does not say %q", want)
		}
	}
}

func TestTheCookiesOfAFrameAreItsTopLevelSites(t *testing.T) {
	box := with(html(`<p>a widget</p><img src="/pixel.png" alt="p">`), "Set-Cookie",
		"seen=1; SameSite=None; Secure; Path=/; Max-Age=600")
	f, transport := framingOf(t, map[string]answer{
		"news.test/": with(html(`<iframe id="widget" src="https://widget.test/box"></iframe>`+
			`<iframe id="own" src="/own"></iframe>`), "Set-Cookie", "session=1; Path=/\nstrict=1; Path=/; SameSite=Strict"),
		"news.test/own":         html(`<p>the site's own</p><img src="/own.png" alt="o">`),
		"news.test/own.png":     typed("image/png", string(picture(t, 2, 2))),
		"shop.test/":            html(`<iframe id="widget" src="https://widget.test/box"></iframe>`),
		"widget.test/box":       box,
		"widget.test/pixel.png": typed("image/png", string(picture(t, 2, 2))),
		"widget.test/":          with(html(`<p>the widget's own page</p>`), "Set-Cookie", "home=1; Path=/"),
		"widget.test/again":     html(`<p>again</p>`),
	})
	dir := t.TempDir()
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir(), Dir: dir})
	see := func(address string) {
		t.Helper()
		s.visit(address)
		s.framedText("widget")
		s.quiet()
	}
	see("https://news.test/")
	see("https://news.test/?again")
	if got := f.sent("widget.test/box"); got != 0 {
		t.Fatalf("with third parties blocked, a frame of another site sent %d cookies", got)
	}
	if got := f.sent("news.test/own"); got != 2 {
		t.Errorf("a frame of the page's own site sent %d cookies, want the site's 2", got)
	}
	if got := f.sent("news.test/own.png"); got != 2 {
		t.Errorf("the picture of a frame of the page's own site sent %d cookies, want the site's 2", got)
	}

	s.configure(func(c *config.Config) { c.ThirdPartyCookies = true })
	see("https://news.test/?third")
	if got := f.sent("widget.test/box"); got != 0 {
		t.Fatalf("the frame sent %d cookies before it was ever set one", got)
	}
	see("https://news.test/?fourth")
	if box, pixel := f.sent("widget.test/box"), f.sent("widget.test/pixel.png"); box != 1 || pixel != 1 {
		t.Fatalf("the frame sent %d cookies and its picture %d, want the one it was set under its page's site", box,
			pixel)
	}
	see("https://shop.test/")
	if got := f.sent("widget.test/box"); got != 0 {
		t.Fatalf("under another site's page the frame sent %d cookies: the two share a jar", got)
	}
	see("https://shop.test/?again")
	if got := f.sent("widget.test/box"); got != 1 {
		t.Fatalf("under the second site's page the frame sent %d cookies, want its own one", got)
	}
	s.visit("https://widget.test/")
	s.visit("https://widget.test/again")
	if got := f.sent("widget.test/again"); got != 1 {
		t.Fatalf("the widget's own page sent %d cookies, want the one it set itself and none of its frames'", got)
	}
	see("https://news.test/?fifth")
	if got := f.sent("widget.test/box"); got != 1 {
		t.Fatalf("after a visit of the widget's own site, its frame sent %d cookies, want its one", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again := showWith(t, Options{Transport: transport, Downloads: t.TempDir(), Dir: dir})
	again.visit("https://shop.test/?next-run")
	again.framedText("widget")
	if got := f.sent("widget.test/box"); got != 1 {
		t.Fatalf("in the next run the frame sent %d cookies, want the one kept under its page's site", got)
	}
}

func TestASandboxedFrameHasNoOriginAndDoesWhatItsTokensAllow(t *testing.T) {
	inner := `<body style="margin:0"><a id="top" href="/away" target="_top">top</a> <a id="pop" href="/away"
		target="_blank">new</a> <a id="save" href="/file.bin" download>save</a>
		<form action="/found"><input name="q" value="1"><button id="find">find</button></form>
		<img src="/pixel.png" alt="p">`
	f, transport := framingOf(t, map[string]answer{
		"site.test/": with(html(`<body style="margin:0">
			<iframe id="closed" sandbox src="/inner" width="400"></iframe>
			<iframe id="same" sandbox="allow-same-origin" src="/inner2" width="400"></iframe>
			<iframe id="open" sandbox="allow-forms allow-top-navigation allow-same-origin" src="/inner3" width="400">
			</iframe>`), "Set-Cookie", "session=1; Path=/"),
		"site.test/inner":     with(html(inner+`<iframe id="child" src="/child"></iframe>`), "Set-Cookie", "leak=1; Path=/"),
		"site.test/inner2":    html(inner),
		"site.test/inner3":    html(inner),
		"site.test/child":     html(`<p>a child</p>`),
		"site.test/pixel.png": typed("image/png", string(picture(t, 2, 2))),
		"site.test/found":     html(`<p>found</p>`),
		"site.test/away":      html(`<title>Away</title><p>away</p>`),
		"site.test/after":     html(`<p>after</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	for _, id := range []string{"closed", "same", "open", "child"} {
		s.framedText(id)
	}
	s.quiet()
	if got := f.sent("site.test/inner"); got != 0 {
		t.Errorf("a sandboxed frame sent %d cookies", got)
	}
	if got := f.sent("site.test/child"); got != 0 {
		t.Errorf("a frame in a sandboxed frame sent %d cookies", got)
	}
	if got := f.sent("site.test/inner2"); got != 1 {
		t.Errorf("a sandboxed frame allowed its origin sent %d cookies, want 1", got)
	}
	closed := s.frameAt("closed")
	if !closed.sandbox.Opaque() || !s.frameAt("child").sandbox.Opaque() || s.frameAt("same").sandbox.Opaque() {
		t.Error("the sandboxes were not read, or not handed down")
	}

	s.clickAt(s.inFrame(closed, "find"))
	s.clickAt(s.inFrame(closed, "top"))
	s.clickAt(s.inFrame(closed, "pop"))
	s.clickAt(s.inFrame(closed, "save"))
	for range 10 {
		s.frame()
	}
	if f.came("site.test/found") || f.came("site.test/away") || f.came("site.test/file.bin") {
		t.Error("a sandboxed frame sent a form, left its tab, opened a window or saved a file")
	}
	if got := s.browser.Tab().Current(); got != "https://site.test/" || closed.page.Address != "https://site.test/inner" {
		t.Errorf("the tab is at %s and the frame at %s", got, closed.page.Address)
	}
	for _, want := range []string{
		"Blocked form submission to 'https://site.test/found?q=1' because the form's frame is sandboxed and the " +
			"'allow-forms' permission is not set.",
		"The frame attempting navigation of the top-level window is sandboxed",
		"'allow-popups' permission is not set",
		"the flag 'allow-downloads' is not set",
	} {
		if !s.consoleSays(console.Error, want) && !s.consoleSays(console.Warn, want) {
			t.Errorf("the console does not say %q: %v", want, s.session().said.Lines())
		}
	}

	open := s.frameAt("open")
	s.clickAt(s.inFrame(open, "find"))
	if got := s.framedTextOf("open", "https://site.test/found?q=1"); got != "found" {
		t.Errorf("a sandboxed frame allowed forms shows %q", got)
	}
	s.navigate(open, wanted{from: open.in, ref: "/inner3"})
	s.framedTextOf("open", "https://site.test/inner3")
	s.clickAt(s.inFrame(s.frameAt("open"), "top"))
	s.until("a sandboxed frame allowed to took its tab elsewhere", func() bool {
		return s.browser.Tab().Current() == "https://site.test/away" && s.session().pages["https://site.test/away"] != nil
	})
	s.visit("https://site.test/after")
	if got := f.sent("site.test/after"); got != 1 {
		t.Errorf("the site sends %d cookies after its sandboxed frame was set one, want its own 1", got)
	}
}

func TestAPageOfTheInternetFramesNothingOfTheLocalNetwork(t *testing.T) {
	l := newLAN(t)
	site, other, router, here := l.serve(localnet.Public), l.serve(localnet.Public), l.serve(localnet.Private),
		l.serve(localnet.Loopback)
	l.show(site+"/", `<iframe id="router" src="`+router+`/admin"></iframe><iframe id="here" src="`+here+`/admin">
		</iframe><iframe id="turned" src="`+other+`/hop?to=`+router+`/reached"></iframe>
		<iframe id="public" src="`+other+`/page"></iframe>`)
	l.show(other+"/page", `<p>public</p><iframe id="deep" src="`+router+`/deep"></iframe>`)
	l.show(router+"/", `<iframe id="public" src="`+other+`/page2"></iframe><iframe id="here" src="`+here+`/admin">
		</iframe><iframe id="self" src="/admin"></iframe>`)
	l.show(router+"/admin", `<p>the router</p>`)
	l.show(other+"/page2", `<p>public</p>`)
	s := showWith(t, Options{Downloads: t.TempDir(), Space: l.space})
	s.visit(site + "/")
	s.blocked(map[string]string{router + "/admin": toLocal, here + "/admin": toLoopback, router + "/reached": toLocal,
		router + "/deep": toLocal})
	if got := l.got(router) + l.got(here); got != "" {
		t.Errorf("a page of the Internet framed the local network: %s", got)
	}
	for _, id := range []string{"router", "here", "turned", "deep"} {
		if text := s.framedText(id); !strings.Contains(text, "may not frame the network around this machine") {
			t.Errorf("the frame %s shows %q, want light's notice", id, text)
		}
	}
	if !s.consoleSays(console.Error, toLocal) {
		t.Errorf("the console does not say why: %v", s.session().said.Lines())
	}
	s.visit(router + "/")
	if text := s.framedText("self"); text != "the router" {
		t.Errorf("a page of the local network shows %q in a frame of its own network", text)
	}
	if text := s.framedText("public"); text != "public" {
		t.Errorf("a page of the local network shows %q in a frame of the Internet", text)
	}
	s.blocked(map[string]string{here + "/admin": strings.Replace(toLoopback, "`public`", "`local`", 1)})
}

func TestAnHTTPSPageShowsNoFrameSentInTheClear(t *testing.T) {
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="plain" src="http://plain.test/x"></iframe><iframe id="hop" src="/hop"></iframe>
			<iframe id="mid" src="https://other.test/mid"></iframe>`),
		"site.test/hop":   moved("http://plain.test/y"),
		"other.test/mid":  html(`<p>mid</p>`),
		"plain.test/x":    html(`<p>in the clear</p>`),
		"plain.test/y":    html(`<p>in the clear</p>`),
		"plain.test/":     html(`<iframe id="plain" src="http://plain.test/x"></iframe>`),
		"plain.test/deep": html(`<p>in the clear</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	for _, id := range []string{"plain", "hop"} {
		if text := s.framedText(id); !strings.Contains(text, "it is not sent over https") {
			t.Errorf("the frame %s shows %q, want light's notice", id, text)
		}
	}
	if f.came("plain.test/x") || f.came("plain.test/y") {
		t.Error("a frame in the clear was asked for by an https page")
	}
	if got := s.reason("http://plain.test/x"); got != "mixed content" {
		t.Errorf("the frame's request is blocked for %q", got)
	}
	if got := s.reason("http://plain.test/y"); got != "mixed content" {
		t.Errorf("the request a frame was redirected to is blocked for %q", got)
	}
	if want := "Mixed Content: The page at 'https://site.test/' was loaded over HTTPS, but requested an insecure " +
		"frame 'http://plain.test/x'. This request has been blocked; the content must be served over HTTPS."; !s.
		consoleSays(console.Error, want) {
		t.Errorf("the console does not say %q: %v", want, s.session().said.Lines())
	}
	s.navigate(s.frameAt("mid"), wanted{from: s.frameAt("mid").in, ref: "http://plain.test/deep"})
	s.quiet()
	if f.came("plain.test/deep") {
		t.Error("a frame was sent to an address in the clear")
	}
	s.visit("http://plain.test/")
	if text := s.framedText("plain"); text != "in the clear" {
		t.Errorf("a page in the clear shows %q in a frame in the clear", text)
	}
}

func TestATabHoldsSoManyFramesAndSoDeep(t *testing.T) {
	many := ""
	for i := range frames.MaxFrames + 6 {
		many += `<iframe id="f` + strconv.Itoa(i) + `" src="/leaf?` + strconv.Itoa(i) + `" width="20" height="10"></iframe>`
	}
	f, transport := framingOf(t, map[string]answer{
		"site.test/many": html(many),
		"site.test/leaf": html(`<p>leaf</p>`),
		"site.test/loop": html(`<p>loop</p><iframe id="again" src="/loop#part"></iframe><iframe id="other" src="/loop?x">
			</iframe>`),
	})
	for i := range frames.MaxDepth + 3 {
		f.pages["site.test/deep"+strconv.Itoa(i)] = html(`<p>` + strconv.Itoa(i) + `</p><iframe id="d` +
			strconv.Itoa(i+1) + `" src="/deep` + strconv.Itoa(i+1) + `"></iframe>`)
	}
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/many")
	s.quiet()
	shown, empty := 0, 0
	for _, fr := range s.framesOfPage().all {
		if fr.page != nil {
			shown++
		} else {
			empty++
		}
	}
	if shown != frames.MaxFrames || empty != 6 {
		t.Errorf("%d frames show a document and %d are empty, want %d and 6", shown, empty, frames.MaxFrames)
	}
	if !s.consoleSays(console.Warn, "A frame was left empty: "+frames.ErrTooMany.Error()) {
		t.Errorf("the console does not say a frame was one too many: %v", s.session().said.Lines())
	}

	s.visit("https://site.test/deep0")
	s.until("the deepest frame was reached", func() bool {
		return s.consoleSays(console.Warn, frames.ErrTooDeep.Error())
	})
	s.quiet()
	if !f.came("site.test/deep"+strconv.Itoa(frames.MaxDepth)) ||
		f.came("site.test/deep"+strconv.Itoa(frames.MaxDepth+1)) {
		t.Errorf("the frames did not stop %d deep", frames.MaxDepth)
	}

	s.visit("https://site.test/loop")
	s.until("the page's own address was refused its frame", func() bool {
		return s.consoleSays(console.Warn, frames.ErrRecursive.Error())
	})
	s.quiet()
	if s.frameAt("again").page != nil {
		t.Error("a page was shown in a frame of itself")
	}
	if n := len(s.framesOfPage().all); n > 8 {
		t.Errorf("a page that frames itself under another query holds %d frames", n)
	}
}

func TestFramesThatGoLetGoOfTheirDocumentsAndTheirLoads(t *testing.T) {
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<iframe id="one" src="/inner"></iframe><iframe id="held" src="/slow"></iframe>
			<iframe id="turning" src="/turn0"></iframe>`),
		"site.test/inner": html(`<p>inner</p><iframe id="deep" src="/leaf"></iframe>`),
		"site.test/leaf":  html(`<p>leaf</p>`),
		"site.test/next":  html(`<p>next</p>`),
	})
	for i := range 12 {
		f.pages["site.test/turn"+strconv.Itoa(i)] = html(`<p>turn ` + strconv.Itoa(i) + `</p>`)
	}
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	doc := s.visit("https://site.test/")
	s.framedText("one")
	s.framedText("deep")
	top := s.session().pages["https://site.test/"]
	set := s.framesOfPage()
	one, held, turning := s.frameAt("one"), s.frameAt("held"), s.frameAt("turning")
	if held.load == nil {
		t.Fatal("the slow frame is not on its way")
	}
	var gone []weak.Pointer[ui.Document]
	gone = append(gone, weak.Make(one.page.Doc), weak.Make(s.frameAt("deep").page.Doc))
	for i := 1; i < 12; i++ {
		s.framedTextOf("turning", "https://site.test/turn"+strconv.Itoa(i-1))
		gone = append(gone, weak.Make(turning.page.Doc))
		turning.node.SetAttr("src", "/turn"+strconv.Itoa(i))
	}
	s.framedTextOf("turning", "https://site.test/turn11")

	one.node.Parent().Remove(one.node)
	held.node.Parent().Remove(held.node)
	s.frame()
	s.frame()
	if one.page != nil || one.load != nil || held.load != nil || doc.Frame(one.node) != nil || one.live {
		t.Fatal("a frame taken out of its document still shows it, or still loads")
	}
	if n := len(set.all); n != 3 {
		t.Fatalf("the page holds %d frames after two left and one went with its document, want the 2 that left and 1", n)
	}
	collected := func() bool {
		for range 20 {
			runtime.GC()
			left := 0
			for _, p := range gone {
				if p.Value() != nil {
					left++
				}
			}
			if left == 0 {
				return true
			}
			s.frame()
			time.Sleep(5 * time.Millisecond)
		}
		return false
	}
	if !collected() {
		t.Error("the Documents of frames that went are still held")
	}

	last := weak.Make(turning.page.Doc)
	s.visit("https://site.test/next")
	if set.all[0].load != nil || s.session().frames[top] == nil {
		t.Fatal("the frames of a page the tab left for another were dropped, or still load")
	}
	s.session().pages["https://site.test/"] = nil
	delete(s.session().pages, "https://site.test/")
	s.show()
	if s.session().frames[top] != nil || len(set.all) != 0 {
		t.Fatalf("the tab holds %d frames of a page it forgot", len(set.all))
	}
	gone = []weak.Pointer[ui.Document]{last}
	if !collected() {
		t.Error("the Document of a frame of a page the tab forgot is still held")
	}
}

func TestWhatIsInAFrameIsReachedAsWhatIsInThePage(t *testing.T) {
	f, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<body style="margin:0"><iframe id="one" src="https://other.test/dir/links" width="400">
			</iframe>`),
		"other.test/dir/links": html(`<body style="margin:0"><a id="link" href="next">a link</a>`),
		"other.test/dir/next":  html(`<title>Next</title><p>next</p>`),
	})
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.system = "darwin"
	s.visit("https://site.test/")
	s.framedText("one")
	one := s.frameAt("one")
	at := s.inFrame(one, "link")

	s.in.SetMouse(at)
	s.frame()
	s.frame()
	if got := s.status.Text(); got != "https://other.test/dir/next" && got != "other.test/dir/next" {
		t.Errorf("over a link in a frame the bubble says %q", got)
	}
	s.rightClickAt(at)
	if got := labels(s.entries()); !strings.Contains(got, "Open link in new tab") || !strings.Contains(got,
		"Inspect element") {
		t.Fatalf("the menu of a link in a frame is %s", got)
	}
	s.choose("Inspect element")
	if got := s.inspect.Selected(); got != one.page.Doc.ByID("link") {
		t.Errorf("Inspect element in a frame selected %v, want the link of the frame's document", got)
	}
	s.toggleTools()
	s.frame()
	at = s.inFrame(one, "link")
	s.rightClickAt(at)
	s.choose("Open link in new tab")
	if got := s.opened(); !strings.Contains(got, "https://other.test/dir/next") {
		t.Fatalf("the tabs are %s after opening a frame's link in a tab", got)
	}
	s.click("tab-0")
	s.framedText("one")
	tabs := len(s.browser.Tabs)
	s.middleClickAt(s.inFrame(s.frameAt("one"), "link"))
	if len(s.browser.Tabs) != tabs+1 {
		t.Fatalf("a middle click on a frame's link opened %d tabs", len(s.browser.Tabs)-tabs)
	}
	s.clickHolding(s.inFrame(s.frameAt("one"), "link"), input.KeyLeftSuper)
	if len(s.browser.Tabs) != tabs+2 || s.frameAt("one").page.Address != "https://other.test/dir/links" {
		t.Fatalf("a command click on a frame's link opened %d tabs, the frame at %s", len(s.browser.Tabs)-tabs,
			s.frameAt("one").page.Address)
	}
	if got := f.header("other.test/dir/next", "Sec-Fetch-Dest"); got != "document" {
		t.Errorf("a frame's link opened in a tab is asked for as a %q", got)
	}
}
