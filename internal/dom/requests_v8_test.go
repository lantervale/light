//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/light/internal/dom"
	"github.com/lantervale/light/pkg/fetch"
)

// asked is the request the page made for a path of its site.
func (s *scripted) asked(path string) dom.Request {
	s.t.Helper()
	for _, q := range s.fetched {
		if q.Address == "https://site.test"+path {
			return q
		}
	}
	s.t.Fatalf("the page never asked for %s", path)
	return dom.Request{}
}

func TestTheBodiesAScriptSends(t *testing.T) {
	s := run(t, `<script>var done = 0;
		const bodies = {
			text: "a=1&é",
			params: new URLSearchParams({ a: "1 2", "b&": "é" }),
			blob: new Blob(["<p>", new Uint8Array([0, 255, 128])], { type: "Text/HTML" }),
			untyped: new Blob(["x"]),
			buffer: new Uint8Array([0, 1, 254, 255]).buffer,
			view: new Uint8Array([9, 0, 255, 128, 13, 10]).subarray(1, 5),
			words: new Uint16Array([0x4142]),
			json: JSON.stringify({ n: 1 }),
			number: 42,
			empty: "",
		};
		for (const name of Object.keys(bodies)) {
			fetch("/fetch/" + name, { method: "POST", body: bodies[name] }).then(() => done++);
			const xhr = new XMLHttpRequest(); xhr.open("POST", "/xhr/" + name); xhr.onloadend = () => done++;
			xhr.send(bodies[name]);
		}
		fetch("/typed", { method: "PUT", body: "{}", headers: { "Content-Type": "application/json" } }).then(() => done++);
		const typed = new XMLHttpRequest(); typed.open("PUT", "/xhr/typed"); typed.onloadend = () => done++;
		typed.setRequestHeader("Content-Type", "application/json"); typed.send("{}");
		fetch("/none", { method: "POST" }).then(() => done++);
		const got = new XMLHttpRequest(); got.open("GET", "/xhr/get"); got.onloadend = () => done++; got.send("ignored");
		var thrown = [];
		for (const init of [{ body: "x" }, { method: "HEAD", body: "x" }, { method: "POST", body: { getReader() {} } }]) {
			fetch("/thrown", init).catch((e) => thrown.push(e.name));
		}</script>`)
	s.frames("the answers", func() bool { return s.eval(`done`) == "24" })
	const text, form = "text/plain;charset=UTF-8", "application/x-www-form-urlencoded;charset=UTF-8"
	for name, want := range map[string]struct{ body, kind string }{
		"text": {"a=1&é", text}, "params": {"a=1+2&b%26=%C3%A9", form}, "blob": {"<p>\x00\xff\x80", "text/html"},
		"untyped": {"x", ""}, "buffer": {"\x00\x01\xfe\xff", ""}, "view": {"\x00\xff\x80\r", ""}, "words": {"BA", ""},
		"json": {`{"n":1}`, text}, "number": {"42", text}, "empty": {"", text},
	} {
		for _, api := range []string{"/fetch/", "/xhr/"} {
			q := s.asked(api + name)
			if string(q.Body) != want.body || q.Body == nil || q.Header["content-type"] != want.kind ||
				q.Method != http.MethodPost {
				t.Errorf("%s%s carried %q as %q, want %q as %q", api, name, q.Body, q.Header["content-type"], want.body,
					want.kind)
			}
		}
	}
	for _, path := range []string{"/typed", "/xhr/typed"} {
		if q := s.asked(path); string(q.Body) != "{}" || q.Header["content-type"] != "application/json" {
			t.Errorf("%s carried %q as %q", path, q.Body, q.Header["content-type"])
		}
	}
	for _, path := range []string{"/none", "/xhr/get"} {
		if q := s.asked(path); q.Body != nil || q.Header["content-type"] != "" {
			t.Errorf("%s carried %q as %q, want nothing", path, q.Body, q.Header["content-type"])
		}
	}
	if got := s.eval(`thrown.join()`); got != "TypeError,TypeError,TypeError" {
		t.Errorf("a GET with a body, a HEAD with one and a stream were answered %q", got)
	}
}

func TestAScriptSendsAFormDataAsMultipart(t *testing.T) {
	s := run(t, `<form id="f"><input name="q" value="kept"><input type="checkbox" name="off"></form><script>var done = 0;
		const data = new FormData();
		data.append("name", "Zoé \"Z\"\nline");
		data.append("file", new File(["a\r\nb", new Uint8Array([0, 255])], "né\".txt", { type: "text/x-test" }));
		data.append("blob", new Blob(["raw"]));
		data.append("named", new Blob(["n"], { type: "a/b" }), "given.bin");
		fetch("/form", { method: "POST", body: data }).then(() => done++);
		const xhr = new XMLHttpRequest(); xhr.open("POST", "/xhr/form"); xhr.onloadend = () => done++;
		xhr.send(new FormData(document.getElementById("f")));</script>`)
	s.frames("the answers", func() bool { return s.eval(`done`) == "2" })
	q := s.asked("/form")
	kind := q.Header["content-type"]
	if !regexp.MustCompile(`^multipart/form-data; boundary=----lightFormBoundary[0-9a-z]{16}$`).MatchString(kind) {
		t.Fatalf("the form was sent as %q", kind)
	}
	_, params, err := mime.ParseMediaType(kind)
	if err != nil {
		t.Fatal(err)
	}
	want := "--B\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\nZoé \"Z\"\r\nline\r\n" +
		"--B\r\nContent-Disposition: form-data; name=\"file\"; filename=\"né%22.txt\"\r\nContent-Type: text/x-test\r\n\r\n" +
		"a\r\nb\x00\xff\r\n" +
		"--B\r\nContent-Disposition: form-data; name=\"blob\"; filename=\"blob\"\r\n" +
		"Content-Type: application/octet-stream\r\n\r\nraw\r\n" +
		"--B\r\nContent-Disposition: form-data; name=\"named\"; filename=\"given.bin\"\r\nContent-Type: a/b\r\n\r\nn\r\n" +
		"--B--\r\n"
	if got := strings.ReplaceAll(string(q.Body), params["boundary"], "B"); got != want {
		t.Errorf("the form carried\n%q\nwant\n%q", got, want)
	}
	var parts []string
	for r := multipart.NewReader(bytes.NewReader(q.Body), params["boundary"]); ; {
		part, ended := r.NextPart()
		if errors.Is(ended, io.EOF) {
			break
		}
		if ended != nil {
			t.Fatal(ended)
		}
		value, _ := io.ReadAll(part)
		parts = append(parts, fmt.Sprintf("%s %q %q", part.FormName(), part.FileName(), value))
	}
	if got, read := strings.Join(parts, "|"), `name "" "Zoé \"Z\"\r\nline"|file "né%22.txt" "a\r\nb\x00\xff"|`+
		`blob "blob" "raw"|named "given.bin" "n"`; got != read {
		t.Errorf("a server reads the form as %s, want %s", got, read)
	}
	plain := s.asked("/xhr/form")
	_, params, _ = mime.ParseMediaType(plain.Header["content-type"])
	if got, fields := strings.ReplaceAll(string(plain.Body), params["boundary"], "B"),
		"--B\r\nContent-Disposition: form-data; name=\"q\"\r\n\r\nkept\r\n--B--\r\n"; got != fields {
		t.Errorf("a form's fields carried %q, want %q", got, fields)
	}
}

func TestTheMethodsAndTheModesOfAScriptsRequest(t *testing.T) {
	s := run(t, `<script>var done = 0, thrown = [];
		for (const method of ["GET", "head", "Post", "put", "PATCH", "patch", "delete", "OPTIONS", "Custom"]) {
			fetch("/fetch/" + method, { method }).then(() => done++);
			const xhr = new XMLHttpRequest(); xhr.open(method, "/xhr/" + method); xhr.onloadend = () => done++; xhr.send();
		}
		const tried = (name, run) => {
			try { run(); thrown.push(name + " sent"); } catch (e) { thrown.push(name + " " + e.name); }
		};
		for (const method of ["CONNECT", "trace", "Track", "BAD METHOD", ""]) {
			tried("fetch " + method, () => new Request("/never", { method }));
			tried("xhr " + method, () => new XMLHttpRequest().open(method, "/never"));
			fetch("/never", { method }).catch((e) => thrown.push("rejected " + e.name));
		}
		tried("mode", () => new Request("/never", { mode: "navigate" }));
		tried("credentials", () => new Request("/never", { credentials: "all" }));
		tried("redirect", () => new Request("/never", { redirect: "maybe" }));
		tried("no-cors PUT", () => new Request("/never", { mode: "no-cors", method: "PUT" }));
		tried("password", () => new Request("https://user:pw@site.test/never"));
		tried("unsent header", () => new XMLHttpRequest().setRequestHeader("X-A", "1"));
		fetch("/plain").then(() => done++);
		fetch("/told", { method: "POST", mode: "no-cors", credentials: "omit", redirect: "manual", keepalive: true,
			headers: { "X-A": "1", Cookie: "c=1" } }).then(() => done++);
		const copy = new Request(new Request("/copied", { method: "PUT", body: "b", mode: "same-origin",
			credentials: "include", redirect: "error", headers: [["X-B", "2"]] }));
		fetch(copy).then(() => done++);
		const xhr = new XMLHttpRequest(); xhr.open("POST", "/with"); xhr.withCredentials = true;
		xhr.setRequestHeader("X-A", "1"); xhr.setRequestHeader("X-A", "2"); xhr.onloadend = () => done++; xhr.send("b");
		</script>`)
	s.frames("the answers", func() bool { return s.eval(`done`) == "22" })
	for method, want := range map[string]string{"GET": "GET", "head": "HEAD", "Post": "POST", "put": "PUT",
		"PATCH": "PATCH", "patch": "patch", "delete": "DELETE", "OPTIONS": "OPTIONS", "Custom": "Custom"} {
		for _, api := range []string{"/fetch/", "/xhr/"} {
			if q := s.asked(api + method); q.Method != want {
				t.Errorf("%s%s was sent as %s, want %s", api, method, q.Method, want)
			}
		}
	}
	want := []string{
		"fetch CONNECT TypeError", "xhr CONNECT SecurityError", "fetch trace TypeError", "xhr trace SecurityError",
		"fetch Track TypeError", "xhr Track SecurityError", "fetch BAD METHOD TypeError", "xhr BAD METHOD SyntaxError",
		"fetch  TypeError", "xhr  SyntaxError", "mode TypeError", "credentials TypeError", "redirect TypeError",
		"no-cors PUT TypeError", "password TypeError", "unsent header InvalidStateError",
		"rejected TypeError", "rejected TypeError", "rejected TypeError", "rejected TypeError", "rejected TypeError",
	}
	if got := strings.Split(s.eval(`thrown.join("|")`), "|"); !slices.Equal(got, want) {
		t.Errorf("what no script may send was answered\n%q\nwant\n%q", got, want)
	}
	for _, q := range s.fetched {
		if strings.HasSuffix(q.Address, "/never") {
			t.Errorf("light was asked for %s %s", q.Method, q.Address)
		}
	}
	say := func(q dom.Request) string {
		return fmt.Sprintf("%s %s %s %s %v %q %v", q.Method, q.Mode, q.Credentials, q.Redirect, q.Keepalive, q.Body,
			q.Header)
	}
	for path, want := range map[string]string{
		"/plain":  `GET cors same-origin follow false "" map[]`,
		"/told":   `POST no-cors omit manual true "" map[cookie:c=1 x-a:1]`,
		"/copied": `PUT same-origin include error false "b" map[content-type:text/plain;charset=UTF-8 x-b:2]`,
		"/with":   `POST cors include follow false "b" map[content-type:text/plain;charset=UTF-8 x-a:1, 2]`,
	} {
		if got := say(s.asked(path)); got != want {
			t.Errorf("%s was asked for as %s, want %s", path, got, want)
		}
	}
}

func TestWhatAScriptSeesOfItsAnswer(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/own": {Status: 201, StatusText: "Created", Text: "own", Kind: "basic",
			Header: map[string]string{"X-Own": "1", "Content-Type": "text/plain"}},
		"https://other.test/shared": {Status: 200, Text: "shared", Kind: "cors", Redirected: true,
			Address: "https://other.test/landed", Header: map[string]string{"Content-Type": "text/plain"}},
		"https://other.test/opaque":   {Kind: "opaque"},
		"https://site.test/moved":     {Kind: "opaqueredirect"},
		"https://other.test/unsaid":   {Status: 200, Text: "secret", Header: map[string]string{"X-Secret": "s"}},
		"https://site.test/elsewhere": {Status: 200, Text: "secret", Address: "https://other.test/unsaid"},
		"https://other.test/open": {Status: 200, Text: "open", Header: map[string]string{
			"Access-Control-Allow-Origin": "https://site.test", "Access-Control-Expose-Headers": "X-Shown",
			"X-Shown": "v", "X-Secret": "s", "Content-Type": "text/plain", "Set-Cookie": "a=b"}},
		"https://other.test/blocked": {Err: &fetch.BlockedError{Address: "https://other.test/blocked", CORS: true, Sent: true,
			Origin: "https://site.test", Why: "No 'Access-Control-Allow-Origin' header is present on the requested resource."}},
		"http://other.test/clear": {Err: &fetch.BlockedError{Address: "http://other.test/clear", Mixed: true,
			Page: address, Why: "mixed"}},
		"https://site.test/down": {Err: errors.New("no route to host")},
	}}
	s.start(`<script>var seen = {}, left = 0;
		const see = (name, input, init) => { left++; fetch(input, init).then(
			(r) => r.text().then((text) => { seen[name] = [r.type, r.status, r.statusText, r.ok, r.redirected, r.url, text,
				[...r.headers.keys()].sort().join()].join(" "); left--; }),
			(e) => { seen[name] = e.name + ": " + e.message; left--; }); };
		see("own", "/own", { method: "POST", body: "b" });
		see("shared", "https://other.test/shared", { method: "POST", body: "b" });
		see("opaque", "https://other.test/opaque", { method: "POST", mode: "no-cors" });
		see("moved", "/moved", { redirect: "manual" });
		see("unsaid", "https://other.test/unsaid");
		see("elsewhere", "/elsewhere");
		see("open", "https://other.test/open");
		see("blocked", "https://other.test/blocked", { method: "PUT" });
		see("clear", "http://other.test/clear", { method: "POST" });
		see("down", "/down", { method: "DELETE" });
		see("file", "file:///etc/passwd");
		const xhr = new XMLHttpRequest(); xhr.open("POST", "https://other.test/blocked"); left++;
		xhr.onloadend = () => { seen.xhr = [xhr.status, xhr.readyState, JSON.stringify(xhr.responseText),
			JSON.stringify(xhr.getAllResponseHeaders())].join(" "); left--; };
		xhr.send("b");</script>`, dom.Host{})
	s.frames("the answers", func() bool { return s.eval(`left`) == "0" })
	for name, want := range map[string]string{
		"own":       "basic 201 Created true false https://site.test/own own content-type,x-own",
		"shared":    "cors 200  true true https://other.test/landed shared content-type",
		"opaque":    "opaque 0  false false   ",
		"moved":     "opaqueredirect 0  false false   ",
		"unsaid":    "TypeError: Failed to fetch",
		"elsewhere": "TypeError: Failed to fetch",
		"open":      "cors 200  true false https://other.test/open open content-type,x-shown",
		"blocked":   "TypeError: Failed to fetch",
		"clear":     "TypeError: Failed to fetch",
		"down":      "TypeError: Failed to fetch",
		"file":      "TypeError: Failed to fetch",
		"xhr":       `0 4 "" ""`,
	} {
		if got := s.eval(`seen.` + name); got != want {
			t.Errorf("of %s the script sees %q, want %q", name, got, want)
		}
	}
	said := s.said()
	for _, want := range []string{
		"Access to fetch at 'https://other.test/unsaid' from origin 'https://site.test' has been blocked by CORS policy: " +
			"No 'Access-Control-Allow-Origin' header is present on the requested resource.",
		"Access to fetch at 'https://other.test/blocked' from origin 'https://site.test' has been blocked by CORS policy: ",
		"Access to XMLHttpRequest at 'https://other.test/blocked' from origin 'https://site.test' has been blocked by " +
			"CORS policy: ",
		"Mixed Content: The page at '" + address + "' was loaded over HTTPS, but requested an insecure resource " +
			"'http://other.test/clear'. This request has been blocked; the content must be served over HTTPS.",
		"Failed to load https://site.test/down: no route to host.",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the console lacks %q in\n%s", want, said)
		}
	}
	if strings.Count(said, "blocked by CORS policy") != 4 {
		t.Errorf("the console says\n%s", said)
	}
}

func TestTheEventsOfAnXMLHttpRequest(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/ok":      {Status: 200, StatusText: "OK", Text: "fine", Kind: "basic"},
		"https://site.test/missing": {Status: 404, StatusText: "Not Found", Text: "none", Kind: "basic"},
		"https://site.test/down":    {Err: errors.New("no route")},
		"https://other.test/closed": {Status: 200, Text: "secret"},
		"https://site.test/slow":    {Status: pending},
		"https://site.test/slower":  {Status: pending},
		"https://site.test/late":    {Status: pending},
	}}
	s.start(`<script>var logs = {}, left = 0;
		const watch = (name, method, url, before) => {
			const xhr = new XMLHttpRequest(), log = logs[name] = [];
			left++;
			xhr.onreadystatechange = () => log.push("readystatechange " + xhr.readyState);
			for (const type of ["loadstart", "progress", "load", "error", "abort", "timeout"]) {
				xhr.addEventListener(type, () => log.push(type + " " + xhr.readyState + " " + xhr.status));
			}
			xhr.addEventListener("loadend", () => { log.push("loadend " + xhr.readyState + " " + xhr.status + " " +
				JSON.stringify(xhr.responseText)); left--; });
			xhr.open(method, url);
			if (before) before(xhr);
			xhr.send(method === "GET" ? null : "b");
			return xhr;
		};
		watch("ok", "POST", "/ok");
		watch("missing", "PUT", "/missing");
		watch("down", "DELETE", "/down");
		watch("closed", "POST", "https://other.test/closed");
		const slow = watch("slow", "POST", "/slow");
		setTimeout(() => { slow.abort(); logs.slow.push("after " + slow.readyState); slow.abort(); }, 10);
		watch("slower", "POST", "/slower", (xhr) => { xhr.timeout = 20; });
		const again = watch("again", "POST", "/late");
		setTimeout(() => { again.open("GET", "/ok"); again.send(); }, 10);
		var after = [];
		const done = watch("done", "GET", "/ok");
		done.addEventListener("loadend", () => {
			done.abort();
			after.push(done.readyState, done.status, done.responseText);
		});
		</script>`, dom.Host{})
	s.frames("the requests' ends", func() bool { return s.eval(`left`) == "0" })
	for name, want := range map[string]string{
		"ok": "readystatechange 1|loadstart 1 0|readystatechange 2|readystatechange 3|readystatechange 4|load 4 200|" +
			`loadend 4 200 "fine"`,
		"missing": "readystatechange 1|loadstart 1 0|readystatechange 2|readystatechange 3|readystatechange 4|" +
			`load 4 404|loadend 4 404 "none"`,
		"down":   `readystatechange 1|loadstart 1 0|readystatechange 4|error 4 0|loadend 4 0 ""`,
		"closed": `readystatechange 1|loadstart 1 0|readystatechange 4|error 4 0|loadend 4 0 ""`,
		"slow":   `readystatechange 1|loadstart 1 0|readystatechange 4|abort 4 0|loadend 4 0 ""|after 0`,
		"slower": `readystatechange 1|loadstart 1 0|readystatechange 4|timeout 4 0|loadend 4 0 ""`,
		"again": "readystatechange 1|loadstart 1 0|readystatechange 1|loadstart 1 0|readystatechange 2|readystatechange 3|" +
			`readystatechange 4|load 4 200|loadend 4 200 "fine"`,
	} {
		if got := s.eval(`logs.` + name + `.join("|")`); got != want {
			t.Errorf("%s:\n%s\nwant\n%s", name, got, want)
		}
	}
	if got := s.eval(`after.join()`); got != "0,0," {
		t.Errorf("an XMLHttpRequest aborted once done is left as %q", got)
	}
	slices.Sort(s.stopped)
	if want := []string{"https://site.test/late", "https://site.test/slow", "https://site.test/slower"}; !slices.Equal(
		s.stopped, want) {
		t.Errorf("light was told to stop %q, want %q", s.stopped, want)
	}
}

func TestSendBeaconAndRequestsThatOutliveThePage(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{
		"https://site.test/kept":     {Status: pending},
		"https://site.test/dropped":  {Status: pending},
		"https://stats.test/collect": {Status: pending},
		"https://site.test/aborted":  {Status: pending},
	}}
	s.start(`<script>var said = [], failed = "";
		said.push(navigator.sendBeacon("https://stats.test/collect", "seen=1"));
		said.push(navigator.sendBeacon("/beacon/blob", new Blob(["{}"], { type: "application/json" })));
		said.push(navigator.sendBeacon("/beacon/params", new URLSearchParams({ a: "1" })));
		said.push(navigator.sendBeacon("/beacon/none"));
		said.push(navigator.sendBeacon("/beacon/big", "x".repeat(64 * 1024 + 1)));
		try { navigator.sendBeacon("ftp://site.test/x", "x"); } catch (e) { said.push(e.name); }
		fetch("/kept", { method: "POST", body: "k", keepalive: true });
		fetch("/dropped", { method: "POST", body: "d" });
		fetch("/big", { method: "POST", body: "x".repeat(64 * 1024 + 1), keepalive: true })
			.catch((e) => { failed = e.name; });
		const stop = new AbortController();
		var aborted = "";
		fetch("/aborted", { method: "POST", body: "a", keepalive: true, signal: stop.signal })
			.catch((e) => { aborted = e.name; });
		stop.abort();</script>`, dom.Host{})
	s.frames("the fetches", func() bool { return s.eval(`failed`) != "" && s.eval(`aborted`) != "" })
	if got := s.eval(`said.join()`); got != "true,true,true,true,false,TypeError" {
		t.Errorf("sendBeacon said %s", got)
	}
	if got := s.eval(`failed + " " + aborted`); got != "TypeError AbortError" {
		t.Errorf("a request too big to outlive its page, and one stopped, ended as %s", got)
	}
	var beacons []string
	for _, q := range s.fetched {
		if strings.Contains(q.Address, "/big") {
			t.Errorf("light was asked for %s", q.Address)
		}
		if strings.Contains(q.Address, "/beacon/") || strings.Contains(q.Address, "stats.test") {
			beacons = append(beacons, fmt.Sprintf("%s %s %s %s %v %q %q", q.Method, q.Address, q.Mode, q.Credentials,
				q.Keepalive, q.Body, q.Header["content-type"]))
		}
	}
	if want := []string{
		`POST https://stats.test/collect no-cors include true "seen=1" "text/plain;charset=UTF-8"`,
		`POST https://site.test/beacon/blob cors include true "{}" "application/json"`,
		`POST https://site.test/beacon/params no-cors include true "a=1" "application/x-www-form-urlencoded;charset=UTF-8"`,
		`POST https://site.test/beacon/none no-cors include true "" ""`,
	}; !slices.Equal(beacons, want) {
		t.Errorf("the beacons were\n%s\nwant\n%s", strings.Join(beacons, "\n"), strings.Join(want, "\n"))
	}
	if want := []string{"https://site.test/aborted"}; !slices.Equal(s.stopped, want) {
		t.Errorf("before the page was left light was told to stop %q, want %q", s.stopped, want)
	}
	s.rt.Close()
	if want := []string{"https://site.test/aborted", "https://site.test/dropped"}; !slices.Equal(s.stopped, want) {
		t.Errorf("once the page was left light was told to stop %q, want %q", s.stopped, want)
	}
}
