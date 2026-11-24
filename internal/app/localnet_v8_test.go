//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/localnet"
)

func TestTheScriptsOfAPageOfTheInternetReachNothingOfTheLocalNetwork(t *testing.T) {
	l := newLAN(t)
	site, other, router, here := l.serve(localnet.Public), l.serve(localnet.Public), l.serve(localnet.Private),
		l.serve(localnet.Loopback)
	named := "http://localhost:" + portOf(here)
	l.show(site+"/", fmt.Sprintf(`<title>Public</title><p id="out"></p><script src="%[2]s/s.js"></script>
		<script type="module">import one from "%[2]s/m.js";</script>
		<script>const other = %[1]q, router = %[2]q, here = %[3]q, named = %[4]q;
		const seen = [], note = (name) => (what) => seen.push(name + " " + what);
		const xhr = (method, url) => new Promise((settle) => {
			const x = new XMLHttpRequest(); x.open(method, url);
			x.onerror = () => settle("error " + x.status); x.onload = () => settle("load " + x.responseText);
			x.send(method === "POST" ? "b" : null);
		});
		Promise.all([
			fetch(router + "/api").then(() => "read", (e) => e.name).then(note("fetch local")),
			fetch(here + "/api", { mode: "no-cors" }).then((r) => r.type, (e) => e.name).then(note("no-cors here")),
			fetch(named + "/api").then(() => "read", (e) => e.name).then(note("named")),
			fetch(other + "/api").then((r) => r.text(), (e) => e.name).then(note("fetch public")),
			fetch(other + "/turn?to=" + router + "/turned").then(() => "read", (e) => e.name).then(note("redirected")),
			fetch(router + "/put", { method: "PUT", body: "b" }).then(() => "read", (e) => e.name).then(note("preflighted")),
			xhr("POST", router + "/xhr").then(note("xhr local")), xhr("GET", other + "/xhr").then(note("xhr public")),
		]).then(() => {
			seen.push("beacon " + navigator.sendBeacon(router + "/beacon", "b"), "ran " + window.ran);
			document.getElementById("out").textContent = seen.sort().join("|");
		});</script>`, other, router, here, named))
	s := showWith(t, Options{Downloads: t.TempDir(), Space: l.space})
	s.visit(site + "/")
	s.until("the script was done", func() bool { return s.pageText("out") != "" })
	equal(t, "the script saw", strings.Split(s.pageText("out"), "|"), []string{
		"beacon true", "fetch local TypeError", "fetch public ok", "named TypeError", "no-cors here TypeError",
		"preflighted TypeError", "ran undefined", "redirected TypeError", "xhr local error 0", "xhr public load ok",
	})
	s.blocked(map[string]string{
		router + "/s.js": toLocal, router + "/m.js": toLocal, router + "/api": toLocal, router + "/turned": toLocal,
		router + "/put": toLocal, router + "/xhr": toLocal, router + "/beacon": toLocal,
		here + "/api": toLoopback, named + "/api": toLoopback,
	})
	if got := l.got(router) + l.got(here); got != "" {
		t.Errorf("the scripts of a page of the Internet asked the local network for %s", got)
	}
	for _, line := range []string{
		"Access to fetch at '" + router + "/api' from origin '" + site + "' has been blocked by CORS policy: " + toLocal,
		"Access to fetch at '" + here + "/api' from origin '" + site + "' has been blocked by CORS policy: " + toLoopback,
		"Access to fetch at '" + router + "/turned' (redirected from '" + other + "/turn?to=" + router + "/turned') " +
			"from origin '" + site + "' has been blocked by CORS policy: " + toLocal,
		"Access to XMLHttpRequest at '" + router + "/xhr' from origin '" + site + "' has been blocked by CORS policy: " +
			toLocal,
	} {
		if !strings.Contains(s.said(), line) {
			t.Errorf("the console does not say %q; it says:\n%s", line, s.said())
		}
	}
}

func TestAScriptTakesItsPageNoFurtherThanItReachesUnlessTheUserClicked(t *testing.T) {
	l := newLAN(t)
	site, router := l.serve(localnet.Public), l.serve(localnet.Private)
	l.show(site+"/moves", `<title>Moves</title><script>setTimeout(() => { location.href = "`+router+`/reboot"; }, 0);
		</script>`)
	l.show(site+"/posts", `<title>Posts</title><form id="f" method="post" action="`+router+`/set">
		<input name="a" value="1"></form><script>setTimeout(() => document.getElementById("f").submit(), 0);</script>`)
	l.show(site+"/asked", `<title>Asked</title><style>body { margin: 0 } button { display: block; height: 24px }</style>
		<button id="go">go</button><script>
		document.getElementById("go").addEventListener("click", () => { location.href = "`+router+`/admin"; });</script>`)
	for _, path := range []string{"/reboot", "/set", "/admin"} {
		l.show(router+path, `<title>Reached</title>`)
	}
	s := showWith(t, Options{Downloads: t.TempDir(), Space: l.space})
	for _, c := range []struct{ from, to string }{{"/moves", "/reboot"}, {"/posts", "/set"}} {
		s.Open(site + c.from)
		s.until("the script's move ended", func() bool { return s.session().pages[router+c.to] != nil })
		var refused *localnet.BlockedError
		if page := s.session().pages[router+c.to]; !errors.As(page.Err, &refused) || refused.From != localnet.Public ||
			refused.To != localnet.Private {
			t.Errorf("the script of %s took its page to %+v", c.from, page)
		}
		if got := s.reason(router + c.to); got != toLocal {
			t.Errorf("the move from %s is blocked for %q", c.from, got)
		}
	}
	if got := l.asked[router]; len(got) != 0 {
		t.Errorf("the local network was asked for %s", got)
	}
	s.visit(site + "/asked")
	s.until("its scripts ran", func() bool { return s.session().scripts != nil })
	s.clickAt(s.inPage("go"))
	s.until("the move the user clicked for ended", func() bool { return s.session().pages[router+"/admin"] != nil })
	if page := s.session().pages[router+"/admin"]; page.Err != nil || page.Title != "Reached" {
		t.Errorf("the click took the page to %+v", page)
	}
}
