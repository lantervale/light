package complete_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/complete"
)

func at(day int) time.Time { return time.Date(2026, 3, day, 9, 0, 0, 0, time.UTC) }

func addresses(pages []complete.Page) string {
	var all []string
	for _, p := range pages {
		all = append(all, p.Address)
	}
	return strings.Join(all, " ")
}

func TestPagesMatchEveryWordInTheirTitleOrAddressWhateverItsCase(t *testing.T) {
	pages := []complete.Page{
		{Address: "https://ferry.test/times", Title: "Ferry times", At: at(1)},
		{Address: "https://harbour.test/", Title: "The harbour and its ferry", At: at(2)},
		{Address: "https://crypt.test/vaults", Title: "Vaults", At: at(3)},
	}
	for typed, want := range map[string]string{
		"FERRY":        "https://harbour.test/ https://ferry.test/times",
		"ferry times":  "https://ferry.test/times",
		"crypt.test/v": "https://crypt.test/vaults",
		"har fer":      "https://harbour.test/",
		"nothing":      "",
		"  ":           "",
	} {
		if got := addresses(complete.Match(typed, pages, 5)); got != want {
			t.Errorf("%q offers %q, want %q", typed, got, want)
		}
	}
}

func TestAnAddressIsOfferedOnceTheMostVisitedFirstThenTheMostRecent(t *testing.T) {
	pages := []complete.Page{
		{Address: "https://a.test/", Title: "", At: at(1)},
		{Address: "https://b.test/", Title: "B once", At: at(2)},
		{Address: "https://a.test/", Title: "A twice", At: at(3)},
		{Address: "https://c.test/", Title: "C once, later", At: at(4)},
		{Address: "https://a.test/", Title: "", At: at(5)},
	}
	got := complete.Match("test", pages, 5)
	if addresses(got) != "https://a.test/ https://c.test/ https://b.test/" {
		t.Fatalf("the pages are offered as %q", addresses(got))
	}
	if got[0].Title != "A twice" || !got[0].At.Equal(at(5)) {
		t.Fatalf("the page visited three times is offered as %+v", got[0])
	}
	if got := addresses(complete.Match("test", pages, 2)); got != "https://a.test/ https://c.test/" {
		t.Fatalf("two rows offer %q", got)
	}
}

func TestAKeptPageKeepsItsNameAndIsOfferedWithoutAVisit(t *testing.T) {
	pages := []complete.Page{
		{Address: "https://a.test/", Title: "Visited", At: at(4)},
		{Address: "https://a.test/", Title: "My name for it", At: at(1), Kept: true},
		{Address: "https://b.test/", Title: "Kept, never visited", At: at(2), Kept: true},
	}
	got := complete.Match("test", pages, 5)
	if addresses(got) != "https://a.test/ https://b.test/" || got[0].Title != "My name for it" || !got[0].Kept ||
		!got[1].Kept {
		t.Fatalf("the pages are offered as %+v", got)
	}
	if found := addresses(complete.Match("visited", pages, 5)); found != "https://a.test/ https://b.test/" {
		t.Fatalf("a word of a visit's title and of a bookmark's name offers %q", found)
	}
}
