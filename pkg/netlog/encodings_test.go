package netlog_test

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/lantervale/light/pkg/netlog"
)

func TestTheEncodingsAskedForAreThoseUndone(t *testing.T) {
	text := strings.Repeat("a { color: red }\n", 500)
	var zlibbed, bare bytes.Buffer
	z := zlib.NewWriter(&zlibbed)
	_, _ = io.WriteString(z, text)
	_ = z.Close()
	f, _ := flate.NewWriter(&bare, flate.DefaultCompression)
	_, _ = io.WriteString(f, text)
	_ = f.Close()
	var brotlied bytes.Buffer
	b := brotli.NewWriter(&brotlied)
	_, _ = io.WriteString(b, text)
	_ = b.Close()
	var asked []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.Header.Get("Accept-Encoding"))
		switch r.URL.Path {
		case "/zlib":
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(zlibbed.Bytes())
		case "/bare":
			w.Header().Set("Content-Encoding", "Deflate")
			_, _ = w.Write(bare.Bytes())
		case "/br":
			w.Header().Set("Content-Encoding", "br")
			_, _ = w.Write(brotlied.Bytes())
		case "/zstd":
			w.Header().Set("Content-Encoding", "zstd")
			_, _ = io.WriteString(w, text)
		}
	}))
	t.Cleanup(srv.Close)
	l := netlog.New(10)
	for _, path := range []string{"/zlib", "/bare", "/br", "/zstd"} {
		req, err := http.NewRequestWithContext(netlog.With(t.Context(), l, "document", netlog.Stylesheet),
			http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := (&http.Client{Transport: &netlog.Transport{Base: srv.Client().Transport}}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		left := resp.Header.Get("Content-Encoding")
		if err != nil || string(got) != text || (left != "") != (path == "/zstd") {
			t.Errorf("%s is read as %d bytes (%v), still encoded %q", path, len(got), err, left)
		}
	}
	for _, encodings := range asked {
		if encodings != "gzip, deflate, br" {
			t.Errorf("the server was asked for %q", encodings)
		}
	}
	for _, row := range l.Rows() {
		wire := row.Transferred < row.Size/4
		if row.Unzipped != !strings.HasSuffix(row.Address, "/zstd") || row.Unzipped != wire {
			t.Errorf("%s: undone %t, %d bytes read of %d sent", row.Address, row.Unzipped, row.Size, row.Transferred)
		}
	}
}

func TestBrotliIsAskedForOnlyOverTLS(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		asked = r.Header.Get("Accept-Encoding")
	}))
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(netlog.With(t.Context(), netlog.New(10), "document", netlog.Stylesheet),
		http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if asked != "gzip, deflate" {
		t.Errorf("a server in the clear was asked for %q", asked)
	}
}
