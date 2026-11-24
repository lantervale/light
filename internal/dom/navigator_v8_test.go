//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/identity"
)

func TestTheNavigatorSaysWhatLightIsTold(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.start(`<p>page</p>`, dom.Host{
		Navigator: dom.Navigator{Platform: "Win32", Languages: []string{"fr-FR", "fr", "en"}, Cores: 6, Driven: true},
		Window:    func() dom.Window { return dom.Window{Width: 1024, Height: 680, X: 40, Y: 60, Ratio: 2} },
	})
	for _, c := range []struct{ what, expression, want string }{
		{"userAgent", `navigator.userAgent`, "light/test"},
		{"the names every browser answers", `[navigator.appName, navigator.appCodeName, navigator.product,
			navigator.vendor].join()`, "Netscape,Mozilla,Gecko,"},
		{"platform", `navigator.platform`, "Win32"},
		{"language", `navigator.language`, "fr-FR"},
		{"languages", `navigator.languages.join() + Object.isFrozen(navigator.languages)`, "fr-FR,fr,entrue"},
		{"hardwareConcurrency", `navigator.hardwareConcurrency`, "6"},
		{"what it does not say", `["deviceMemory", "userAgentData"].filter((name) => name in navigator).join()`, ""},
		{"cookieEnabled, onLine", `[navigator.cookieEnabled, navigator.onLine].join()`, "true,true"},
		{"webdriver", `navigator.webdriver`, "true"},
		{"plugins", `[navigator.plugins.length, navigator.plugins.item(0), navigator.plugins.namedItem("PDF Viewer"),
			[...navigator.plugins].length, typeof navigator.plugins.refresh].join()`, "0,,,0,function"},
		{"mimeTypes", `[navigator.mimeTypes.length, navigator.mimeTypes.item(0),
			navigator.mimeTypes.namedItem("application/pdf")].join()`, "0,,"},
		{"what light has not", `[navigator.pdfViewerEnabled, navigator.javaEnabled(), navigator.maxTouchPoints,
			navigator.doNotTrack].join()`, "false,false,0,"},
		{"the screen is the window", `[screen.width, screen.height, screen.availWidth, screen.availHeight,
			screen.colorDepth, screen.pixelDepth, screen.orientation.type].join()`,
			"1024,680,1024,680,24,24,landscape-primary"},
		{"the window", `[outerWidth, outerHeight, innerWidth, innerHeight, screenX, screenY, screenLeft, screenTop,
			devicePixelRatio].join()`, "1024,680,800,600,40,60,40,60,2"},
	} {
		if got := s.eval(c.expression); got != c.want {
			t.Errorf("%s: %q, want %q", c.what, got, c.want)
		}
	}
}

func TestANavigatorToldNothingSaysWhatIsTrueOfThisMachine(t *testing.T) {
	s := run(t, `<p>page</p>`)
	for _, c := range []struct{ what, expression, want string }{
		{"platform", `navigator.platform`, identity.Platform(runtime.GOOS, runtime.GOARCH)},
		{"languages", `navigator.languages.join()`, "en-US,en"},
		{"hardwareConcurrency", `navigator.hardwareConcurrency`, strconv.Itoa(runtime.NumCPU())},
		{"webdriver", `navigator.webdriver`, "false"},
		{"a page in no window is its own view", `[screen.width, screen.height, outerWidth, outerHeight, screenX,
			screenY, devicePixelRatio].join()`, "800,600,800,600,0,0,1"},
	} {
		if got := s.eval(c.expression); got != c.want {
			t.Errorf("%s: %q, want %q", c.what, got, c.want)
		}
	}
}

func TestScriptsReadTheSystemsTimeZone(t *testing.T) {
	s := run(t, `<p>page</p>`)
	_, offset := time.Now().Zone()
	if got := s.eval(`new Date().getTimezoneOffset()`); got != strconv.Itoa(-offset/60) {
		t.Errorf("Date is %s minutes from UTC, and the system %d", got, -offset/60)
	}
	zone := s.eval(`Intl.DateTimeFormat().resolvedOptions().timeZone`)
	if zone == "" || zone == "undefined" {
		t.Fatalf("Intl names no time zone: %q", zone)
	}
	t.Logf("Intl says %s; Go says %s", zone, time.Local)
}
