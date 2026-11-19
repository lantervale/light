package recall_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/recall"
)

func day() time.Time { return time.Date(2026, time.November, 12, 9, 0, 0, 0, time.UTC) }

// axis is a unit vector of three dimensions along one of them.
func axis(i int) []float32 {
	v := make([]float32, 3)
	v[i] = 1
	return v
}

func between(a, b int) []float32 {
	v := make([]float32, 3)
	v[a], v[b] = 0.8, 0.6
	return v
}

func add(t testing.TB, s *recall.Store, address, title string, at time.Time, said map[string][]float32) {
	t.Helper()
	var texts []string
	var vectors [][]float32
	for _, text := range sorted(said) {
		texts, vectors = append(texts, text), append(vectors, said[text])
	}
	if err := s.Add(address, title, at, texts, vectors); err != nil {
		t.Fatal(err)
	}
}

func sorted(said map[string][]float32) []string {
	var out []string
	for text := range said {
		out = append(out, text)
	}
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func ferries(t testing.TB) *recall.Store {
	t.Helper()
	s := recall.New(recall.MaxBytes)
	add(t, s, "https://gazette.example/ferry", "Council votes", day(), map[string][]float32{
		"The council met on Tuesday.":       axis(2),
		"A battery-electric vessel is due.": axis(0),
	})
	add(t, s, "https://www.marine.example/charging", "Shore charging", day().Add(24*time.Hour), map[string][]float32{
		"An arm tops up the batteries.": between(0, 1),
	})
	add(t, s, "https://marine.example/knots", "Knots", day().Add(48*time.Hour), map[string][]float32{
		"A bowline does not slip.": axis(1),
	})
	return s
}

func found(matches []recall.Match) string {
	var out []string
	for _, m := range matches {
		out = append(out, m.Title+": "+m.Passage)
	}
	return strings.Join(out, " | ")
}

func TestASearchGivesEachPageItsBestPassageTheClosestFirst(t *testing.T) {
	s := ferries(t)
	got := s.Search(axis(0), recall.Query{Threshold: 0.5, Most: 8})
	if found(got) != "Council votes: A battery-electric vessel is due. | Shore charging: An arm tops up the batteries." {
		t.Fatalf("the search finds %s", found(got))
	}
	if got[0].Score != 1 || got[0].Address != "https://gazette.example/ferry" || !got[0].At.Equal(day()) {
		t.Fatalf("the first match is %+v", got[0])
	}
	if got = s.Search(axis(0), recall.Query{Threshold: 0.5, Most: 1}); len(got) != 1 {
		t.Fatalf("asked for one match the search gives %d", len(got))
	}
	if got = s.Search(axis(0), recall.Query{Threshold: 0.5, Most: 8, Since: day().Add(time.Hour)}); found(got) !=
		"Shore charging: An arm tops up the batteries." {
		t.Fatalf("since the next day the search finds %s", found(got))
	}
	if got = s.Search([]float32{1, 0}, recall.Query{Most: 8}); len(got) != 0 {
		t.Fatalf("a vector of another model finds %s", found(got))
	}
}

func TestAPageVisitedAgainSaysWhatItSaysNow(t *testing.T) {
	s := ferries(t)
	add(t, s, "https://gazette.example/ferry", "Council voted", day().Add(72*time.Hour), map[string][]float32{
		"The vote was put off.": axis(2),
	})
	if at, kept := s.At("https://gazette.example/ferry"); s.Len() != 3 || !kept || !at.Equal(day().Add(72*time.Hour)) {
		t.Fatalf("the store holds %d pages, that one of %v", s.Len(), at)
	}
	if _, kept := s.At("https://gazette.example/"); kept {
		t.Fatal("a page never visited is kept")
	}
	if got := s.Search(axis(0), recall.Query{Threshold: 0.9, Most: 8}); len(got) != 0 {
		t.Fatalf("what the page said before is still found: %s", found(got))
	}
	got := s.Search(axis(2), recall.Query{Threshold: 0.9, Most: 8})
	if found(got) != "Council voted: The vote was put off." {
		t.Fatalf("the search finds %s", found(got))
	}
}

func TestAPageASiteAndEverythingAreForgotten(t *testing.T) {
	s := ferries(t)
	if !s.Forget("https://gazette.example/ferry") || s.Forget("https://gazette.example/ferry") || s.Len() != 2 {
		t.Fatalf("forgetting a page leaves %d", s.Len())
	}
	if s.Has("https://gazette.example/ferry") || !s.Has("https://marine.example/knots") {
		t.Fatal("the wrong page went")
	}
	if n := s.ForgetSite("marine.example"); n != 2 || s.Len() != 0 {
		t.Fatalf("forgetting a site took %d pages and left %d", n, s.Len())
	}
	s = ferries(t)
	if n := s.Keep(func(address string) bool { return strings.Contains(address, "knots") }); n != 2 || s.Len() != 1 {
		t.Fatalf("keeping one page took %d and left %d", n, s.Len())
	}
	s.Clear()
	if s.Len() != 0 || s.Size() != 0 || len(s.Search(axis(1), recall.Query{Most: 8})) != 0 {
		t.Fatalf("cleared, the store holds %d pages in %d bytes", s.Len(), s.Size())
	}
}

func TestTheSiteOfAnAddressIsItsHostWithoutWWWOrPort(t *testing.T) {
	for address, want := range map[string]string{
		"https://www.Marine.example/a": "marine.example",
		"http://127.0.0.1:8080/x":      "127.0.0.1",
		"light://history":              "",
		"not an address":               "",
	} {
		if got := recall.Site(address); got != want {
			t.Errorf("the site of %s is %q, want %q", address, got, want)
		}
	}
}

func TestTheOldestPagesGoOnceTheStoreIsFull(t *testing.T) {
	s := recall.New(recall.MaxBytes)
	add(t, s, "https://a.example/", "A", day(), map[string][]float32{"a first passage": axis(0)})
	one := s.Size()
	s = recall.New(2*one + one/2)
	for i, name := range []string{"a", "b", "c", "d"} {
		add(t, s, "https://"+name+".example/", strings.ToUpper(name), day().Add(time.Duration(i)*time.Hour),
			map[string][]float32{name + " first passage": axis(0)})
	}
	if s.Len() != 2 || s.Has("https://a.example/") || s.Has("https://b.example/") || s.Size() > 2*one+one/2 {
		t.Fatalf("the store holds %d pages in %d bytes", s.Len(), s.Size())
	}
	add(t, s, "https://c.example/", "C", day().Add(9*time.Hour), map[string][]float32{"c first passage": axis(0)})
	add(t, s, "https://e.example/", "E", day().Add(10*time.Hour), map[string][]float32{"e first passage": axis(0)})
	if !s.Has("https://c.example/") || s.Has("https://d.example/") {
		t.Fatal("a page visited again went before an older one")
	}
	huge := strings.Repeat("word ", 4*one)
	if err := s.Add("https://f.example/", "F", day(), []string{huge}, [][]float32{axis(0)}); err != nil {
		t.Fatal(err)
	}
	if s.Size() > 2*one+one/2 {
		t.Fatalf("a page bigger than the cap is kept: %d bytes", s.Size())
	}
}

func TestAPageKeepsOnlySoManyPassagesOfSoManyBytes(t *testing.T) {
	s := recall.New(recall.MaxBytes)
	var texts []string
	var vectors [][]float32
	for range recall.PassagesPerPage + 5 {
		texts, vectors = append(texts, strings.Repeat("é", recall.PassageSize)), append(vectors, axis(0))
	}
	if err := s.Add("https://a.example/", "A", day(), texts, vectors); err != nil {
		t.Fatal(err)
	}
	if most := 20 + recall.PassagesPerPage*(recall.PassageSize+4+12+4); s.Size() > most+64 {
		t.Fatalf("the page takes %d bytes, over %d", s.Size(), most)
	}
	got := s.Search(axis(0), recall.Query{Most: 1})
	if len(got) != 1 || len(got[0].Passage) > recall.PassageSize || !strings.HasSuffix(got[0].Passage, "é") {
		t.Fatalf("the passage kept is %d bytes", len(got[0].Passage))
	}
}

func TestWhatDoesNotGoTogetherIsRefused(t *testing.T) {
	s := ferries(t)
	for name, add := range map[string]func() error{
		"more texts than vectors": func() error {
			return s.Add("https://a.example/", "A", day(), []string{"one", "two"}, [][]float32{axis(0)})
		},
		"vectors of two lengths": func() error {
			return s.Add("https://a.example/", "A", day(), []string{"one", "two"}, [][]float32{axis(0), {1}})
		},
		"an empty vector": func() error {
			return s.Add("https://a.example/", "A", day(), []string{"one"}, [][]float32{{}})
		},
		"no address": func() error { return s.Add("", "A", day(), []string{"one"}, [][]float32{axis(0)}) },
	} {
		if add() == nil || s.Len() != 3 {
			t.Errorf("%s is kept", name)
		}
	}
	if err := s.Add("https://a.example/", "A", day(), nil, nil); err != nil || s.Has("https://a.example/") {
		t.Fatalf("a page that says nothing is kept, %v", err)
	}
}

func TestVectorsOfAnotherModelReplaceTheOnesKept(t *testing.T) {
	s := ferries(t)
	if err := s.Add("https://a.example/", "A", day(), []string{"one"}, [][]float32{{0, 1}}); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 1 || !s.Has("https://a.example/") {
		t.Fatalf("the store holds %d pages", s.Len())
	}
}

func TestATextIsCutInPassagesAtItsSentencesAndWords(t *testing.T) {
	text := "Home\nThe council voted on Tuesday. The ferry is to be replaced.\n\nIt enters service in 2028.\n" +
		strings.Repeat("long ", 30) + "end."
	got := recall.Passages(text, 60, 10)
	want := []string{
		"Home The council voted on Tuesday.",
		"The ferry is to be replaced. It enters service in 2028.",
		strings.TrimSpace(strings.Repeat("long ", 12)),
		strings.TrimSpace(strings.Repeat("long ", 12)),
		strings.Repeat("long ", 6) + "end.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the passages are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, p := range got {
		if len(p) > 60 {
			t.Errorf("%q is over 60 bytes", p)
		}
	}
	if got = recall.Passages(text, 60, 2); len(got) != 2 {
		t.Fatalf("asked for two passages there are %d", len(got))
	}
	if got = recall.Passages(strings.Repeat("é", 100), 15, 10); len(got) != 10 || got[0] != strings.Repeat("é", 7) {
		t.Fatalf("a word longer than a passage is cut as %q", got)
	}
	if got = recall.Passages(" \n\t ", 60, 10); len(got) != 0 {
		t.Fatalf("no text gives %q", got)
	}
}
