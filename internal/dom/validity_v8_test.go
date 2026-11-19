//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import "testing"

func TestAControlSaysWhetherWhatItHoldsIsValid(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><form id="f"><input id="req" required><input id="mail" type="email" value="no">
		<input id="url" type="url" value="nowhere"><input id="pat" pattern="[a-c]+" value="abd">
		<input id="num" type="number" min="2" max="6" step="2" value="7"><input id="date" type="date" min="2020-01-10"
		value="2020-01-02"><input id="box" type="checkbox" required><input id="r1" type="radio" name="g" required>
		<input id="r2" type="radio" name="g"><select id="sel" required><option value="">pick</option><option value="a">A
		</option></select><textarea id="area" required></textarea><input id="off" required disabled>
		<input id="ro" required readonly><input id="hid" type="hidden" required><button id="go">go</button>
		<button id="plain" type="button">b</button><fieldset id="set" disabled><legend><input id="inlegend" required>
		</legend><input id="inset" required></fieldset><output id="out"></output></form>
		<form id="fine"><input id="ok" value="x" required></form>`)
	const of = `const $ = (id) => document.getElementById(id), flags = (id) => { const v = $(id).validity;
		return ["valueMissing", "typeMismatch", "patternMismatch", "tooLong", "tooShort", "rangeUnderflow",
		"rangeOverflow", "stepMismatch", "badInput", "customError"].filter((k) => v[k]).join("+") +
		(v.valid ? "valid" : "") };`
	for _, c := range []struct{ name, script, want string }{
		{"what is wrong with a value is named", of + `["req", "mail", "url", "pat", "num", "date", "box", "r1", "r2",
			"sel", "area", "ok"].map(flags).join()`,
			"valueMissing,typeMismatch,typeMismatch,patternMismatch,rangeOverflow+stepMismatch,rangeUnderflow," +
				"valueMissing,valueMissing,valueMissing,valueMissing,valueMissing,valid"},
		{"a value put right is valid", of + `(() => { $("req").value = "x"; $("mail").value = "a@b.c"; $("url").value =
			"http://a/"; $("pat").value = "abc"; $("num").value = "4"; $("date").value = "2020-02-01"; $("box").checked =
			true; $("r2").checked = true; $("sel").value = "a"; $("area").value = "t";
			const seen = ["req", "mail", "url", "pat", "num", "date", "box", "r1", "r2", "sel", "area"].map(flags);
			$("req").value = ""; $("num").value = "7"; $("box").checked = false; $("r2").checked = false;
			$("sel").selectedIndex = 0; $("mail").value = "no"; return seen.join() })()`,
			"valid,valid,valid,valid,valid,valid,valid,valid,valid,valid,valid"},
		{"only a control that is used is validated", of + `["req", "off", "ro", "hid", "go", "plain", "inset", "inlegend",
			"set", "out"].map((id) => $(id).willValidate + ":" + flags(id) + ":" + $(id).checkValidity()).join()`,
			"true:valueMissing:false,false:valueMissing:true,false:valueMissing:true,false:valid:true,true:valid:true," +
				"false:valid:true,false:valueMissing:true,true:valueMissing:false,false:valid:true,false:valid:true"},
		{"a control checked says it is invalid, and its form with it", of + `(() => { const heard = [];
			$("f").addEventListener("invalid", (e) => heard.push(e.target.id + e.bubbles + e.cancelable), true);
			const seen = [$("req").checkValidity(), $("ok").reportValidity(), $("f").checkValidity(),
			$("fine").checkValidity(), $("fine").reportValidity()];
			return seen.concat(heard.map((h) => h.replace("falsetrue", "")).join(" "), heard[0],
			heard.includes("offfalsetrue"), $("req").validationMessage !== "",
			$("ok").validationMessage, $("off").validationMessage).join() })()`,
			"false,true,false,true,true,req req mail num box r1 r2 sel inlegend,reqfalsetrue,false,true,,"},
		{"a message of the page's makes a control invalid", of + `(() => { const ok = $("ok"), seen = [];
			ok.setCustomValidity("no good"); seen.push(flags("ok"), ok.validationMessage, ok.checkValidity(),
			ok.matches(":invalid"), $("fine").matches(":invalid")); ok.setCustomValidity(""); seen.push(flags("ok"),
			ok.matches(":valid"), $("fine").matches(":valid"), ok.validity === ok.validity, ok.validity instanceof
			ValidityState, $("out").validity.valid, $("set").validity.valid); return seen.join() })()`,
			"customError,no good,false,true,true,valid,true,true,true,true,true,true"},
	} {
		if got := s.eval(c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}

func TestSelectorsMatchWhatControlsAre(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><form id="f"><input id="req" required><input id="opt" value="v"
		placeholder="p"><input id="ph" placeholder="p"><input id="ro" readonly><input id="num" type="number" min="2"
		max="6" value="7"><input id="in" type="number" min="2" max="6" value="3"><input id="box" type="checkbox" checked>
		<input id="r1" type="radio" name="g"><input id="r2" type="radio" name="g"><button id="go">go</button>
		<button id="second">2</button><select id="sel"><option id="o1">a</option><option id="o2" selected>b</option>
		<optgroup id="grp" disabled><option id="o3">c</option></optgroup></select><textarea id="area"></textarea>
		<fieldset id="set" disabled><legend id="leg"><input id="inlegend"></legend><input id="inset"><fieldset id="deep">
		<button id="deepb">x</button></fieldset></fieldset><progress id="prog"></progress><progress id="done" value="1">
		</progress></form><p id="p" contenteditable><b id="b">x</b></p><a id="a" href="/x">a</a><a id="named">n</a>
		<x-thing id="x"></x-thing><div id="empty"><!-- c --></div><map><area id="ar" href="/y"></map>`)
	const ids = `const ids = (selector) => [...document.querySelectorAll(selector)].map((e) => e.id).join(" ");`
	for _, c := range []struct{ selector, want string }{
		{":required", "req"},
		{"form :optional", "opt ph ro num in box r1 r2 sel area inlegend inset"},
		{":placeholder-shown", "ph"},
		{"form :read-write", "req opt ph num in area inlegend"},
		{"input:read-only, #p:read-only, #b:read-only, #a:read-only", "ro box r1 r2 inset a"},
		{"#p:read-write, #b:read-write", "p b"},
		{":checked", "box o2"},
		{":default", "box go o2"},
		{":indeterminate", "r1 r2 prog"},
		{":disabled", "grp o3 set inset deep deepb"},
		{"form :enabled", "req opt ph ro num in box r1 r2 go second sel o1 o2 area inlegend"},
		{":invalid", "f req num"},
		{"input:valid", "opt ph in box r1 r2 inlegend"},
		{":in-range", "in"},
		{":out-of-range", "num"},
		{":link, :any-link", "a ar"},
		{"body :not(:defined)", "x"},
		{"#empty:empty, #p:empty", "empty"},
		{"form:has(:invalid) > :is(#req, #opt):not(:valid)", "req"},
	} {
		if got := s.eval(ids + `ids(` + "`" + c.selector + "`" + `)`); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.selector, got, c.want)
		}
	}
	for script, want := range map[string]string{
		`(() => { const i = document.createElement("input"); i.required = true; const d = document.createElement("div");
			d.append(i); return [i.matches(":invalid"), d.querySelector(":invalid") === i, i.closest(":valid")]
			.join() })()`: "true,true,",
		`document.getElementById("req").closest(":invalid").id +
			document.getElementById("num").matches(":out-of-range")`: "reqtrue",
	} {
		if got := s.eval(script); got != want {
			t.Errorf("%s:\n got %s\nwant %s", script, got, want)
		}
	}
	s.quiet()
}
