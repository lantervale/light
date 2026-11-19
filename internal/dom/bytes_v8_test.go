//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import (
	"encoding/hex"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/lantervale/light/internal/dom"
)

// every is the 256 values a byte takes, in their order.
func every() []byte {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	return all
}

// hexOf is what the page's hex() gives of bytes.
const hexOf = `const hex = (bytes) => Array.from(new Uint8Array(bytes), (b) => b.toString(16).padStart(2, "0"))
		.join("");
	var got = {}, done = 0;
	const note = (name) => (value) => { got[name] = value; done++; };`

// read is what the page noted, by name.
func (s *scripted) read(count int) map[string]any {
	s.t.Helper()
	for end := time.Now().Add(time.Second); s.eval(`done`) != strconv.Itoa(count); time.Sleep(time.Millisecond) {
		if time.Now().After(end) {
			s.t.Fatalf("the page read %s of %d: %s; the console says %s", s.eval(`done`), count,
				s.eval(`JSON.stringify(got)`), s.said())
		}
		s.rt.Frame()
		s.layout()
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(s.eval(`JSON.stringify(got)`)), &got); err != nil {
		s.t.Fatal(err)
	}
	return got
}

func (s *scripted) answering(path, kind string, body []byte) {
	s.answers["https://site.test"+path] = dom.Response{Status: 200, StatusText: "200 OK", Body: body,
		Header: map[string]string{"Content-Type": kind}}
}

func expect(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	for name, value := range want {
		if got[name] != value {
			t.Errorf("%s is %#v, want %#v", name, got[name], value)
		}
	}
}

func TestAFetchReadsTheBytesOfItsAnswerAsTheyCame(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.answering("/bytes", "application/octet-stream", every())
	s.answering("/none", "text/plain", []byte{})
	s.start(`<script>`+hexOf+`
		const get = () => fetch("/bytes");
		get().then((r) => r.arrayBuffer()).then((b) => note("buffer")(hex(b) + (b instanceof ArrayBuffer)));
		get().then((r) => r.bytes()).then((b) => note("bytes")(hex(b) + (b instanceof Uint8Array)));
		get().then((r) => r.blob()).then((blob) => blob.arrayBuffer().then((b) => {
			note("blob")(hex(b)); note("blobIs")(blob.size + " " + blob.type + " " + (blob instanceof Blob));
		}));
		get().then((r) => { const twin = r.clone(); return Promise.all([r.arrayBuffer(), twin.arrayBuffer()]); })
			.then(([a, b]) => { new Uint8Array(a)[0] = 9; note("clone")(hex(b)); });
		get().then((r) => r.arrayBuffer().then(() => r.text().then(() => "read twice", (e) => e.name + " " + r.bodyUsed)))
			.then(note("used"));
		get().then((r) => { const before = r.bodyUsed; r.bytes(); try { r.clone(); return "cloned"; } catch (e) {
			return before + " " + e.name; } }).then(note("cloneUsed"));
		fetch("/none").then((r) => r.arrayBuffer()).then((b) => note("none")(b.byteLength));
		get().then((r) => r.body).then(note("stream"));
		get().then((r) => r.formData()).then(() => "a form", (e) => e.name).then(note("form"));
		new Response(new Uint8Array([0, 255, 128])).arrayBuffer().then((b) => note("made")(hex(b)));
		new Response(new Blob([new Uint8Array([254])], { type: "image/png" })).blob()
			.then((b) => b.bytes().then((raw) => note("madeBlob")(hex(raw) + " " + b.type)));
		new Response("\u00e9").bytes().then((b) => note("madeText")(hex(b)));
		new Response("a=1&b=%C3%A9", { headers: { "Content-Type": "application/x-www-form-urlencoded" } }).formData()
			.then((d) => note("madeForm")(d.get("a") + d.get("b")));
		new Response().text().then((text) => note("empty")(text.length));
		new Request("/r", { method: "POST", body: new Uint8Array([1, 0, 255]) }).arrayBuffer()
			.then((b) => note("request")(hex(b)));
		</script>`, dom.Host{})
	all := hex.EncodeToString(every())
	expect(t, s.read(16), map[string]any{
		"buffer": all + "true", "bytes": all + "true", "blob": all, "blobIs": "256 application/octet-stream true",
		"clone": all, "used": "TypeError true", "cloneUsed": "false TypeError", "none": 0.0, "stream": nil,
		"form": "TypeError", "made": "00ff80", "madeBlob": "fe image/png", "madeText": "c3a9", "madeForm": "1é",
		"empty": 0.0, "request": "0100ff",
	})
	s.quiet()
}

func TestTheTextOfAnAnswerIsDecodedWhereAScriptAsksForText(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.answering("/latin", "text/plain; charset=ISO-8859-1", []byte("caf\xe9"))
	s.answering("/marked", "application/json", []byte("\xef\xbb\xbf{\"a\":\"\xc3\xa9\"}"))
	s.answering("/broken", "text/plain", []byte("a\xffb\xe2\x82c\x00d"))
	s.answering("/wide", "text/plain; charset=utf-8", []byte("\xff\xfeh\x00i\x00"))
	s.answering("/plain", "", []byte("caf\xc3\xa9"))
	s.answers["https://site.test/said"] = dom.Response{Status: 200, Text: "é"}
	s.start(`<script>`+hexOf+`
		for (const name of ["latin", "marked", "broken", "wide", "plain", "said"]) {
			fetch("/" + name).then((r) => r.text()).then(note("fetch " + name));
			const xhr = new XMLHttpRequest(); xhr.open("GET", "/" + name);
			xhr.onload = () => note("xhr " + name)(xhr.responseText); xhr.send();
		}
		fetch("/marked").then((r) => r.json()).then((value) => note("json")(value.a));
		fetch("/marked").then((r) => r.bytes()).then((b) => note("marked bytes")(hex(b)));
		fetch("/latin").then((r) => r.blob()).then((b) => b.text()).then(note("blob text"));
		const other = new XMLHttpRequest(); other.open("GET", "/latin");
		other.overrideMimeType("text/plain; charset=windows-1251");
		other.onload = () => { let late; try { other.overrideMimeType("text/html"); } catch (e) { late = e.name; }
			note("override")(other.responseText + " " + late + " " + other.response); };
		other.send();
		note("decoder")(new TextDecoder().decode(new Uint8Array([0xef, 0xbb, 0xbf, 0xc3, 0xa9, 0xff])));
		note("encoder")(hex(new TextEncoder().encode("\u00e9\u20ac\ud83d\ude00\ud800")));
		</script>`, dom.Host{})
	expect(t, s.read(18), map[string]any{
		"fetch latin": "caf�", "xhr latin": "café",
		"fetch marked": `{"a":"é"}`, "xhr marked": `{"a":"é"}`, "json": "é", "marked bytes": "efbbbf7b2261223a22c3a9227d",
		"fetch broken": "a�b�c\x00d", "xhr broken": "a�b�c\x00d",
		"fetch wide": "��h\x00i\x00", "xhr wide": "hi",
		"fetch plain": "café", "xhr plain": "café", "fetch said": "é", "xhr said": "é",
		"blob text": "caf�", "override": "cafй InvalidStateError cafй",
		"decoder": "é�", "encoder": "c3a9e282acf09f9880efbfbd",
	})
	s.quiet()
}

func TestAnXMLHttpRequestReadsItsAnswerAsTheTypeAskedFor(t *testing.T) {
	s := &scripted{t: t, answers: map[string]dom.Response{}}
	s.answering("/bytes", "image/png", every())
	s.answering("/json", "application/json; charset=iso-8859-1", []byte("{\"a\":\"\xc3\xa9\"}"))
	s.answering("/page", "text/html", []byte("<p id=in>caf\xc3\xa9</p>"))
	s.start(`<script>`+hexOf+`
		const ask = (path, type, read) => {
			const xhr = new XMLHttpRequest(); xhr.open("GET", path); xhr.responseType = type;
			let early;
			xhr.onreadystatechange = () => { if (xhr.readyState === 2) early = xhr.response; };
			xhr.onload = (e) => { let late; try { xhr.responseType = "text"; } catch (thrown) { late = thrown.name; }
				note(path + " " + type)(read(xhr, e) + " " + early + " " + late); };
			xhr.send();
		};
		const text = (xhr) => { try { return xhr.responseText; } catch (e) { return e.name; } };
		ask("/bytes", "arraybuffer", (xhr, e) => hex(xhr.response) + (xhr.response === xhr.response) +
			(xhr.response instanceof ArrayBuffer) + " " + text(xhr) + " " + e.loaded + "/" + e.total);
		ask("/bytes", "blob", (xhr) => xhr.response.size + " " + xhr.response.type + " " + (xhr.response === xhr.response) +
			" " + text(xhr));
		ask("/json", "json", (xhr) => xhr.response.a + " " + (xhr.response === xhr.response) + " " + text(xhr));
		ask("/bytes", "json", (xhr) => xhr.response);
		ask("/json", "text", (xhr) => xhr.response + " " + text(xhr));
		ask("/json", "", (xhr) => xhr.response);
		ask("/page", "document", (xhr) => xhr.response.getElementById("in").textContent + " " + text(xhr));
		const odd = new XMLHttpRequest(); odd.responseType = "moz-chunked"; note("odd")(odd.responseType + odd.response);
		const blob = new XMLHttpRequest(); blob.open("GET", "/bytes"); blob.responseType = "blob";
		blob.onload = () => blob.response.arrayBuffer().then((b) => note("blob bytes")(hex(b)));
		blob.send();
		</script>`, dom.Host{})
	all := hex.EncodeToString(every())
	expect(t, s.read(9), map[string]any{
		"/bytes arraybuffer": all + "truetrue InvalidStateError 256/256 null InvalidStateError",
		"/bytes blob":        "256 image/png true InvalidStateError null InvalidStateError",
		"/json json":         "é true InvalidStateError null InvalidStateError",
		"/bytes json":        "null null InvalidStateError",
		"/json text":         "{\"a\":\"Ã©\"} {\"a\":\"Ã©\"}  InvalidStateError",
		"/json ":             "{\"a\":\"Ã©\"}  InvalidStateError",
		"/page document":     "café InvalidStateError null InvalidStateError",
		"odd":                "", "blob bytes": all,
	})
	s.quiet()
}

func TestABlobHoldsBytes(t *testing.T) {
	s := run(t, `<script>`+hexOf+`
		const raw = new Uint8Array(256).map((_, i) => i);
		const blob = new Blob([raw.subarray(0, 128), raw.buffer.slice(128), "\u00e9", new Blob([new Uint8Array([7])])],
			{ type: "Application/X-Thing" });
		note("is")(blob.size + " " + blob.type);
		blob.arrayBuffer().then((b) => note("buffer")(hex(b)));
		blob.slice(254, 258).bytes().then((b) => note("slice")(hex(b)));
		blob.slice(-3, -1, "a/b").arrayBuffer().then((b) => note("tail")(hex(b)));
		note("sliced")(blob.slice(1, 3, "a/b").type + blob.slice(300).size + blob.slice(5, 2).size);
		blob.slice(128, 132).text().then(note("text"));
		const file = new File([raw.subarray(250)], "f.bin", { type: "x/y", lastModified: 5 });
		file.arrayBuffer()
			.then((b) => note("file")(hex(b) + " " + file.name + " " + file.size + file.type + file.lastModified));
		raw[0] = 99;
		blob.slice(0, 1).bytes().then((b) => note("kept")(hex(b)));
		</script>`)
	all := hex.EncodeToString(every())
	expect(t, s.read(8), map[string]any{
		"is": "259 application/x-thing", "buffer": all + "c3a907", "slice": "feffc3a9", "tail": "c3a9", "sliced": "a/b00",
		"text": "����", "file": "fafbfcfdfeff f.bin 6x/y5", "kept": "00",
	})
	s.quiet()
}
