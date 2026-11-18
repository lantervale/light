package downloads_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/downloads"
	"github.com/lantervale/light/pkg/fetch"
)

// gate is a server whose /slow answers its first half, then waits to be
// let through, or for its client to leave.
type gate struct {
	*httptest.Server
	through chan struct{}
	left    chan struct{}
	once    sync.Once
}

func (g *gate) open() { g.once.Do(func() { close(g.through) }) }

func site(t *testing.T) *gate {
	t.Helper()
	g := &gate{through: make(chan struct{}), left: make(chan struct{}, 8)}
	mux := http.NewServeMux()
	mux.HandleFunc("/paper.pdf", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF a paper"))
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="../q3 figures.csv"`)
		_, _ = w.Write([]byte("a,b\n"))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/paper.pdf", http.StatusFound)
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", "2000")
		_, _ = w.Write(make([]byte, 1000))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-g.through:
			_, _ = w.Write(make([]byte, 1000))
		case <-r.Context().Done():
			g.left <- struct{}{}
		}
	})
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	t.Cleanup(g.open)
	return g
}

func manager(t *testing.T) (*downloads.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	m := downloads.New(dir, fetch.New(nil))
	t.Cleanup(m.Close)
	return m, dir
}

// ended waits for download id to leave the active state.
func ended(t *testing.T, m *downloads.Manager, id int) downloads.Item {
	t.Helper()
	return until(t, m, id, func(i downloads.Item) bool { return i.State != downloads.Active })
}

func until(t *testing.T, m *downloads.Manager, id int, held func(downloads.Item) bool) downloads.Item {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if item, ok := m.Item(id); ok && held(item) {
			return item
		}
		time.Sleep(2 * time.Millisecond)
	}
	item, _ := m.Item(id)
	t.Fatalf("download %d stays %+v", id, item)
	return item
}

func files(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return strings.Join(names, ", ")
}

func TestADownloadIsWrittenUnderItsNameAndSaysWhatItIs(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	item := ended(t, m, m.Start(t.Context(), srv.URL+"/moved", ""))
	want := filepath.Join(dir, "paper.pdf")
	if item.State != downloads.Done || item.Path != want || item.Name != "paper.pdf" || item.Received != 12 ||
		item.Total != 12 || item.Media != "application/pdf" || item.Address != srv.URL+"/moved" {
		t.Fatalf("the download ended as %+v", item)
	}
	if data, err := os.ReadFile(want); err != nil || string(data) != "%PDF a paper" {
		t.Fatalf("the file holds %q: %v", data, err)
	}
	if got := files(t, dir); got != "paper.pdf" {
		t.Fatalf("the folder holds %s", got)
	}
}

func TestADownloadNeverWritesOverAFileNorOutOfItsFolder(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	if err := os.WriteFile(filepath.Join(dir, "paper.pdf"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := ended(t, m, m.Start(t.Context(), srv.URL+"/paper.pdf", ""))
	second := ended(t, m, m.Start(t.Context(), srv.URL+"/paper.pdf", ""))
	named := ended(t, m, m.Start(t.Context(), srv.URL+"/paper.pdf", "../../notes"))
	report := ended(t, m, m.Start(t.Context(), srv.URL+"/report", ""))
	got := fmt.Sprint(first.Name, ", ", second.Name, ", ", named.Name, ", ", report.Name)
	if got != "paper (1).pdf, paper (2).pdf, notes.pdf, q3 figures.csv" {
		t.Fatalf("saved as %s", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "paper.pdf")); string(data) != "mine" {
		t.Fatalf("the file that was there holds %q", data)
	}
	if filepath.Dir(report.Path) != dir {
		t.Fatalf("the report went to %s", report.Path)
	}
}

func TestADownloadInProgressCountsItsBytesInATemporaryFile(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	id := m.Start(t.Context(), srv.URL+"/slow", "")
	item := until(t, m, id, func(i downloads.Item) bool { return i.Received == 1000 })
	if item.State != downloads.Active || item.Total != 2000 || m.Active() != 1 {
		t.Fatalf("half way it is %+v, %d active", item, m.Active())
	}
	if part, ok := item.Fraction(); !ok || part != 0.5 {
		t.Fatalf("half way its fraction is %v", part)
	}
	if got := files(t, dir); got != "slow.zip.lightdownload" {
		t.Fatalf("half way the folder holds %s", got)
	}
	srv.open()
	if item = ended(t, m, id); item.State != downloads.Done || item.Received != 2000 || m.Active() != 0 {
		t.Fatalf("it ended as %+v", item)
	}
	if got := files(t, dir); got != "slow.zip" {
		t.Fatalf("at the end the folder holds %s", got)
	}
}

func TestACancelledDownloadLeavesNoFileAndIsRetried(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	id := m.Start(t.Context(), srv.URL+"/slow", "")
	until(t, m, id, func(i downloads.Item) bool { return i.Received == 1000 })
	version := m.Version()
	m.Cancel(id)
	if item := ended(t, m, id); item.State != downloads.Cancelled || m.Version() == version {
		t.Fatalf("cancelled, it is %+v", item)
	}
	<-srv.left
	if got := files(t, dir); got != "" {
		t.Fatalf("after a cancel the folder holds %s", got)
	}
	srv.open()
	m.Retry(t.Context(), id)
	if item := ended(t, m, id); item.State != downloads.Done || item.Name != "slow.zip" || len(m.Items()) != 1 {
		t.Fatalf("retried, it is %+v among %d", item, len(m.Items()))
	}
}

func TestAFailedDownloadSaysWhy(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	for address, why := range map[string]string{
		srv.URL + "/gone":       "the server answered 404 Not Found",
		"http://127.0.0.1:1/x":  "refused",
		"ftp://files.test/file": "unsupported protocol",
	} {
		if item := ended(t, m, m.Start(t.Context(), address, "")); item.State != downloads.Failed ||
			!strings.Contains(item.Err, why) {
			t.Errorf("%s ended as %s, %q", address, item.State, item.Err)
		}
	}
	nowhere := downloads.New("", fetch.New(nil))
	defer nowhere.Close()
	if item := ended(t, nowhere, nowhere.Start(t.Context(), srv.URL+"/paper.pdf", "")); item.State != downloads.Failed ||
		!strings.Contains(item.Err, "no folder") {
		t.Errorf("with no folder it ended as %s, %q", item.State, item.Err)
	}
	if got := files(t, dir); got != "" {
		t.Fatalf("failures left %s", got)
	}
}

func TestClosingCancelsWhatIsInFlightAndRemovesItsFile(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	ctx, cancel := context.WithCancel(t.Context())
	a, b := m.Start(ctx, srv.URL+"/slow", ""), m.Start(ctx, srv.URL+"/slow", "")
	until(t, m, a, func(i downloads.Item) bool { return i.Received == 1000 })
	until(t, m, b, func(i downloads.Item) bool { return i.Received == 1000 })
	cancel()
	if item, _ := m.Item(a); item.State != downloads.Active {
		t.Fatalf("the end of the context it was started from ends a download: %+v", item)
	}
	m.Close()
	for _, id := range []int{a, b} {
		if item, _ := m.Item(id); item.State != downloads.Cancelled {
			t.Fatalf("after Close download %d is %+v", id, item)
		}
	}
	if got := files(t, dir); got != "" {
		t.Fatalf("after Close the folder holds %s", got)
	}
	if id := m.Start(t.Context(), srv.URL+"/paper.pdf", ""); id != 0 {
		t.Fatalf("a closed manager started download %d", id)
	}
}

func TestBytesHeldAreSavedAsADownloadIs(t *testing.T) {
	m, dir := manager(t)
	id := m.Write("https://crypt.test/", "crypt.html", "text/html", []byte("<p>wet</p>"))
	item := ended(t, m, id)
	if item.State != downloads.Done || item.Name != "crypt.html" || item.Received != 10 {
		t.Fatalf("saved as %+v", item)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "crypt.html")); string(data) != "<p>wet</p>" {
		t.Fatalf("the file holds %q", data)
	}
}

func TestItemsAreNewestFirstAndRemovedOrCleared(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	a := m.Start(t.Context(), srv.URL+"/paper.pdf", "")
	ended(t, m, a)
	b := m.Start(t.Context(), srv.URL+"/slow", "")
	c := m.Start(t.Context(), srv.URL+"/report", "")
	ended(t, m, c)
	if items := m.Items(); len(items) != 3 || items[0].ID != c || items[2].ID != a {
		t.Fatalf("the items are %+v", items)
	}
	m.Remove(a)
	m.Remove(b)
	if items := m.Items(); len(items) != 2 || items[1].ID != b {
		t.Fatalf("removing took an active download, or left a done one: %+v", items)
	}
	m.Clear()
	if items := m.Items(); len(items) != 1 || items[0].ID != b {
		t.Fatalf("clearing left %+v", items)
	}
	if got := files(t, dir); !strings.Contains(got, "paper.pdf") || !strings.Contains(got, "q3 figures.csv") {
		t.Fatalf("removing from the list removed files: %s", got)
	}
}

func TestSpeedAndTimeLeftComeFromWhatWasReceived(t *testing.T) {
	item := downloads.Item{State: downloads.Active, Received: 1000, Total: 5000, Speed: 2000}
	if left, ok := item.Remaining(); !ok || left != 2*time.Second {
		t.Fatalf("4000 bytes at 2000 a second leave %v", left)
	}
	for name, unknown := range map[string]downloads.Item{
		"no total": {State: downloads.Active, Received: 10, Total: -1, Speed: 5},
		"no speed": {State: downloads.Active, Received: 10, Total: 50},
		"ended":    {State: downloads.Done, Received: 50, Total: 50, Speed: 5},
	} {
		if left, ok := unknown.Remaining(); ok {
			t.Errorf("%s: %v left", name, left)
		}
	}
	if _, ok := (downloads.Item{Received: 10, Total: -1}).Fraction(); ok {
		t.Error("a download of unknown size has a fraction")
	}
}

func TestADoneDownloadWhoseFileIsGoneIsMissing(t *testing.T) {
	gone := func(string) bool { return false }
	if !(downloads.Item{State: downloads.Done, Path: "/x"}).Missing(gone) ||
		(downloads.Item{State: downloads.Failed}).Missing(gone) {
		t.Error("Missing misreads an item")
	}
}

func TestTheListIsKeptInAFileCappedAndTolerant(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	for range 3 {
		ended(t, m, m.Start(t.Context(), srv.URL+"/paper.pdf", ""))
	}
	active := m.Start(t.Context(), srv.URL+"/slow", "")
	until(t, m, active, func(i downloads.Item) bool { return i.Received == 1000 })
	path := filepath.Join(dir, "state", "downloads.json")
	if err := downloads.Save(path, m.Items()); err != nil {
		t.Fatal(err)
	}
	items, err := downloads.Load(path, 3)
	if err != nil || len(items) != 3 || items[0].State != downloads.Failed || items[0].Err == "" ||
		items[1].Name != "paper (2).pdf" || items[1].State != downloads.Done {
		t.Fatalf("loaded %+v, %v", items, err)
	}
	again := downloads.New(dir, fetch.New(nil))
	defer again.Close()
	again.Restore(items)
	if id := again.Start(t.Context(), srv.URL+"/gone", ""); id <= items[0].ID {
		t.Fatalf("a new download takes the number %d of an old one", id)
	}
	none, err := downloads.Load(filepath.Join(dir, "none.json"), 3)
	if err != nil || len(none) != 0 {
		t.Fatalf("a missing file gives %v, %v", none, err)
	}
	bad := filepath.Join(dir, "bad.json")
	if err = os.WriteFile(bad, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken, err := downloads.Load(bad, 3)
	if err == nil || len(broken) != 0 {
		t.Fatalf("a broken file gives %v, %v", broken, err)
	}
}

func TestTheFolderIsChangedForTheDownloadsThatFollow(t *testing.T) {
	srv := site(t)
	m, dir := manager(t)
	ended(t, m, m.Start(t.Context(), srv.URL+"/moved", ""))
	other := t.TempDir()
	m.SetDir(other)
	item := ended(t, m, m.Start(t.Context(), srv.URL+"/moved", ""))
	if item.Path != filepath.Join(other, "paper.pdf") || files(t, dir) != "paper.pdf" || files(t, other) != "paper.pdf" {
		t.Fatalf("after the change it was saved at %s", item.Path)
	}
}
