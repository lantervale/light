//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import "testing"

func TestASignalCallsOffWhatFollowsIt(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="p"></p>`)
	const thrown = `const thrown = (run) => { try { run(); return "none" } catch (e) { return e.name || String(e) } };`
	for _, c := range []struct{ name, script, want string }{
		{"a controller aborts its signal once, with a reason", `(() => { const c = new AbortController(), s = c.signal,
			heard = []; s.onabort = (e) => heard.push(e.type + e.isTrusted + (e.target === s) + s.aborted);
			const before = [s.aborted, s.reason, thrown(() => s.throwIfAborted()), c.signal === s];
			c.abort(); c.abort("again"); return before.concat(heard, s.reason.name, thrown(() => s.throwIfAborted()),
			thrown(() => new AbortSignal()), s instanceof EventTarget, String(s)).join() })()`,
			"false,,none,true,aborttruetruetrue,AbortError,AbortError,TypeError,true,[object AbortSignal]"},
		{"a signal is made aborted, or to time out", `[AbortSignal.abort().reason.name, AbortSignal.abort("why").reason,
			AbortSignal.abort().aborted, AbortSignal.timeout(5).aborted, thrown(() => AbortSignal.timeout(-1)),
			thrown(() => AbortSignal.timeout())].join()`, "AbortError,why,true,false,TypeError,TypeError"},
		{"a signal follows any of others, before their own listeners' followers", `(() => { const a =
			new AbortController(), b = new AbortController(), any = AbortSignal.any([a.signal, b.signal]), deep =
			AbortSignal.any([any]), heard = []; a.signal.addEventListener("abort", () => heard.push("a" + any.aborted +
			deep.aborted)); any.addEventListener("abort", () => heard.push("any")); deep.addEventListener("abort",
			() => heard.push("deep")); b.abort("b"); a.abort("a"); return heard.concat(any.reason, deep.reason,
			AbortSignal.any([]).aborted, AbortSignal.any([AbortSignal.abort("was")]).reason,
			thrown(() => AbortSignal.any([{}])), thrown(() => AbortSignal.any(null))).join() })()`,
			"any,deep,atruetrue,b,b,false,was,TypeError,TypeError"},
		{"a listener added with a signal leaves when it aborts", `(() => { const c = new AbortController(), p =
			document.getElementById("p"), heard = []; p.addEventListener("x", () => heard.push("kept"));
			p.addEventListener("x", () => heard.push("signalled"), { signal: c.signal });
			c.signal.addEventListener("abort", () => p.dispatchEvent(new Event("x")));
			p.dispatchEvent(new Event("x")); c.abort(); p.addEventListener("x", () => heard.push("late"),
			{ signal: c.signal }); p.dispatchEvent(new Event("x")); return heard.concat(thrown(() =>
			p.addEventListener("x", () => {}, { signal: {} }))).join() })()`,
			"kept,signalled,kept,kept,TypeError"},
	} {
		if got := s.eval(thrown + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}
