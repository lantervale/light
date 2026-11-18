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
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			_, _ = io.WriteString(w, text)
		}
	}))
	t.Cleanup(srv.Close)
	l := netlog.New(10)
	for _, path := range []string{"/zlib", "/bare", "/br"} {
		req, err := http.NewRequestWithContext(netlog.With(t.Context(), l, "document", netlog.Stylesheet),
			http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := (&http.Client{Transport: &netlog.Transport{}}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		left := resp.Header.Get("Content-Encoding")
		if err != nil || string(got) != text || (left != "") != (path == "/br") {
			t.Errorf("%s is read as %d bytes (%v), still encoded %q", path, len(got), err, left)
		}
	}
	for _, encodings := range asked {
		if encodings != "gzip, deflate" {
			t.Errorf("the server was asked for %q", encodings)
		}
	}
	for _, row := range l.Rows() {
		wire := row.Transferred < row.Size/4
		if row.Unzipped != !strings.HasSuffix(row.Address, "/br") || row.Unzipped != wire {
			t.Errorf("%s: undone %t, %d bytes read of %d sent", row.Address, row.Unzipped, row.Size, row.Transferred)
		}
	}
}
