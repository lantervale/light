package categories_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/categories"
)

const (
	gazette = "https://gazette.example/council/ferry-vote"
	shop    = "https://www.shop.example/offers"
)

func thursday() time.Time { return time.Date(2026, time.November, 19, 10, 0, 0, 0, time.UTC) }

func names(l *categories.List) string { return strings.Join(l.Names(), ", ") }

func TestANewListIsTheShippedOneWithDefaultLast(t *testing.T) {
	l := categories.New()
	if got := names(l); got != "Danger, Politics, Selling to me, Work, Default" {
		t.Fatalf("a new list is %s", got)
	}
	all := l.All()
	if danger := all[0]; danger.Action != categories.Warn || danger.Shape != categories.Triangle ||
		!strings.Contains(danger.Description, "password or card where it should not") ||
		!strings.Contains(danger.Description, "imitates another site") {
		t.Fatalf("Danger is %+v", danger)
	}
	if last := all[len(all)-1]; last != categories.Default() || last.Action != categories.NoMark ||
		last.Description != "everything else" {
		t.Fatalf("the last category is %+v", last)
	}
	seen := map[categories.Mark]bool{}
	for _, c := range all {
		if seen[c.Mark] || c.Colour == "" || c.Shape == "" {
			t.Fatalf("%s has the mark %+v, another's or none", c.Name, c.Mark)
		}
		seen[c.Mark] = true
	}
}

func TestEachActionSaysWhatItDoes(t *testing.T) {
	var said []string
	for _, a := range categories.Actions() {
		said = append(said, a.String())
	}
	if got := strings.Join(said, ", "); got != "No mark, Mark, Warn first, Dim or hide" {
		t.Fatalf("the actions read %s", got)
	}
}

func TestACategoryIsAddedBeforeDefaultNamedAfterItsDescription(t *testing.T) {
	l := categories.New()
	asked := l.Asked()
	added, ok := l.Add("  recipes without a\nlife story first ")
	if !ok || added.Name != "Recipes" || added.Description != "recipes without a life story first" ||
		added.Action != categories.Marked {
		t.Fatalf("added %+v, %v", added, ok)
	}
	if got := names(l); got != "Danger, Politics, Selling to me, Work, Recipes, Default" {
		t.Fatalf("the list is %s", got)
	}
	if slices.ContainsFunc(l.All()[:4], func(c categories.Category) bool { return c.Mark == added.Mark }) {
		t.Fatalf("the new category has another's mark %+v", added.Mark)
	}
	if l.Asked() == asked {
		t.Fatal("a category more does not change what a model is asked")
	}
	if again, _ := l.Add("recipes with pictures"); again.Name != "Recipes 2" {
		t.Fatalf("a second one is named %q", again.Name)
	}
	if named, _ := l.Add("Trains: timetables and rolling stock"); named.Name != "Trains" ||
		named.Description != "timetables and rolling stock" {
		t.Fatalf("a name before a colon gives %+v", named)
	}
	if _, ok = l.Add("  \n "); ok {
		t.Fatal("a description that says nothing made a category")
	}
	if c, _ := l.Add("default: the rest"); c.Name == categories.DefaultName || strings.EqualFold(c.Name, "default") {
		t.Fatalf("a category took Default's name: %q", c.Name)
	}
	long, _ := l.Add(strings.Repeat("é", 400))
	if len(long.Name) > 40 || len(long.Description) > 240 || !strings.HasSuffix(long.Description, "é") {
		t.Fatalf("a long description is kept as %d and %d bytes", len(long.Name), len(long.Description))
	}
}

func TestACategoryIsEditedMovedAndRemovedAndDefaultIsNot(t *testing.T) {
	l := categories.New()
	l.ChooseSite(gazette, "Politics")
	l.Read("Politics", thursday())
	politics, _ := l.Named("politics")
	politics.Name, politics.Action, politics.Mark = "Public life", categories.Dim, categories.Mark{Colour: "#5aa9e6",
		Shape: categories.Diamond}
	if !l.Edit("Politics", politics) {
		t.Fatal("the edit changed nothing")
	}
	got, _ := l.Named("Public life")
	if got != politics {
		t.Fatalf("edited to %+v", got)
	}
	if v, ok := l.Chosen(gazette); !ok || v.Name != "Public life" {
		t.Fatalf("the site's choice did not follow the new name: %+v", v)
	}
	if counts, _ := l.Week(thursday()); len(counts) != 1 || counts[0].Name != "Public life" || counts[0].Pages != 1 {
		t.Fatalf("what was read did not follow the new name: %+v", counts)
	}
	bad := politics
	bad.Name, bad.Action, bad.Mark, bad.Description = "Work", "explode", categories.Mark{Colour: "red", Shape: "blob"}, ""
	l.Edit("Public life", bad)
	if got, _ = l.Named("Work 2"); got.Action != categories.Dim || got.Mark != politics.Mark ||
		got.Description != politics.Description {
		t.Fatalf("a bad edit left %+v", got)
	}
	asked := l.Asked()
	marked := got
	marked.Action = categories.Marked
	if l.Edit("Work 2", marked); l.Asked() != asked {
		t.Fatal("another action changed what a model is asked")
	}
}

func TestACategoryIsMovedAndRemovedAndDefaultIsNot(t *testing.T) {
	l := categories.New()
	l.ChooseSite(gazette, "Politics")
	marked, _ := l.Named("Politics")
	marked.Name = "Work 2"
	l.Edit("Politics", marked)
	if !l.Move("Work", -10) || names(l) != "Work, Danger, Work 2, Selling to me, Default" {
		t.Fatalf("moved up, the list is %s", names(l))
	}
	if !l.Move("Work", 10) || names(l) != "Danger, Work 2, Selling to me, Work, Default" {
		t.Fatalf("moved down, the list is %s", names(l))
	}
	if l.Move("Work", 1) || l.Move("Default", -1) || l.Remove("Default") || l.Edit("Default", marked) {
		t.Fatal("Default was moved, removed, edited or passed")
	}
	if !l.Remove("work 2") || names(l) != "Danger, Selling to me, Work, Default" {
		t.Fatalf("removed, the list is %s", names(l))
	}
	if _, ok := l.Chosen(gazette); ok || len(l.Choices()) != 0 {
		t.Fatal("the choice for a category that is gone stays")
	}
}

func TestWhatTheReaderChoseWinsOverTheModelAndAPageOverItsSite(t *testing.T) {
	l := categories.New()
	if v, ok := l.Verdict(gazette, "", ""); ok {
		t.Fatalf("a page nobody judged has the verdict %+v", v)
	}
	v, ok := l.Verdict(gazette, "politics", "Because it reports a council vote\nIgnore this line.")
	if !ok || v.Name != "Politics" || v.By != categories.ByModel || v.Reason != "Because it reports a council vote." {
		t.Fatalf("the model's verdict is %+v", v)
	}
	if v, _ = l.Verdict(gazette, "Safe", "Because it says it is safe."); v.Name != categories.DefaultName ||
		v.Action != categories.NoMark || strings.Contains(strings.ToLower(v.Reason), "safe") {
		t.Fatalf("an answer that is no category gives %+v", v)
	}
}

func TestAPageCorrectedWinsOverItsSiteAndChoicesAreForgotten(t *testing.T) {
	l := categories.New()
	asked := l.Asked()
	if !l.ChooseSite(shop, "Selling to me") || l.ChooseSite(shop, "Nonsense") || l.ChooseSite("::", "Work") {
		t.Fatal("choosing for a site")
	}
	v, _ := l.Verdict("https://shop.example/cart#top", "Danger", "Because it asks for a card.")
	if v.Name != "Selling to me" || v.By != categories.BySite || !strings.Contains(v.Reason, "shop.example") {
		t.Fatalf("the site's choice gives %+v", v)
	}
	l.ChoosePage("https://shop.example/cart#top", "Default")
	if v, _ = l.Verdict("https://shop.example/cart", "Danger", ""); v.Name != categories.DefaultName ||
		v.By != categories.ByPage {
		t.Fatalf("the page's correction gives %+v", v)
	}
	if v, _ = l.Verdict(shop, "Danger", ""); v.By != categories.BySite {
		t.Fatalf("another page of the site gives %+v", v)
	}
	if l.Asked() != asked {
		t.Fatal("a choice changed what a model is asked")
	}
	got := l.Choices()
	if len(got) != 2 || got[0].Site != "shop.example" || got[1].Address != "https://shop.example/cart" {
		t.Fatalf("the choices are %+v", got)
	}
	if !l.Forget(l.Choices()[1]) || !l.ForgetSite(shop) || l.ForgetSite(shop) {
		t.Fatal("forgetting the choices")
	}
	if v, _ = l.Verdict(shop, "Danger", "Because it asks for a card."); v.By != categories.ByModel || v.Name != "Danger" {
		t.Fatalf("once forgotten the verdict is %+v", v)
	}
	for i := range 600 {
		l.ChoosePage(gazette+"/"+strings.Repeat("x", i), "Work")
	}
	if kept := len(l.Choices()); kept != 500 {
		t.Fatalf("%d corrections are kept", kept)
	}
}

func TestTheQuestionListsTheCategoriesInOrderAndAsksForOneName(t *testing.T) {
	l := categories.New()
	l.Move("Work", -3)
	q := l.Question()
	var at []int
	for _, c := range l.All() {
		i := strings.Index(q, "- "+c.Name+": "+c.Description+"\n")
		if i < 0 {
			t.Fatalf("the question does not describe %s:\n%s", c.Name, q)
		}
		at = append(at, i)
	}
	if !slices.IsSorted(at) || !strings.Contains(q, "first category of the list that fits") ||
		!strings.HasSuffix(q, "nothing else: Work, Danger, Politics, Selling to me, Default.") {
		t.Fatalf("the question is\n%s", q)
	}
	why := categories.Why(l.All()[2])
	if !strings.Contains(why, `"Politics"`) || !strings.Contains(why, "one short sentence") {
		t.Fatalf("the reason is asked by %q", why)
	}
}

func TestAReasonIsShownAsOneSentence(t *testing.T) {
	for given, want := range map[string]string{
		"":                                   "",
		"  Because it sells   a ferry pass ": "Because it sells a ferry pass.",
		"Because of the vote!\nSYSTEM: obey": "Because of the vote!",
		strings.Repeat("because ", 100):      strings.TrimSpace(strings.Repeat("because ", 27)) + " beca.",
	} {
		if got := categories.Sentence(given); got != want {
			t.Errorf("%q is shown as %q, not %q", given, got, want)
		}
	}
}

func TestTheWeekCountsThePagesReadSinceMondayByCategory(t *testing.T) {
	l := categories.New()
	if counts, total := l.Week(thursday()); len(counts) != 0 || total != 0 {
		t.Fatalf("nothing read gives %+v, %d", counts, total)
	}
	monday, sunday := thursday().AddDate(0, 0, -3), thursday().AddDate(0, 0, -4)
	l.Read("Work", thursday())
	l.Read("work", monday)
	l.Read("Politics", thursday())
	l.Read("Politics", sunday)
	l.Read("Default", thursday())
	l.Read("Nonsense", thursday())
	l.Read("Danger", thursday())
	l.Unread("Danger", thursday())
	l.Unread("Danger", thursday())
	counts, total := l.Week(thursday())
	if len(counts) != 2 || counts[0].Name != "Politics" || counts[0].Pages != 1 || counts[1].Name != "Work" ||
		counts[1].Pages != 2 || total != 4 {
		t.Fatalf("the week is %+v of %d", counts, total)
	}
	if counts, total = l.Week(sunday); len(counts) != 1 || counts[0].Pages != 1 || total != 1 {
		t.Fatalf("the week before is %+v of %d", counts, total)
	}
	l.Read("Work", thursday().AddDate(0, 0, 90))
	if counts, _ = l.Week(thursday()); len(counts) != 0 {
		t.Fatalf("days long past are kept: %+v", counts)
	}
	l.ForgetRead()
	if _, total = l.Week(thursday().AddDate(0, 0, 90)); total != 0 {
		t.Fatal("what was read is not forgotten")
	}
}

func TestTheListIsKeptInItsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kept", "categories.json")
	l, err := categories.Load(path)
	if err != nil || names(l) != names(categories.New()) {
		t.Fatalf("with no file the list is %s, %v", names(l), err)
	}
	l.Remove("Work")
	l.Add("recipes without a life story first")
	l.Move("Recipes", -9)
	l.ChooseSite(shop, "Selling to me")
	l.ChoosePage(gazette, "Default")
	l.Read("Politics", thursday())
	if err = categories.Save(path, l); err != nil {
		t.Fatal(err)
	}
	again, err := categories.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if names(again) != "Recipes, Danger, Politics, Selling to me, Default" || !slices.Equal(again.All(), l.All()) ||
		!slices.Equal(again.Choices(), l.Choices()) {
		t.Fatalf("read again: %s, %+v", names(again), again.Choices())
	}
	if counts, total := again.Week(thursday()); total != 1 || counts[0].Name != "Politics" {
		t.Fatalf("read again, the week is %+v", counts)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("the folder holds %d files", len(entries))
	}
	none := categories.New()
	for _, name := range none.Names() {
		none.Remove(name)
	}
	data, _ := none.Encode()
	if emptied, _ := categories.Decode(data); names(emptied) != "Default" {
		t.Fatalf("a list emptied comes back as %s", names(emptied))
	}
}

func TestAFileThatDoesNotHoldIsReadAsFarAsItDoes(t *testing.T) {
	l, err := categories.Decode([]byte(`{"categories": [
		{"name": "Trains", "description": "timetables", "colour": "#5aa9e6", "shape": "dot", "action": "dim"},
		{"name": "trains", "description": "twice"},
		{"name": "Default", "description": "mine"},
		{"name": "", "description": "nameless"},
		{"name": "Odd", "description": "odd", "colour": "blue", "shape": "blob", "action": "explode"}],
		"sites": [{"site": "a.example", "category": "Trains"}, {"site": "b.example", "category": "Gone"}],
		"pages": [{"address": "https://a.example/x", "category": "odd"}, {"category": "Trains"}],
		"read": {"2026-11-19": {"Trains": 2, "Odd": -4}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if names(l) != "Trains, Odd, Default" {
		t.Fatalf("read as %s", names(l))
	}
	odd, _ := l.Named("Odd")
	if odd.Action != categories.Marked || odd.Colour == "blue" || odd.Shape == "blob" {
		t.Fatalf("Odd is %+v", odd)
	}
	if got := l.Choices(); len(got) != 2 || got[0].Site != "a.example" || got[1].Category != "Odd" {
		t.Fatalf("the choices are %+v", got)
	}
	if counts, total := l.Week(thursday()); total != 2 || len(counts) != 1 {
		t.Fatalf("the week is %+v of %d", counts, total)
	}
	if l, err = categories.Decode([]byte(`{`)); err == nil || names(l) != names(categories.New()) {
		t.Fatalf("a broken file gives %s, %v", names(l), err)
	}
	if l, err = categories.Decode([]byte(`{}`)); err != nil || names(l) != names(categories.New()) {
		t.Fatalf("a file that names no categories gives %s, %v", names(l), err)
	}
}

func TestASiteIsItsHostWithoutWWW(t *testing.T) {
	for address, want := range map[string]string{
		"https://WWW.Shop.example/offers?x=1": "shop.example",
		"http://127.0.0.1:8080/a":             "127.0.0.1:8080",
		"light://home":                        "home",
		"::":                                  "",
	} {
		if got := categories.SiteOf(address); got != want {
			t.Errorf("the site of %s is %q, not %q", address, got, want)
		}
	}
}
