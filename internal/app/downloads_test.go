package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lantervale/ember/input"
)

// slowSite serves /big.zip in two halves, the second once let through or
// never when its client leaves.
type slowSite struct {
	*httptest.Server
	through chan struct{}
	once    sync.Once
}

func (g *slowSite) open() { g.once.Do(func() { close(g.through) }) }

func slow(t *testing.T) *slowSite {
	t.Helper()
	g := &slowSite{through: make(chan struct{})}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/big.zip" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", "4000")
		_, _ = w.Write(make([]byte, 1000))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-g.through:
			_, _ = w.Write(make([]byte, 3000))
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(g.Close)
	t.Cleanup(g.open)
	return g
}

// shell notes what light asks the system to do with files, and does
// nothing.
type shell struct{ asked []string }

func (f *shell) Open(path string) error {
	f.asked = append(f.asked, "open "+filepath.Base(path))
	return nil
}

func (f *shell) Reveal(path string) error {
	f.asked = append(f.asked, "reveal "+filepath.Base(path))
	return nil
}

// settled waits until no download is in progress and the screen has seen
// it.
func (s *shown) settled() {
	s.t.Helper()
	s.until("the downloads ended", func() bool { return s.saves.Active() == 0 && s.savesSeen == s.saves.Version() })
}

// saved waits for the file name in the downloads folder, a navigation
// having led to it, and for the downloads to end.
func (s *shown) saved(name string) {
	s.t.Helper()
	s.until(name+" was downloaded", func() bool { return s.downloaded(name) != "" })
	s.settled()
}

// quarter waits until the newest download received the first quarter
// slowSite sends, and the screen has seen it.
func (s *shown) quarter() {
	s.t.Helper()
	s.until("a quarter came", func() bool {
		items := s.saves.Items()
		return len(items) > 0 && items[0].Received == 1000 && s.savesSeen == s.saves.Version()
	})
}

// act presses the button of class action in the row of download id on
// the downloads page.
func (s *shown) act(id int, action string) {
	s.t.Helper()
	for _, b := range s.node("download-" + strconv.Itoa(id)).Children()[2].Children() {
		if b.HasClass(action) {
			r, _ := s.doc.BoxOf(b)
			s.clickAt(r.Center())
			return
		}
	}
	s.t.Fatalf("download %d has no %s button", id, action)
}

func TestAFileLightDoesNotShowIsDownloadedAtOnceAndTheTabStays(t *testing.T) {
	srv, seen := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/vaults")
	s.enter(srv.URL + "/paper.pdf")
	s.saved("paper.pdf")
	s.at(srv.URL + "/vaults")
	if got := s.downloaded("paper.pdf"); got != "%PDF-1.7 a paper" {
		t.Fatalf("the file holds %q", got)
	}
	if got := s.status.Text(); got != "Downloaded paper.pdf" || s.host == nil || !s.disabled("forward") {
		t.Fatalf("the bubble says %q, the page shown %v", got, s.host != nil)
	}
	s.enter(srv.URL + "/report")
	s.saved("q3.txt")
	if got := s.downloaded("q3.txt"); got != "figures" {
		t.Fatalf("an attachment's file holds %q", got)
	}
	s.at(srv.URL + "/vaults")
	if seen.count("/paper.pdf") != 2 {
		t.Fatalf("the file was asked for %d times, not once to see what it is and once to save it", seen.count("/paper.pdf"))
	}

	s.OpenInNewTab(srv.URL + "/paper.pdf")
	s.until("a tab opened on a file says it is downloaded", func() bool {
		text := s.node("page").Text()
		return strings.Contains(text, "paper.pdf") && strings.Contains(text, "downloaded")
	})
	s.settled()
	if got := s.downloaded("paper (1).pdf"); got == "" {
		t.Fatal("the second download wrote over the first, or nowhere")
	}
	s.clean()
}

func TestTheMenuSavesALinkAPictureAndThePage(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.rightClickAt(s.inPage("vault"))
	s.choose("Save link as…")
	s.settled()
	if got := s.downloaded("vaults.html"); !strings.Contains(got, "<title>Vaults</title>") {
		t.Fatalf("the link's file holds %q", got)
	}
	s.at(srv.URL + "/")
	s.rightClickAt(s.inPage("space"))
	s.choose("Save page as…")
	s.settled()
	if got := s.downloaded("A crypt.html"); !strings.HasPrefix(got, "<title>A crypt</title>") {
		t.Fatalf("the page's file holds %q", got)
	}
	if rows := strings.Join(s.requests(), ", "); !strings.Contains(rows, "OK /vaults by download") {
		t.Fatalf("the requests are %s", rows)
	}
}

func TestALinkWithADownloadAttributeIsSavedUnderItsName(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/files")
	s.clickAt(s.inPage("named"))
	s.settled()
	if got := s.downloaded("kept notes.txt"); got != "line one\nline two\n" {
		t.Fatalf("the file holds %q", got)
	}
	s.at(srv.URL + "/files")
	s.clickAt(s.inPage("paper"))
	s.saved("paper.pdf")
	s.at(srv.URL + "/files")
	if got := s.downloaded("paper.pdf"); got == "" {
		t.Fatal("a link to a file light does not show saved nothing")
	}
}

func TestTheDownloadsButtonShowsProgressAndTheRecentOnes(t *testing.T) {
	srv := slow(t)
	s := show(t)
	if s.node("downloads").HasClass("busy") {
		t.Fatal("the button is busy with nothing to download")
	}
	s.enter(srv.URL + "/big.zip")
	s.quarter()
	s.frame()
	bar, button := s.box("downloads-progress"), s.box("downloads")
	if !s.node("downloads").HasClass("busy") || bar.W != button.W/4 {
		t.Fatalf("a quarter in, the bar is %v wide in a button of %v", bar.W, button.W)
	}
	if field := s.box("field"); button.X < field.X+field.W {
		t.Fatalf("the button at %v is not right of the address field %v", button, field)
	}
	if got := s.status.Text(); got != "Downloading big.zip…" {
		t.Fatalf("the bubble says %q", got)
	}
	s.clean()
	s.click("downloads")
	recent := s.node("downloads-recent")
	if _, shown := s.doc.BoxOf(recent); !shown || !strings.Contains(recent.Text(), "big.zip") ||
		!strings.Contains(recent.Text(), "1.0 kB of 4.0 kB") {
		t.Fatalf("the list of recent downloads reads %q", recent.Text())
	}
	s.clean()
	all := s.node("all-downloads")
	s.showRecent()
	if s.node("all-downloads") != all {
		t.Fatal("the list is built again while nothing but the progress changes: a click on it would be lost")
	}
	srv.open()
	s.settled()
	s.frame()
	if s.node("downloads").HasClass("busy") || !strings.Contains(recent.Text(), "4.0 kB") ||
		strings.Contains(recent.Text(), " of ") {
		t.Fatalf("at the end the list reads %q", recent.Text())
	}
	s.click("all-downloads")
	s.at(downloadsPage)
	if _, shown := s.doc.BoxOf(recent); shown {
		t.Fatal("the list stays open over the page it led to")
	}
}

func TestTheDownloadsPageListsThemWithWhatCanBeDone(t *testing.T) {
	site, _ := crypt(t)
	slowly := slow(t)
	files := &shell{}
	s := showWith(t, Options{Downloads: t.TempDir(), Files: files})
	s.Open(downloadsPage)
	s.frame()
	if s.doc.ByID("downloads-none") == nil {
		t.Fatal("an empty list says nothing")
	}
	done := s.saves.Start(t.Context(), site.URL+"/paper.pdf", "")
	s.settled()
	failed := s.saves.Start(t.Context(), site.URL+"/missing", "")
	s.settled()
	if got, want := s.status.Text(),
		"missing was not downloaded: the server answered 404 Not Found"; got != want {
		t.Fatalf("the bubble says %q, want %q", got, want)
	}
	active := s.saves.Start(t.Context(), slowly.URL+"/big.zip", "")
	s.quarter()
	s.frame()
	rows := s.node("downloads-list").Children()
	if len(rows) != 3 || !strings.Contains(rows[0].Text(), "big.zip") ||
		!strings.Contains(rows[0].Text(), "1.0 kB of 4.0 kB") ||
		!strings.Contains(rows[1].Text(), "Failed: the server answered 404 Not Found") ||
		!strings.Contains(rows[2].Text(), "paper.pdf") || !strings.Contains(rows[2].Text(), site.URL+"/paper.pdf") ||
		!strings.Contains(rows[2].Text(), "16 bytes") {
		t.Fatalf("the page lists %q", s.node("downloads-list").Text())
	}
	s.clean()
	row := s.node("download-" + strconv.Itoa(active))
	s.listDownloads(s.listed)
	if s.node("download-"+strconv.Itoa(active)) != row {
		t.Fatal("the rows are built again while nothing but the progress changes: a click on one would be lost")
	}

	s.act(done, "open")
	s.act(done, "reveal")
	if got := strings.Join(files.asked, ", "); got != "open paper.pdf, reveal paper.pdf" {
		t.Fatalf("the system was asked to %s", got)
	}
	s.act(done, "copy")
	if got := s.win.Clipboard(); got != site.URL+"/paper.pdf" {
		t.Fatalf("copied %q", got)
	}
	for _, id := range []int{failed, active} {
		for _, b := range s.node("download-" + strconv.Itoa(id)).Children()[2].Children() {
			if b.HasClass("open") || b.HasClass("reveal") {
				t.Fatalf("download %d, which has no file, offers to open it", id)
			}
		}
	}
	s.act(active, "cancel")
	s.settled()
	s.frame()
	if text := s.node("download-" + strconv.Itoa(active)).Text(); !strings.Contains(text, "Cancelled") {
		t.Fatalf("cancelled, it reads %q", text)
	}
	slowly.open()
	s.act(active, "retry")
	s.settled()
	s.frame()
	if got := s.downloaded("big.zip"); len(got) != 4000 {
		t.Fatalf("retried, the file holds %d bytes", len(got))
	}
	if err := os.Remove(filepath.Join(s.opts.Downloads, "paper.pdf")); err != nil {
		t.Fatal(err)
	}
	s.Open(downloadsPage)
	s.frame()
	if text := s.node("download-" + strconv.Itoa(done)).Text(); !strings.Contains(text, "File removed") {
		t.Fatalf("its file gone, it reads %q", text)
	}
	s.act(failed, "remove")
	if rows = s.node("downloads-list").Children(); len(rows) != 2 {
		t.Fatalf("%d rows after removing one", len(rows))
	}
	s.click("downloads-clear")
	if s.doc.ByID("downloads-none") == nil || s.downloaded("big.zip") == "" {
		t.Fatal("Clear left rows, or removed a file")
	}
	s.clean()
}

func TestDownloadsAreReachedByTheirTileTheirKeysAndTheMenu(t *testing.T) {
	s := show(t)
	s.click("go-downloads")
	s.at(downloadsPage)
	s.Open("light://guide")
	s.frame()
	if runtime.GOOS == "darwin" {
		s.in.SetKey(input.KeyLeftShift, true)
		s.command(input.KeyJ)
		s.in.SetKey(input.KeyLeftShift, false)
	} else {
		s.chord(input.KeyLeftControl, input.KeyJ)
	}
	s.frame()
	s.at(downloadsPage)
	s.Open("light://guide")
	s.frame()
	s.rightClickAt(s.node("page").Children()[0].Box().Center())
	s.choose("Downloads")
	s.at(downloadsPage)
}

func TestDownloadsAreKeptBetweenRunsAndQuittingEndsThoseInFlight(t *testing.T) {
	site, _ := crypt(t)
	slowly := slow(t)
	dir, folder := t.TempDir(), t.TempDir()
	s := showWith(t, Options{Dir: dir, Downloads: folder})
	s.saves.Start(t.Context(), site.URL+"/paper.pdf", "")
	s.settled()
	s.saves.Start(t.Context(), slowly.URL+"/big.zip", "")
	s.quarter()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(folder); len(entries) != 1 || entries[0].Name() != "paper.pdf" {
		t.Fatalf("after quitting the folder holds %v", entries)
	}
	again := showWith(t, Options{Dir: dir, Downloads: folder})
	again.Open(downloadsPage)
	again.frame()
	rows := again.node("downloads-list").Children()
	if len(rows) != 2 || !strings.Contains(rows[0].Text(), "Cancelled") || !strings.Contains(rows[1].Text(), "paper.pdf") {
		t.Fatalf("a new run lists %q", again.node("downloads-list").Text())
	}
}
