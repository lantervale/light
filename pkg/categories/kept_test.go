package categories_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/categories"
)

const letters = "https://gazette.example/letters"

func TestAJudgmentIsKeptForItsPageNotItsSiteAndForATime(t *testing.T) {
	l := categories.New()
	now := thursday()
	since := now.Add(-24 * time.Hour)
	l.Keep(gazette+"#votes", "politics", "Because it reports a vote\nand a second line", now.Add(-time.Hour))
	got, ok := l.Kept(gazette, since)
	if !ok || got.Category != "Politics" || got.Reason != "Because it reports a vote." ||
		!got.At.Equal(now.Add(-time.Hour)) {
		t.Fatalf("the judgment kept is %+v, %v", got, ok)
	}
	if _, ok = l.Kept(letters, since); ok {
		t.Fatal("a judgment is kept for another page of the site")
	}
	if _, ok = l.Kept(gazette, now); ok {
		t.Fatal("a judgment is kept past its time")
	}
	l.Keep(letters, "Nonsense", "", now)
	for _, address := range []string{"light://home", "file:///etc/hosts", "view-source:" + gazette, ""} {
		l.Keep(address, "Work", "", now)
		if _, ok = l.Kept(address, since); ok {
			t.Errorf("a judgment is kept for %q", address)
		}
	}
	l.Expire(now.Add(-30 * time.Minute))
	if _, ok = l.Kept(gazette, since); ok {
		t.Fatal("a judgment expired stays")
	}
	if _, ok = l.Kept(letters, since); !ok {
		t.Fatal("a judgment not expired went")
	}
	l.ForgetKept()
	if _, ok = l.Kept(letters, since); ok {
		t.Fatal("the judgments forgotten stay")
	}
}

func TestTheJudgmentsKeptGoWhenWhatTheModelIsAskedChanges(t *testing.T) {
	now := thursday()
	since := now.Add(-time.Hour)
	for what, change := range map[string]func(l *categories.List){
		"added":   func(l *categories.List) { l.Add("recipes without a life story first") },
		"removed": func(l *categories.List) { l.Remove("Work") },
		"moved":   func(l *categories.List) { l.Move("Work", -1) },
		"described": func(l *categories.List) {
			work, _ := l.Named("Work")
			work.Description = "my trade"
			l.Edit("Work", work)
		},
	} {
		l := categories.New()
		l.Keep(gazette, "Politics", "", now)
		if change(l); len(kept(l, since)) != 0 {
			t.Errorf("a category %s, the judgments are kept", what)
		}
	}
	l := categories.New()
	l.Keep(gazette, "Politics", "", now)
	politics, _ := l.Named("Politics")
	politics.Action, politics.Tag = categories.Dim, "Pol"
	l.Edit("Politics", politics)
	l.ChooseSite(shop, "Work")
	l.Read("Work", now)
	if len(kept(l, since)) != 1 {
		t.Fatal("what does not change the question dropped the judgments")
	}
}

func TestTheLastPageJudgedOfASiteHintsAtTheCategoryOfItsOthers(t *testing.T) {
	l := categories.New()
	now := thursday()
	since := now.Add(-24 * time.Hour)
	l.Keep(gazette, "Politics", "", now.Add(-time.Hour))
	if c, ok := l.KeptOfSite(letters, since); !ok || c.Name != "Politics" {
		t.Fatalf("the site's last category is %+v, %v", c, ok)
	}
	if _, ok := l.KeptOfSite(letters, now); ok {
		t.Fatal("a judgment past its time hints at its site's category")
	}
	l.Keep(letters, "Nonsense", "", now)
	if c, _ := l.KeptOfSite(gazette, since); c.Name != categories.DefaultName {
		t.Fatalf("after a later page, the site's last category is %+v", c)
	}
	if _, ok := l.KeptOfSite(shop, since); ok {
		t.Fatal("a site no page of was judged has a category")
	}
}

// kept are the addresses of the pages l keeps a judgment of.
func kept(l *categories.List, since time.Time) []string {
	var out []string
	for _, address := range []string{gazette, letters, shop} {
		if _, ok := l.Kept(address, since); ok {
			out = append(out, address)
		}
	}
	return out
}

func TestTheJudgmentsAreKeptInTheFileForTheListTheyWereMadeFor(t *testing.T) {
	now := thursday()
	since := now.Add(-time.Hour)
	l := categories.New()
	l.Keep(gazette, "Politics", "Because it reports a vote.", now)
	l.Keep(shop, "Selling to me", "", now)
	data, err := l.Encode()
	if err != nil {
		t.Fatal(err)
	}
	again, err := categories.Decode(data)
	if got, ok := again.Kept(gazette, since); err != nil || !ok || got.Category != "Politics" ||
		got.Reason != "Because it reports a vote." || !got.At.Equal(now) || len(kept(again, since)) != 2 {
		t.Fatalf("read again, the judgment is %+v, %v, %v", got, ok, err)
	}
	other, err := categories.Decode([]byte(strings.Replace(string(data), `"name": "Work"`, `"name": "Trade"`, 1)))
	if err != nil || len(kept(other, since)) != 0 {
		t.Fatalf("a file whose categories are not those judged for keeps %v, %v", kept(other, since), err)
	}
	old, err := categories.Decode([]byte(`{"judged": [{"address": "` + gazette + `", "category": "Politics"}]}`))
	if err != nil || len(kept(old, time.Time{})) != 0 {
		t.Fatalf("judgments said for no list are kept: %v", err)
	}
}
