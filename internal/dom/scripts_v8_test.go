//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"sync"
	"testing"

	"github.com/lantervale/light/internal/dom"
)

// held answers the scripts a page asks for only when told to.
type held struct {
	sync.Mutex
	waiting map[string]func(dom.Response)
}

func (h *held) fetch(q dom.Request, done func(dom.Response)) func() {
	h.Lock()
	defer h.Unlock()
	h.waiting[q.Address] = done
	return func() {}
}

// answer answers the script at address with source.
func (h *held) answer(t *testing.T, address, source string) {
	t.Helper()
	h.Lock()
	done := h.waiting[address]
	delete(h.waiting, address)
	h.Unlock()
	if done == nil {
		t.Fatalf("%s was not asked for: %v", address, h.waiting)
	}
	done(dom.Response{Status: 200, Address: address, Text: source})
}

func TestAScriptPutInThePageRunsAsItIsInserted(t *testing.T) {
	s := run(t, `<div id="box"></div><script>var seen = [];
		const one = document.createElement("script"); one.textContent = "seen.push('inline ' + document.currentScript.id)";
		one.id = "one";
		seen.push("before"); box.appendChild(one); seen.push("after " + document.currentScript.id);</script>`)
	if got := s.eval(`seen.join()`); got != "before,inline one,after " {
		t.Errorf("the page saw %q", got)
	}
	s.quiet()
}

func TestAScriptRunsOnceItHasTextOrASrc(t *testing.T) {
	s := run(t, `<div id="box"></div><script>var seen = [];
		const empty = document.createElement("script"); box.appendChild(empty); seen.push("empty");
		empty.appendChild(document.createElement("b")); seen.push("element");
		empty.appendChild(document.createTextNode("seen.push('text')"));
		empty.appendChild(document.createTextNode("seen.push('again')"));
		const data = document.createElement("script"); data.type = "text/plain"; data.text = "seen.push('data')";
		box.appendChild(data); data.type = ""; box.appendChild(data);
		const filled = document.createElement("script"); box.appendChild(filled); filled.text = "seen.push('filled')";
		</script>`)
	if got := s.eval(`seen.join()`); got != "empty,element,text,data,filled" {
		t.Errorf("the page saw %q", got)
	}
	s.quiet()
}

func TestTheTypesAndLanguagesOfScripts(t *testing.T) {
	s := run(t, `<div id="box"></div><script>var ran = [];
		const tried = (attribute, value) => { const e = document.createElement("script"); e.setAttribute(attribute, value);
			e.text = "ran.push(" + JSON.stringify(attribute + "=" + value) + ")"; box.appendChild(e); };
		for (const type of ["", " ", "TEXT/JavaScript", " application/x-ecmascript\n", "text/javascript1.5",
			"text/javascript1.6", "javascript", "text/javascript\0", "text/javascript; charset=utf-8"]) tried("type", type);
		for (const language of ["", "JavaScript", "javascript1.6", " javascript", "ecmascriptxyz"])
			tried("language", language);
		</script><script type="javascript">ran.push("parsed javascript")</script>
		<script language="livescript">ran.push("parsed livescript")</script>`)
	want := `type=,type=TEXT/JavaScript,type= application/x-ecmascript` + "\n" +
		`,type=text/javascript1.5,language=,language=JavaScript,parsed livescript`
	if got := s.eval(`ran.join()`); got != want {
		t.Errorf("the scripts that ran are %q\n                       want %q", got, want)
	}
}

func TestScriptsPutInThePageRunInTheirOrderUnlessAsync(t *testing.T) {
	h := &held{waiting: map[string]func(dom.Response){}}
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.start(`<script>var seen = [];
		const put = (name, async) => { const e = document.createElement("script"); e.src = "/" + name + ".js";
			if (async !== undefined) e.async = async; document.head.appendChild(e); return e; };
		var asyncs = [put("first", false).async, put("second", false).async, put("free").async];
		window.onload = () => seen.push("load");</script>`, dom.Host{Fetch: h.fetch})
	if got := s.eval(`asyncs.join()`); got != "false,false,true" {
		t.Errorf("the scripts are async %q", got)
	}
	h.answer(t, "https://site.test/second.js", `seen.push("second")`)
	h.answer(t, "https://site.test/free.js", `seen.push("free")`)
	s.frames("the async script", func() bool { return s.eval(`seen.join()`) == "free" })
	h.answer(t, "https://site.test/first.js", `seen.push("first")`)
	s.frames("the scripts in order, then the load", func() bool {
		return s.eval(`seen.join()`) == "free,first,second,load"
	})
}

func TestTheAsyncOfAScript(t *testing.T) {
	s := run(t, `<script></script><script async src="/lib.js"></script><script>
		const made = document.createElement("script"), set = document.createElement("script");
		set.async = false; const attributed = document.createElement("script"); attributed.setAttribute("async", "");
		attributed.removeAttribute("async");</script>`)
	got := s.eval(`[document.scripts[0].async, document.scripts[1].async, document.scripts[2].async, made.async,
		made.cloneNode().async, set.async, set.getAttribute("async"), attributed.async].join()`)
	if want := "true,true,false,true,true,false,,false"; got != want {
		t.Errorf("the scripts are async %q, want %q", got, want)
	}
}
