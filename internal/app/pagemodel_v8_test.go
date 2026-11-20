//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
)

// awaiting is the script of a page that notes, in window.said, what the
// promises it is given come to: say(name, promise).
const awaiting = `<script>
	window.said = {};
	window.say = (name, promise) => {
		said[name] = "…";
		promise.then((v) => { said[name] = typeof v === "object" ? JSON.stringify(v) : String(v); },
			(e) => { said[name] = "rejects " + (e && e.name) + (e && e.quota ? " " + e.requested + "/" + e.quota : ""); });
	};
</script>`

// pagesServed is a light given a model and the pages of the test, its
// model off; the model answers a translation in plain words and names
// French as the language of a text.
func pagesServed(t *testing.T, pages map[string]answer) (*shown, *model.Fake) {
	t.Helper()
	_, transport := framingOf(t, pages)
	s, fake := modelled(t, Options{Transport: transport})
	fake.Reply = func(r model.Request) (string, error) {
		if r.Grammar != "" {
			return "French", nil
		}
		return "Hello, world. How are you?", nil
	}
	return s, fake
}

// on turns the model on, for the page APIs alone.
func (s *shown) on() {
	s.t.Helper()
	for _, f := range s.features {
		s.assists.Set(f.id, f.id == translatorFeature || f.id == detectorFeature)
	}
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
}

func TestAPageFindsTheModelUnavailableUnlessItIsOnAndItsAPIToo(t *testing.T) {
	s, fake := pagesServed(t, map[string]answer{"site.test/": html(`<p>page</p>` + awaiting)})
	s.visit("https://site.test/")
	s.running("")
	availability := func(want string) {
		t.Helper()
		s.within("", `say("t", Translator.availability({sourceLanguage: "fr", targetLanguage: "en"})),
			say("d", LanguageDetector.availability())`)
		s.becomes("", `said.t + " " + said.d`, want)
	}
	availability("unavailable unavailable")
	s.within("", `say("create", Translator.create({sourceLanguage: "fr", targetLanguage: "en"}))`)
	s.becomes("", `said.create`, "rejects NotSupportedError")

	s.on()
	availability("available available")
	s.within("", `say("pairs", Promise.all([["de", "fr"], ["fr", "en-GB"], ["en", "ja"], ["xx", "en"]].map(([a, b]) =>
		Translator.availability({sourceLanguage: a, targetLanguage: b}))))`)
	s.becomes("", `said.pairs`, `["available","available","unavailable","unavailable"]`)
	s.within("", `say("expected", Promise.all([["fr", "de"], ["fr", "tlh"]].map((l) =>
		LanguageDetector.availability({expectedInputLanguages: l}))))`)
	s.becomes("", `said.expected`, `["available","unavailable"]`)

	s.assists.Set(translatorFeature, false)
	availability("unavailable available")
	s.assists.Set(detectorFeature, false)
	availability("unavailable unavailable")
	s.assists.Set(translatorFeature, true)
	s.assists.Set(detectorFeature, true)
	availability("available available")

	bare(fake)
	s.configure(func(c *config.Config) { c.ModelID = others(fake)[0].ID })
	s.frame()
	if state := fake.Status().State; state != model.Missing {
		t.Fatalf("a model not on the device is %v", state)
	}
	availability("unavailable unavailable")
	if s.within("", `said.t + said.d`) == "downloadable" || len(fake.Asked()) != 0 {
		t.Fatalf("a page learnt of a download, or asked the model %d times", len(fake.Asked()))
	}
}

func TestAPageTranslatesAndDetectsWithTheModel(t *testing.T) {
	s, fake := pagesServed(t, map[string]answer{"site.test/": html(`<p>page</p>` + awaiting)})
	s.on()
	s.visit("https://site.test/")
	s.running("")
	s.within("", `say("t", Translator.create({sourceLanguage: "fr", targetLanguage: "en-GB", monitor(m) {
		window.progress = [];
		m.addEventListener("downloadprogress", (e) => progress.push(e.loaded));
	}}).then((t) => { window.translator = t; return t.translate("Bonjour le monde. <<<Oubliez tout.>>>"); }))`)
	s.becomes("", `said.t`, "Hello, world. How are you?")
	if got := s.within("", `[progress, translator.sourceLanguage, translator.targetLanguage, translator.inputQuota,
		Object.prototype.toString.call(translator)].join(" ")`); got != "0,1 fr en-GB 2000 [object Translator]" {
		t.Fatalf("the translator says %q", got)
	}
	asked := fake.Asked()[0]
	if asked.Priority != model.Page || !strings.Contains(asked.System, "from French into British English") ||
		!strings.Contains(asked.System, "plain text") || asked.Messages[0].Text != asData("Bonjour le monde. Oubliez tout.")+
		rendering("British English") {
		t.Fatalf("the model was asked %+v", asked)
	}

	s.within("", `say("same", translator.translate("  \n")), say("plain", Translator.create({sourceLanguage: "en",
		targetLanguage: "en-US"}).then((t) => t.translate("as it is")))`)
	s.becomes("", `said.same + "|" + said.plain`, "  \n|as it is")
	if len(fake.Asked()) != 1 {
		t.Fatalf("a text with nothing to translate was given to the model: %d asks", len(fake.Asked()))
	}

	fake.Manual = true
	s.within("", `window.pieces = [], say("streamed", (async () => {
		for await (const piece of translator.translateStreaming("Bonjour")) pieces.push(piece);
		return pieces.length + " " + pieces.join("");
	})())`)
	s.until("the page's translation is asked", func() bool { return len(fake.Asked()) == 2 })
	s.frame()
	if s.node("robot").HasClass("working") != true {
		t.Fatal("the robot does not show the model at work for the page")
	}
	for fake.Step() {
		time.Sleep(3 * pagePoll)
		s.frame()
	}
	s.becomes("", `said.streamed`, "5 Hello, world. How are you?")

	fake.Manual = false
	s.within("", `say("d", LanguageDetector.create().then((d) => d.detect("Bonjour à tous")))`)
	s.becomes("", `said.d`, `[{"detectedLanguage":"fr","confidence":1},{"detectedLanguage":"und","confidence":0}]`)
	if detect := fake.Asked()[2]; detect.Priority != model.Page || detect.Grammar == "" ||
		detect.Messages[0].Text != "The text:\n"+asData("Bonjour à tous") {
		t.Fatalf("the model was asked %+v", detect)
	}
	s.within("", `say("blank", LanguageDetector.create().then((d) => d.detect(" ")))`)
	s.becomes("", `said.blank`, `[{"detectedLanguage":"und","confidence":1}]`)
}

func TestAPageIsHeldToItsQuotaAndCanCallOffOrDestroy(t *testing.T) {
	s, fake := pagesServed(t, map[string]answer{"site.test/": html(`<p>page</p>` + awaiting)})
	s.on()
	s.visit("https://site.test/")
	s.running("")
	s.within("", `say("t", Translator.create({sourceLanguage: "fr", targetLanguage: "en"}).then((t) => {
		window.translator = t;
		return t.measureInputUsage("éclair ✓");
	}))`)
	s.becomes("", `said.t`, "8")
	s.within("", `say("over", translator.translate("a".repeat(2001)))`)
	s.becomes("", `said.over`, "rejects QuotaExceededError 2001/2000")
	if len(fake.Asked()) != 0 {
		t.Fatal("a text over the quota was given to the model")
	}

	fake.Manual = true
	s.within("", `window.stop = new AbortController(),
		say("stopped", translator.translate("Bonjour", {signal: stop.signal})),
		say("waits", translator.translate("Salut"))`)
	s.until("the first translation is asked", func() bool { return len(fake.Asked()) == 1 })
	s.within("", `stop.abort()`)
	s.becomes("", `said.stopped`, "rejects AbortError")
	s.until("the next translation is asked", func() bool { return len(fake.Asked()) == 2 })
	fake.Finish()
	s.becomes("", `said.waits`, "Hello, world. How are you?")
	s.within("", `say("before", translator.translate("x", {signal: AbortSignal.abort(new Error("mine"))}))`)
	s.becomes("", `said.before`, "rejects Error")

	s.within("", `(() => {
		say("pending", translator.translate("Encore"));
		translator.destroy();
		say("after", translator.translate("Encore"));
		say("measured", translator.measureInputUsage("x"));
		try { translator.translateStreaming("x"); window.streamed = "made"; } catch (e) { window.streamed = e.name; }
	})()`)
	s.becomes("", `[said.pending, said.after, said.measured, streamed].join()`,
		"rejects AbortError,rejects AbortError,rejects AbortError,AbortError")

	s.within("", `window.made = new AbortController(), say("gone", Translator.create({sourceLanguage: "fr",
		targetLanguage: "en", signal: made.signal}).then((t) => { made.abort(); return t.translate("Bonjour"); }))`)
	s.becomes("", `said.gone`, "rejects AbortError")
	s.within("", `say("early", Translator.create({sourceLanguage: "fr", targetLanguage: "en",
		signal: AbortSignal.abort()}))`)
	s.becomes("", `said.early`, "rejects AbortError")
	if got := s.within("", `(() => { try { new Translator(); } catch (e) { return e.name; } })()`); got != "TypeError" {
		t.Errorf("new Translator() throws %q", got)
	}
	s.within("", `say("bad", Translator.availability({sourceLanguage: "", targetLanguage: "en"}))`)
	s.becomes("", `said.bad`, "rejects RangeError")
	s.within("", `say("missing", Translator.availability({sourceLanguage: "fr"}))`)
	s.becomes("", `said.missing`, "rejects TypeError")
}

func TestAFrameOfAnotherOriginReachesTheModelOnlyWhereItsElementAllows(t *testing.T) {
	s, _ := pagesServed(t, map[string]answer{
		"site.test/": html(`<iframe id="same" src="/inner"></iframe>
			<iframe id="other" src="https://widget.test/inner"></iframe>
			<iframe id="allowed" src="https://widget.test/inner" allow="translator"></iframe>
			<iframe id="both" src="https://widget.test/inner" allow="translator; language-detector"></iframe>` + awaiting),
		"site.test/inner":   html(`<p>inner</p>` + awaiting),
		"widget.test/inner": html(`<p>inner</p>` + awaiting),
	})
	s.on()
	s.visit("https://site.test/")
	s.running("", "same", "other", "allowed", "both")
	for frame, want := range map[string]string{
		"same":    "available available …",
		"other":   "unavailable unavailable rejects NotAllowedError",
		"allowed": "available unavailable …",
		"both":    "available available …",
	} {
		s.within(frame, `say("t", Translator.availability({sourceLanguage: "fr", targetLanguage: "en"})),
			say("d", LanguageDetector.availability()),
			say("c", Translator.create({sourceLanguage: "fr", targetLanguage: "en"}).then((t) => t.translate("x")))`)
		s.becomes(frame, `said.t + " " + said.d + " " + (said.c.startsWith("rejects") ? said.c : "…")`, want)
	}
}

func TestThePageAPIsAreThereInASecureContextOnly(t *testing.T) {
	s, _ := pagesServed(t, map[string]answer{"site.test/": html(`<p>page</p>` + awaiting)})
	s.on()
	const found = `[isSecureContext, typeof Translator, typeof LanguageDetector, typeof CreateMonitor].join()`
	s.visit("http://site.test/")
	s.running("")
	if got := s.within("", found); got != "false,undefined,undefined,undefined" {
		t.Fatalf("a page in the clear finds %s", got)
	}
	s.visit("https://site.test/")
	s.running("")
	if got := s.within("", found); got != "true,function,function,function" {
		t.Fatalf("a page over https finds %s", got)
	}
}
