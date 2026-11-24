//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // drives the Screen through its unexported parts

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/identity"
)

func TestWhatLightTellsASiteForWhatItsScriptsAsk(t *testing.T) {
	site := telling(t)
	site.script = `fetch("/data"); fetch("` + site.named() + `/shared", { headers: { Accept: "application/json",
		"Accept-Language": "de", "User-Agent": "a script's" } });
		const asked = new XMLHttpRequest(); asked.open("GET", "/asked"); asked.send();
		const added = document.createElement("script"); added.src = "/added.js"; document.head.append(added);
		import("/module.mjs").catch(() => {});
		document.getElementById("words").onclick = () => { location.href = "/clicked"; };
		window.leave = () => { location.href = "/left"; };`
	s := show(t)
	page := site.URL + "/"
	s.visit(page)
	s.heard(site, "a script element", "GET /app.js", part("*/*", "script no-cors", page))
	s.heard(site, "a script element a script added", "GET /added.js", part("*/*", "script no-cors", page))
	s.heard(site, "a module", "GET /module.mjs", part("*/*", "script cors", page))
	s.heard(site, "a fetch", "GET /data", part("*/*", "empty cors", page))
	s.heard(site, "an XMLHttpRequest", "GET /asked", part("*/*", "empty cors", page))
	s.heard(site, "a fetch of another origin", "GET /shared", strings.Join([]string{"Accept-Encoding: gzip, deflate",
		"Accept-Language: de", "Accept: application/json", "Origin: " + site.URL, "Referer: " + page,
		"Sec-Fetch-Dest: empty", "Sec-Fetch-Mode: cors", "Sec-Fetch-Site: cross-site",
		"User-Agent: " + identity.Default("")}, "\n"))

	s.clickAt(s.inPage("words"))
	s.heard(site, "a script's move as the player clicks", "GET /clicked", navigation("same-origin", page, true))

	s.visit(page)
	if _, err := s.evaluate(`setTimeout(leave, 0)`); err != nil {
		t.Fatal(err)
	}
	s.heard(site, "a script's move of its own", "GET /left", navigation("same-origin", page, false))
}

func TestScriptsReadWhatTheRequestsSay(t *testing.T) {
	site := telling(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Version: "0.4.2", Languages: []string{"fr-FR", "en-US"}})
	s.visit(site.URL + "/")
	read := func(expression string) string {
		t.Helper()
		value, err := s.evaluate(expression)
		if err != nil {
			t.Fatalf("%s: %v", expression, err)
		}
		if text, quoted := strconv.Unquote(value); quoted == nil {
			return text
		}
		return value
	}
	said := site.said("GET /")
	agent := identity.Default("0.4.2")
	if got := read(`navigator.userAgent`); got != agent || !strings.Contains(said, "User-Agent: "+got+"\n") &&
		!strings.HasSuffix(said, "User-Agent: "+got) {
		t.Errorf("scripts read the User-Agent %q, and the site was told:\n%s", got, said)
	}
	if got := read(`navigator.appVersion`); got != strings.TrimPrefix(agent, "Mozilla/") ||
		!strings.HasPrefix(got, "5.0 (") {
		t.Errorf("appVersion is %q", got)
	}
	if got := read(`navigator.languages.join()`); got != "fr-FR,fr,en-US,en" ||
		!strings.Contains(said, "Accept-Language: fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7\n") {
		t.Errorf("scripts read the languages %q, and the site was told:\n%s", got, said)
	}
	for expression, want := range map[string]string{
		`navigator.language`:            "fr-FR",
		`navigator.platform`:            identity.Platform(runtime.GOOS, runtime.GOARCH),
		`navigator.hardwareConcurrency`: strconv.Itoa(runtime.NumCPU()),
		`navigator.webdriver`:           "false",
		`navigator.cookieEnabled`:       "true",
		`[screen.width, screen.height, outerWidth, outerHeight, devicePixelRatio].join()`: "1024,680,1024,680,1",
		`innerWidth <= outerWidth && innerHeight < outerHeight`:                           "true",
	} {
		if got := read(expression); got != want {
			t.Errorf("%s is %q, want %q", expression, got, want)
		}
	}
	s.settings()
	s.click("set-cookies")
	s.set("set-user-agent", "Probe/2.0")
	s.visit(site.URL + "/")
	if got := read(`[navigator.userAgent, navigator.appVersion, navigator.cookieEnabled].join()`); got !=
		"Probe/2.0,Probe/2.0,false" {
		t.Errorf("once set, scripts read %q", got)
	}
}

func TestScriptsAreToldWhenLightIsDriven(t *testing.T) {
	site := telling(t)
	s := showWith(t, Options{Downloads: t.TempDir(), Driven: true})
	s.visit(site.URL + "/")
	if got, err := s.evaluate(`navigator.webdriver`); err != nil || got != "true" {
		t.Errorf("driven, navigator.webdriver is %q (%v)", got, err)
	}
}
