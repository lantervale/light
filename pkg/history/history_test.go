package history_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/history"
)

func at(day, hour int) time.Time { return time.Date(2026, time.November, day, hour, 0, 0, 0, time.UTC) }

func addresses(days []history.Day) [][]string {
	var out [][]string
	for _, d := range days {
		var row []string
		for _, v := range d.Visits {
			row = append(row, v.Address)
		}
		out = append(out, row)
	}
	return out
}

func TestVisitsAreListedNewestFirstByDay(t *testing.T) {
	l := history.New(10)
	l.Add("https://a.test", "A", at(15, 9))
	l.Add("https://b.test", "B", at(15, 22))
	l.Add("https://c.test", "C", at(17, 8))
	days := l.Days("")
	if got, want := fmt.Sprint(addresses(days)), "[[https://c.test] [https://b.test https://a.test]]"; got != want {
		t.Fatalf("days %s, want %s", got, want)
	}
	if !days[0].Date.Equal(at(17, 0)) || !days[1].Date.Equal(at(15, 0)) {
		t.Fatalf("the days are %v and %v", days[0].Date, days[1].Date)
	}
}

func TestTheSameAddressTwiceInARowIsOneVisit(t *testing.T) {
	l := history.New(10)
	l.Add("https://a.test", "", at(15, 9))
	l.Add("https://a.test", "A, loaded", at(15, 10))
	l.Add("https://b.test", "B", at(15, 11))
	l.Add("https://a.test", "A", at(15, 12))
	if l.Len() != 3 {
		t.Fatalf("%d visits, want 3", l.Len())
	}
	last := l.Days("")[0].Visits[2]
	if last.Title != "A, loaded" || !last.At.Equal(at(15, 10)) {
		t.Fatalf("the repeated visit is %+v, want its last title and time", last)
	}
}

func TestTheOldestVisitsGoPastTheCap(t *testing.T) {
	l := history.New(2)
	for i := range 5 {
		l.Add(fmt.Sprint("https://", i, ".test"), "", at(15, i))
	}
	if got := fmt.Sprint(addresses(l.Days(""))); got != "[[https://4.test https://3.test]]" {
		t.Fatalf("kept %s", got)
	}
	l.SetLimit(1)
	if got := fmt.Sprint(addresses(l.Days(""))); got != "[[https://4.test]]" {
		t.Fatalf("a smaller cap kept %s", got)
	}
}

func TestAFilterKeepsVisitsHoldingEveryWord(t *testing.T) {
	l := history.New(10)
	l.Add("https://crypt.test/vaults", "The lower vaults", at(15, 9))
	l.Add("https://crypt.test/", "A crypt", at(15, 10))
	l.Add("https://garden.test/", "Zen garden", at(16, 10))
	for filter, want := range map[string]string{
		"crypt":        "[[https://crypt.test/ https://crypt.test/vaults]]",
		"CRYPT  lower": "[[https://crypt.test/vaults]]",
		"zen":          "[[https://garden.test/]]",
		"nothing":      "[]",
	} {
		if got := fmt.Sprint(addresses(l.Days(filter))); got != want {
			t.Errorf("filter %q keeps %s, want %s", filter, got, want)
		}
	}
}

func TestOneVisitIsRemovedAndAllAreCleared(t *testing.T) {
	l := history.New(10)
	l.Add("https://a.test", "A", at(15, 9))
	l.Add("https://b.test", "B", at(15, 10))
	l.Add("https://a.test", "A", at(15, 11))
	l.Remove(l.Days("")[0].Visits[2])
	if got := fmt.Sprint(addresses(l.Days(""))); got != "[[https://a.test https://b.test]]" {
		t.Fatalf("after removing the first visit: %s", got)
	}
	l.Clear()
	if l.Len() != 0 || len(l.Days("")) != 0 {
		t.Fatal("cleared, it still lists visits")
	}
}

func TestALogIsWrittenAndReadBack(t *testing.T) {
	l := history.New(10)
	l.Add("https://a.test", "A “quoted”", at(15, 9))
	l.Add("https://b.test", "", at(16, 10))
	data, err := l.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := history.Decode(data, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(back.Days("")), fmt.Sprint(l.Days("")); got != want {
		t.Fatalf("read back %s, want %s", got, want)
	}
	short, err := history.Decode(data, 1)
	if err != nil || short.Len() != 1 || short.Days("")[0].Visits[0].Address != "https://b.test" {
		t.Fatalf("read with a cap of 1: %v, %v", short.Days(""), err)
	}
}

func TestACorruptLogIsAnEmptyOneAndAnError(t *testing.T) {
	for _, data := range []string{"", "{", `{"visits": 3}`, "\x00\x01"} {
		l, err := history.Decode([]byte(data), 10)
		if err == nil || l == nil || l.Len() != 0 {
			t.Errorf("%q: log %v, err %v", data, l, err)
		}
		l.Add("https://a.test", "A", at(15, 9))
	}
}
