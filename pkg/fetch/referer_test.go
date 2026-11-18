package fetch_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lantervale/light/pkg/fetch"
)

func TestARequestSaysWhereItComesFromWhenItsContextDoes(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Referer"))
	}))
	t.Cleanup(srv.Close)
	c := fetch.New(nil)
	if _, err := c.Get(fetch.Referring(t.Context(), "https://site.test/css/site.css"), srv.URL, "*/*"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(t.Context(), srv.URL, "*/*"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(fetch.Referring(t.Context(), ""), srv.URL, "*/*"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "https://site.test/css/site.css" || got[1] != "" || got[2] != "" {
		t.Errorf("Referer headers: %q", got)
	}
}
