//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// bytesServer is what a server answers that no text holds: every value of a
// byte, a picture, and megabytes.
type bytesServer struct {
	every, picture, big []byte
	mu                  sync.Mutex
	// took is what each path was sent, and parts the files of a form.
	took  map[string][]byte
	parts map[string][]byte
}

func newBinary(t *testing.T) *bytesServer {
	t.Helper()
	b := &bytesServer{every: make([]byte, 256), big: make([]byte, 5<<20), took: map[string][]byte{},
		parts: map[string][]byte{}}
	for i := range b.every {
		b.every[i] = byte(i)
	}
	for i := range b.big {
		b.big[i] = byte(i*31 + i>>8)
	}
	var drawn bytes.Buffer
	if err := png.Encode(&drawn, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	b.picture = drawn.Bytes()
	return b
}

func (b *bytesServer) serve(w http.ResponseWriter, r *http.Request) {
	sent, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	b.took[r.URL.Path] = sent
	b.mu.Unlock()
	switch r.URL.Path {
	case "/every":
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(b.every)
	case "/picture":
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(b.picture)
	case "/zipped":
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Encoding", "gzip")
		zipped := gzip.NewWriter(w)
		_, _ = zipped.Write(b.every)
		_ = zipped.Close()
	case "/broken":
		_, _ = w.Write([]byte("\xef\xbb\xbfa\xff\xfe\x00b"))
	case "/latin":
		w.Header().Set("Content-Type", "text/plain; charset=iso-8859-1")
		_, _ = w.Write([]byte("caf\xe9"))
	case "/big":
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(b.big)
	case "/echo":
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		_, _ = w.Write(sent)
	case "/form":
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		form := multipart.NewReader(bytes.NewReader(sent), params["boundary"])
		for part, err := form.NextPart(); err == nil; part, err = form.NextPart() {
			held, _ := io.ReadAll(part)
			b.mu.Lock()
			b.parts[part.FormName()+" "+part.FileName()+" "+part.Header.Get("Content-Type")] = held
			b.mu.Unlock()
		}
	}
}

func (b *bytesServer) sent(path string) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.took[path]
}

const digest = `const hex = (bytes) => Array.from(new Uint8Array(bytes), (b) => b.toString(16).padStart(2, "0"))
		.join("");
	const xhr = (path, type, body) => new Promise((settle) => {
		const x = new XMLHttpRequest(); x.open(body ? "POST" : "GET", path); x.responseType = type;
		x.onload = () => settle(x); x.send(body || null);
	});
	const every = new Uint8Array(256).map((_, i) => i);`

func TestAPagesScriptReadsAndSendsBytesAsTheyAre(t *testing.T) {
	served := newBinary(t)
	site := wired(t, served.serve)
	s := show(t)
	saw := s.asks(site, digest+`
		const read = (name, bytes) => seen.push(name + " " + hex(bytes));
		Promise.all([
			fetch("/every").then((r) => r.arrayBuffer()).then((b) => read("every buffer", b)),
			fetch("/every").then((r) => r.bytes()).then((b) => read("every bytes", b)),
			fetch("/every").then((r) => r.blob()).then((b) => b.arrayBuffer()).then((b) => read("every blob", b)),
			xhr("/every", "arraybuffer").then((x) => read("every xhr", x.response)),
			xhr("/every", "blob").then((x) => x.response.arrayBuffer()).then((b) => read("every xhr blob", b)),
			fetch("/zipped").then((r) => r.arrayBuffer()).then((b) => read("zipped", b)),
			fetch("/picture").then((r) => r.blob()).then((b) => b.arrayBuffer().then((raw) => read("picture " + b.type, raw))),
			fetch("/broken").then((r) => r.arrayBuffer()).then((b) => read("broken", b)),
			fetch("/broken").then((r) => r.text()).then((text) => read("broken text", new TextEncoder().encode(text))),
			fetch("/latin").then((r) => r.text()).then((text) => seen.push("latin fetch " + escape(text))),
			xhr("/latin", "").then((x) => seen.push("latin xhr " + escape(x.responseText))),
			fetch("/echo", { method: "POST", body: every }).then((r) => r.arrayBuffer()).then((b) => read("echo view", b)),
			fetch("/echo", { method: "POST", body: every.buffer }).then((r) => r.bytes()).then((b) => read("echo buffer", b)),
			fetch("/echo", { method: "POST", body: new Blob([every], { type: "x/y" }) })
				.then((r) => r.blob()).then((b) => b.arrayBuffer().then((raw) => read("echo blob " + b.type, raw))),
			xhr("/echo", "arraybuffer", every).then((x) => read("echo xhr", x.response)),
			(() => { const form = new FormData(); form.append("file", new File([every], "all.bin", { type: "x/y" }));
				form.append("name", "caf\u00e9"); return fetch("/form", { method: "POST", body: form }); })(),
		]).then(done, (e) => { seen.push(String(e)); done(); });`)
	all := hex.EncodeToString(served.every)
	equal(t, "the script read", saw, []string{
		"broken efbbbf61fffe0062", "broken text 61efbfbdefbfbd0062",
		"echo blob x/y " + all, "echo buffer " + all, "echo view " + all, "echo xhr " + all,
		"every blob " + all, "every buffer " + all, "every bytes " + all, "every xhr " + all, "every xhr blob " + all,
		"latin fetch caf%uFFFD", "latin xhr caf%E9",
		"picture image/png " + hex.EncodeToString(served.picture), "zipped " + all,
	})
	if got := served.sent("/echo"); !bytes.Equal(got, served.every) {
		t.Errorf("the server was sent %x", got)
	}
	served.mu.Lock()
	defer served.mu.Unlock()
	if file := served.parts["file all.bin x/y"]; !bytes.Equal(file, served.every) || len(served.parts) != 2 ||
		string(served.parts["name  "]) != "café" {
		t.Errorf("the form's parts are %q", served.parts)
	}
}

func TestAPagesScriptReadsMegabytesInNoTime(t *testing.T) {
	served := newBinary(t)
	site := wired(t, served.serve)
	s := show(t)
	saw := s.asks(site, `const began = performance.now();
		fetch("/big").then((r) => r.arrayBuffer()).then((buffer) => {
			const took = performance.now() - began, bytes = new Uint8Array(buffer);
			seen.push("length " + bytes.length, "took " + Math.round(took));
			return fetch("/echo", { method: "POST", body: bytes }).then((r) => r.bytes());
		}).then((back) => { seen.push("back " + back.length + " " + back[12345] + " " + back[back.length - 1]); done(); });`)
	if len(saw) != 3 || saw[1] != "length 5242880" || !strings.HasPrefix(saw[2], "took ") ||
		saw[0] != "back 5242880 "+fmt.Sprintf("%d %d", served.big[12345], served.big[len(served.big)-1]) {
		t.Fatalf("the script saw %q", saw)
	}
	if sha256.Sum256(served.sent("/echo")) != sha256.Sum256(served.big) {
		t.Errorf("5 MB came back to the server changed")
	}
	t.Logf("5 MB fetched and read as an ArrayBuffer: %s ms in the page", strings.TrimPrefix(saw[2], "took "))
}
