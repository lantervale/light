//go:build gpu && v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// asLight has the test binary run as light itself, on the main thread,
// so a test drives the real program: its window, its GPU, its capture.
const asLight = "LIGHT_TEST_AS_LIGHT"

func TestMain(m *testing.M) {
	if os.Getenv(asLight) != "" {
		os.Args = append(os.Args[:1], strings.Fields(os.Getenv(asLight))...)
		main()
		return
	}
	os.Exit(m.Run())
}

type client struct {
	t    *testing.T
	base string
}

func (c *client) ask(method, path, body string) json.RawMessage {
	c.t.Helper()
	var sent io.Reader
	if method == http.MethodPost {
		sent = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, sent)
	if err != nil {
		c.t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	var whole struct {
		Value json.RawMessage `json:"value"`
	}
	if err = json.NewDecoder(res.Body).Decode(&whole); err != nil || res.StatusCode != http.StatusOK {
		c.t.Fatalf("%s %s is answered %d %s (%v)", method, path, res.StatusCode, whole.Value, err)
	}
	return whole.Value
}

func (c *client) shot(path string) image.Image {
	c.t.Helper()
	var encoded string
	if err := json.Unmarshal(c.ask("GET", path, ""), &encoded); err != nil {
		c.t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		c.t.Fatal(err)
	}
	picture, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		c.t.Fatal(err)
	}
	return picture
}

// same reports whether two pictures are one size and equal pixel by
// pixel, as a reftest compares them.
func same(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}

// lit starts a hidden light served over WebDriver and gives a client of
// its session.
func lit(t *testing.T) *client {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	_ = l.Close()
	cmd := exec.CommandContext(context.Background(), os.Args[0])
	cmd.Env = append(os.Environ(), asLight+"=drive --address "+address, "EMBER_HIDDEN=1", "EMBER_CRASH_DIR="+t.TempDir(),
		"LIGHT_DOWNLOADS="+t.TempDir())
	var said bytes.Buffer
	cmd.Stdout, cmd.Stderr = &said, &said
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Log(said.String())
		}
	})
	for range 200 {
		conn, failed := net.Dial("tcp", address)
		if failed == nil {
			_ = conn.Close()
			c := &client{t: t, base: "http://" + address}
			var made struct {
				SessionID string `json:"sessionId"`
			}
			if err = json.Unmarshal(c.ask("POST", "/session", `{"capabilities":{"alwaysMatch":{}}}`), &made); err != nil {
				t.Fatal(err)
			}
			c.base += "/session/" + made.SessionID
			return c
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("light never listened at %s: %s", address, said.String())
	return nil
}

func TestTheRealLightTakesScreenshotsAReftestCompares(t *testing.T) {
	pages := map[string]string{
		"/test": `<!doctype html><title>Test</title><style>html, body { margin: 0; background: rgb(30, 144, 255) }
			div { height: 50px }</style><body><div></div><div></div>`,
		"/match": `<!doctype html><title>Match</title><style>html, body { margin: 0; background: rgb(30, 144, 255) }
			</style><body>`,
		"/mismatch": `<!doctype html><title>Mismatch</title><style>html, body { margin: 0;
			background: rgb(30, 144, 255) } div { width: 100px; height: 100px; background: rgb(200, 0, 0) }</style>
			<body><div id="box"></div>`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, pages[r.URL.Path])
	}))
	t.Cleanup(srv.Close)
	c := lit(t)
	var chrome []int
	if err := json.Unmarshal(c.ask("POST", "/execute/sync", `{"script":"return [window.outerWidth - window.innerWidth,`+
		` window.outerHeight - window.innerHeight];","args":[]}`), &chrome); err != nil || len(chrome) != 2 {
		t.Fatalf("the window's frame is %v (%v)", chrome, err)
	}
	c.ask("POST", "/window/rect", `{"x":0,"y":0}`)
	size, _ := json.Marshal(map[string]int{"width": 800 + chrome[0], "height": 600 + chrome[1]})
	c.ask("POST", "/window/rect", string(size))
	shots := map[string]image.Image{}
	for _, path := range []string{"/test", "/match", "/mismatch"} {
		url, _ := json.Marshal(map[string]string{"url": srv.URL + path})
		c.ask("POST", "/url", string(url))
		shots[path] = c.shot("/screenshot")
		if got := shots[path].Bounds().Size(); got != image.Pt(800, 600) {
			t.Fatalf("the screenshot of %s is %v, want 800 by 600", path, got)
		}
	}
	for _, at := range []image.Point{{0, 0}, {799, 0}, {0, 599}, {799, 599}, {400, 300}} {
		if r, g, b, a := shots["/test"].At(at.X, at.Y).RGBA(); r>>8 != 30 || g>>8 != 144 || b>>8 != 255 || a>>8 != 255 {
			t.Fatalf("the pixel at %v of a page of one colour is %v", at, shots["/test"].At(at.X, at.Y))
		}
	}
	if !same(shots["/test"], shots["/match"]) {
		t.Error("two pages that look the same have screenshots that differ")
	}
	if same(shots["/test"], shots["/mismatch"]) {
		t.Error("two pages that look different have the same screenshot")
	}
	var found map[string]string
	_ = json.Unmarshal(c.ask("POST", "/element", `{"using":"css selector","value":"#box"}`), &found)
	box := c.shot("/element/" + found["element-6066-11e4-a52e-4f735466cecf"] + "/screenshot")
	if r, g, b, _ := box.At(50, 50).RGBA(); box.Bounds().Size() != image.Pt(100, 100) || r>>8 != 200 || g != 0 || b != 0 {
		t.Fatalf("the element's screenshot is %v, %v at its middle", box.Bounds().Size(), box.At(50, 50))
	}
}

func TestTheRealLightAnswersItsWindowToAScriptThatAsks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<title>Asks</title><p>asks</p>`)
	}))
	t.Cleanup(srv.Close)
	c := lit(t)
	url, _ := json.Marshal(map[string]string{"url": srv.URL + "/"})
	c.ask("POST", "/url", string(url))
	around := `[screenX, screenY, outerWidth, outerHeight, innerWidth, devicePixelRatio].join()`
	first := string(c.ask("POST", "/execute/sync", `{"script":"return `+around+`","args":[]}`))
	if strings.Count(first, ",") != 5 || strings.Contains(first, "NaN") || strings.Contains(first, "undefined") {
		t.Fatalf("the window says %s of itself", first)
	}
	if got := string(c.ask("POST", "/execute/sync", `{"script":"window.before = `+around+`; window.ok = confirm('sure?');`+
		` window.after = `+around+`; return 'went on'","args":[]}`)); got != "null" {
		t.Fatalf("a script that asks is answered %s", got)
	}
	if got := string(c.ask("GET", "/alert/text", "")); got != `"sure?"` {
		t.Fatalf("the real light asks %s", got)
	}
	c.ask("POST", "/alert/accept", "{}")
	got := string(c.ask("POST", "/execute/sync", `{"script":"return [window.ok, window.before, window.after, `+around+`]`+
		`.join('|')","args":[]}`))
	want := strings.Trim(first, `"`)
	if got != `"true|`+want+`|`+want+`|`+want+`"` {
		t.Fatalf("around its question the script read %s, the window being %s", got, first)
	}
}
