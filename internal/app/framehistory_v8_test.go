//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/console"
)

// travelled is a page with frames whose documents say where they are,
// each with the script that notes what its window hears of the tab's
// history.
func travelled(t *testing.T) *shown {
	t.Helper()
	leaf := func(name string) answer {
		return html(`<p id="at">` + name + `</p><a id="next" href="/` + name + `x">next</a><script>window.pops = [];
			addEventListener("popstate", (e) => pops.push(location.hash + ":" + JSON.stringify(e.state)));</script>`)
	}
	pages := map[string]answer{
		"site.test/": html(`<body style="margin:0"><p id="at">top</p>
			<iframe id="a" name="first" src="/a1" width="300" height="60"></iframe>
			<iframe id="b" src="https://other.test/b1" width="300" height="60"></iframe>
			<iframe id="shut" sandbox="allow-scripts" src="/s1" width="300" height="40"></iframe>
			<script>window.pops = []; addEventListener("popstate", (e) => pops.push(location.hash));</script>`),
		"site.test/next":  html(`<p id="at">next page</p>`),
		"site.test/outer": html(`<p id="at">outer</p><iframe id="n" src="/n1"></iframe><script>window.pops = [];</script>`),
	}
	for _, name := range []string{"a1", "a2", "a3", "n1", "n2", "s1", "s2", "a1x"} {
		pages["site.test/"+name] = leaf(name)
	}
	for _, name := range []string{"b1", "b2", "b3"} {
		pages["other.test/"+name] = leaf(name)
	}
	_, transport := framingOf(t, pages)
	s := showWith(t, Options{Transport: transport, Downloads: t.TempDir()})
	s.visit("https://site.test/")
	s.running("", "a", "b", "shut")
	return s
}

// standing is where the tab stands: what each frame of its page shows,
// "-" for one that is no more, and how many steps its history holds.
func (s *shown) standing() string {
	var out []string
	page := s.session().pages[s.browser.Tab().Current()]
	if page == nil || page.Doc == nil {
		return "elsewhere len=" + strconv.Itoa(s.browser.Tab().Len())
	}
	out = append(out, "top="+textOf(page.Doc.ByID("at")))
	var walk func(parent *frame, prefix string)
	walk = func(parent *frame, prefix string) {
		for _, f := range s.session().frames[page].frameList() {
			if f.parent != parent || !f.live || f.gone {
				continue
			}
			at := "?"
			if f.page != nil && f.load == nil && f.page.Doc != nil {
				at = textOf(f.page.Doc.ByID("at"))
			}
			out = append(out, prefix+f.node.ID()+"="+at)
			walk(f, prefix+f.node.ID()+".")
		}
	}
	walk(nil, "")
	return strings.Join(out, " ") + " len=" + strconv.Itoa(s.browser.Tab().Len())
}

// goes does the steps of a scenario in turn. "a>/a2" has the scripts
// of frame a go there, "a~>/a2" in place of their document, "a#x" has
// them push an address of their own, "a~#x" take one in place, "a<1"
// go that many steps through the history; "" names the page. "back",
// "forward" and "reload" are light's buttons, "click a" a click on the
// link of frame a's document, "rm a" takes the frame away, "src a /a2"
// changes what its element asks for, "=state" waits for the tab to
// stand there.
func (s *shown) goes(steps ...string) {
	s.t.Helper()
	for _, step := range steps {
		word := strings.Fields(step)
		switch {
		case strings.HasPrefix(step, "="):
			want, got := step[1:], ""
			for range 600 {
				s.frame()
				if got = s.standing(); got == want {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if got != want {
				s.t.Fatalf("after %v the tab stands at\n%q, want\n%q; the console says %s", steps, got, want, s.said())
			}
		case step == "back" || step == "forward" || step == "reload":
			s.click(step)
		case word[0] == "click":
			s.clickAt(s.inFrame(s.frameNamed(word[1]), "next"))
		case word[0] == "rm":
			s.does("", `document.getElementById("`+word[1]+`").remove();`)
		case word[0] == "src":
			s.does("", `document.getElementById("`+word[1]+`").src = "`+word[2]+`";`)
		case strings.Contains(step, "~>"):
			from, to, _ := strings.Cut(step, "~>")
			s.does(from, `location.replace("`+to+`");`)
		case strings.Contains(step, ">"):
			from, to, _ := strings.Cut(step, ">")
			s.does(from, `location.href = "`+to+`";`)
		case strings.Contains(step, "~#"):
			from, to, _ := strings.Cut(step, "~#")
			s.does(from, `history.replaceState({at: "`+to+`"}, "", "#`+to+`");`)
		case strings.Contains(step, "#"):
			from, to, _ := strings.Cut(step, "#")
			s.does(from, `history.pushState({at: "`+to+`"}, "", "#`+to+`");`)
		case strings.Contains(step, "<"):
			from, by, _ := strings.Cut(step, "<")
			s.does(from, `history.go(-`+by+`);`)
		default:
			s.t.Fatalf("no step %q", step)
		}
		for range 3 {
			s.frame()
		}
	}
}

func textOf(n interface{ Text() string }) string {
	return strings.Join(strings.Fields(n.Text()), " ")
}

const started = "top=top a=a1 b=b1 shut=s1 len=2"

func TestTheTabsHistoryGoesThroughItsFrames(t *testing.T) {
	for _, c := range []struct {
		name  string
		steps []string
	}{
		{"the first load of a frame is no step", []string{"=" + started}},
		{"a script sending its frame elsewhere is one", []string{"a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3",
			"back", "=top=top a=a1 b=b1 shut=s1 len=3", "forward", "=top=top a=a2 b=b1 shut=s1 len=3"}},
		{"a click on a link in a frame is one", []string{"click a", "=top=top a=a1x b=b1 shut=s1 len=3", "back",
			"=" + strings.Replace(started, "len=2", "len=3", 1)}},
		{"two frames, back in the order they went", []string{"a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3", "b>/b2",
			"=top=top a=a2 b=b2 shut=s1 len=4", "a>/a3", "=top=top a=a3 b=b2 shut=s1 len=5", "back",
			"=top=top a=a2 b=b2 shut=s1 len=5", "back", "=top=top a=a2 b=b1 shut=s1 len=5", "back",
			"=top=top a=a1 b=b1 shut=s1 len=5", "forward", "forward", "forward", "=top=top a=a3 b=b2 shut=s1 len=5"}},
		{"history.go from another frame, several steps", []string{"a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3", "b>/b2",
			"=top=top a=a2 b=b2 shut=s1 len=4", "b<2", "=top=top a=a1 b=b1 shut=s1 len=4"}},
		{"history.back from the page", []string{"b>/b2", "=top=top a=a1 b=b2 shut=s1 len=3", "<1",
			"=top=top a=a1 b=b1 shut=s1 len=3"}},
		{"location.replace is no step", []string{"a~>/a2", "=top=top a=a2 b=b1 shut=s1 len=2", "back",
			"=elsewhere len=2"}},
		{"a step after going back drops what was ahead", []string{"a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3", "a>/a3",
			"=top=top a=a3 b=b1 shut=s1 len=4", "back", "=top=top a=a2 b=b1 shut=s1 len=4", "b>/b2",
			"=top=top a=a2 b=b2 shut=s1 len=4", "forward", "=top=top a=a2 b=b2 shut=s1 len=4"}},
		{"a frame that is no more is passed over", []string{"a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3", "b>/b2",
			"=top=top a=a2 b=b2 shut=s1 len=4", "rm b", "=top=top a=a2 shut=s1 len=4", "back",
			"=top=top a=a1 shut=s1 len=4"}},
		{"a src a script changes is a step", []string{"src a /a2", "=top=top a=a2 b=b1 shut=s1 len=3", "back",
			"=top=top a=a1 b=b1 shut=s1 len=3"}},
		{"a frame in a frame", []string{"a>/outer", "=top=top a=outer a.n=n1 b=b1 shut=s1 len=3", "n>/n2",
			"=top=top a=outer a.n=n2 b=b1 shut=s1 len=4", "back", "=top=top a=outer a.n=n1 b=b1 shut=s1 len=4", "back",
			"=top=top a=a1 b=b1 shut=s1 len=4", "forward", "forward", "=top=top a=outer a.n=n2 b=b1 shut=s1 len=4"}},
		{"a reload keeps the frames where they are", []string{"a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3", "b>/b2",
			"=top=top a=a2 b=b2 shut=s1 len=4", "reload", "=top=top a=a2 b=b2 shut=s1 len=4", "back",
			"=top=top a=a2 b=b1 shut=s1 len=4"}},
		{"a reload keeps a frame in a frame", []string{"a>/outer", "=top=top a=outer a.n=n1 b=b1 shut=s1 len=3", "n>/n2",
			"=top=top a=outer a.n=n2 b=b1 shut=s1 len=4", "reload", "=top=top a=outer a.n=n2 b=b1 shut=s1 len=4"}},
		{"another page and back brings the frames back", []string{"a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3", ">/next",
			"=top=next page len=4", "back", "=top=top a=a2 b=b1 shut=s1 len=4", "back",
			"=top=top a=a1 b=b1 shut=s1 len=4"}},
		{"a sandboxed frame moves itself", []string{"shut>/s2", "=top=top a=a1 b=b1 shut=s2 len=3", "shut<1",
			"=top=top a=a1 b=b1 shut=s1 len=3"}},
		{"a sandboxed frame moves no other frame", []string{"shut>/s2", "=top=top a=a1 b=b1 shut=s2 len=3", "a>/a2",
			"=top=top a=a2 b=b1 shut=s2 len=4", "shut<1", "=top=top a=a2 b=b1 shut=s2 len=4"}},
		{"a sandboxed frame does not leave the page", []string{"shut<1", "=" + started}},
		{"a frame that is not sandboxed may", []string{"b<1", "=elsewhere len=2"}},
	} {
		s := travelled(t)
		s.goes(c.steps...)
		if t.Failed() {
			t.Fatalf("in %q", c.name)
		}
		_ = s.Close()
	}
}

func TestAFramesOwnAddressesAreItsDocuments(t *testing.T) {
	s := travelled(t)
	s.goes("a#x", "a#y", "b#z", "=top=top a=a1 b=b1 shut=s1 len=5")
	if got := s.within("a", `history.length + " " + location.hash + " " + JSON.stringify(history.state)`); got !=
		`5 #y {"at":"y"}` {
		t.Errorf("the frame's history reads %s", got)
	}
	s.goes("back", "back", "=top=top a=a1 b=b1 shut=s1 len=5")
	s.becomes("a", `pops.join()`, `#x:{"at":"x"}`)
	if got := s.within("b", `pops.join() + location.hash`) + "|" + s.within("", `pops.join()`); got != ":null|" {
		t.Errorf("the other frame heard %s", got)
	}
	s.goes("back", "=top=top a=a1 b=b1 shut=s1 len=5")
	s.becomes("a", `pops.join() + " " + location.hash`, `#x:{"at":"x"},:null `)
	s.goes("forward", "forward", "forward", "a~#w", "=top=top a=a1 b=b1 shut=s1 len=5")
	s.becomes("a", `location.hash + " " + pops.length`, "#w 4")
	s.goes("back", "back", "forward", "forward")
	s.becomes("a", `pops.at(-1) + " " + location.hash`, `#w:{"at":"w"} #w`)
	s.goes("a>/a2", "=top=top a=a2 b=b1 shut=s1 len=6", "back", "=top=top a=a1 b=b1 shut=s1 len=6")
	s.becomes("a", `location.hash + pops.length`, "#w0")
	if s.consoleSays(console.Error, "") {
		t.Errorf("the console says %s", s.said())
	}
}

func TestTheHistoryPageListsNoFrame(t *testing.T) {
	s := travelled(t)
	s.goes("a>/a2", "=top=top a=a2 b=b1 shut=s1 len=3", "b>/b2", "=top=top a=a2 b=b2 shut=s1 len=4")
	s.Open(historyPage)
	s.frame()
	visits := strings.Join(s.visits(), "\n")
	if strings.Count(visits, "\n") != 0 || !strings.Contains(visits, "site.test/") || strings.Contains(visits, "a2") ||
		strings.Contains(visits, "b2") {
		t.Errorf("the history lists\n%s", visits)
	}
}
