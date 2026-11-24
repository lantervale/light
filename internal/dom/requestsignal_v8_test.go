//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import "testing"

func TestARequestHasASignalOfItsOwnThatFollowsTheOneItWasGiven(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="p"></p>`)
	for _, c := range []struct{ name, script, want string }{
		{"a page that looks for it finds fetch can be called off", `[typeof Request === "function" &&
			Request.prototype.hasOwnProperty("signal"), typeof AbortController, Object.keys(new Request("/r"))
			.includes("signal")].join()`, "true,function,false"},
		{"a request given none has one that never aborts", `(r => [r.signal instanceof AbortSignal, r.signal.aborted,
			r.signal === r.signal, r.clone().signal instanceof AbortSignal].join())(new Request("/r"))`,
			"true,false,true,true"},
		{"it follows the one given, and so do its copies", `(() => { const c = new AbortController(), r =
			new Request("/r", { signal: c.signal }), copy = new Request(r), heard = [];
			r.signal.addEventListener("abort", () => heard.push("request"));
			const before = [r.signal === c.signal, r.signal.aborted, copy.signal.aborted]; c.abort("why");
			return before.concat(heard, r.signal.aborted, r.signal.reason, copy.signal.reason).join() })()`,
			"false,false,false,request,true,why,why"},
		{"what is no signal is refused", `(() => { try { new Request("/r", { signal: {} }); return "taken" }
			catch (e) { return e.name } })()`, "TypeError"},
		{"a listener takes the signal of a request", `(() => { const c = new AbortController(), r = new Request("/r",
			{ signal: c.signal }), p = document.getElementById("p"), heard = [];
			p.addEventListener("x", () => heard.push("heard"), { signal: r.signal });
			p.dispatchEvent(new Event("x")); c.abort(); p.dispatchEvent(new Event("x")); return heard.join() })()`,
			"heard"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}
