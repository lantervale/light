package history_test

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/history"
)

func TestAVisitKeepsWhatItsPageSaysAndIsReadBackWithIt(t *testing.T) {
	l := history.New(10)
	l.Add("https://a.test", "A", at(15, 9))
	l.Add("https://b.test", "B", at(15, 10))
	l.Add("https://a.test", "A", at(16, 9))
	if _, ok := l.Last("https://c.test"); ok || l.Summarise("https://c.test", "Nothing.") {
		t.Fatal("an address that was not visited was found or summed up")
	}
	if last, ok := l.Last("https://a.test"); !ok || !last.At.Equal(at(16, 9)) {
		t.Fatalf("the last visit is %+v, %v", last, ok)
	}
	if !l.Summarise("https://a.test", " A page about a.\n") {
		t.Fatal("the visit was not summed up")
	}
	visits := l.Days("")
	if last, first := visits[0].Visits[0], visits[1].Visits[1]; last.Summary != "A page about a." || first.Summary != "" {
		t.Fatalf("the last visit says %q, the one before %q", last.Summary, first.Summary)
	}
	l.Add("https://a.test", "A, reloaded", at(16, 10))
	if got := l.Days("")[0].Visits[0]; got.Summary != "A page about a." || got.Title != "A, reloaded" {
		t.Fatalf("reloaded the visit is %+v", got)
	}
	data, err := l.Encode()
	if err != nil || strings.Count(string(data), "summary") != 1 {
		t.Fatalf("written as %s, %v", data, err)
	}
	back, err := history.Decode(data, 10)
	if err != nil || back.Days("")[0].Visits[0].Summary != "A page about a." {
		t.Fatalf("read back as %v, %v", back.Days(""), err)
	}
}
