package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/downloads"
	"github.com/lantervale/light/pkg/har"
	"github.com/lantervale/light/pkg/netlog"
)

// archives are the HAR files of the downloads folder, by name.
func (s *shown) archives() []string {
	s.t.Helper()
	files, err := filepath.Glob(filepath.Join(s.opts.Downloads, "*.har"))
	if err != nil {
		s.t.Fatal(err)
	}
	return files
}

// archived runs frames until the downloads folder holds one HAR file
// more than before, and gives it.
func (s *shown) archived(before []string) string {
	s.t.Helper()
	var file string
	s.until("a HAR file is written", func() bool {
		for _, name := range s.archives() {
			if !slices.Contains(before, name) {
				file = name
			}
		}
		return file != ""
	})
	s.settled()
	s.frame()
	return file
}

// read is data as the HAR 1.2 file it has to be.
func read(t *testing.T, data []byte) har.Archive {
	t.Helper()
	if err := har.Validate(data); err != nil {
		t.Fatalf("not a HAR 1.2 file:\n%v", err)
	}
	var a har.Archive
	if err := json.Unmarshal(data, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func readFile(t *testing.T, path string) har.Archive {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return read(t, data)
}

func header(pairs []har.Pair, name string) string {
	for _, p := range pairs {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

// answered runs frames until every request of the shown tab ended.
func (s *shown) answered() {
	s.t.Helper()
	s.until("every request ended", func() bool {
		return !slices.ContainsFunc(s.session().log.Rows(), func(r netlog.Row) bool { return !r.Done })
	})
	s.frame()
}

// consented checks a, the sanitized archive of a choice posted to the
// consenting site and of the page it led to, of that many requests.
func consented(t *testing.T, a har.Archive, requests int) {
	t.Helper()
	if a.Log.Creator != (har.Creator{Name: "light", Version: "9.9"}) || !strings.HasPrefix(a.Log.Comment, "Sanitized") {
		t.Fatalf("the file is of %+v, %q", a.Log.Creator, a.Log.Comment)
	}
	if len(a.Log.Pages) != 1 || a.Log.Pages[0].Title != "The site" || len(a.Log.Entries) != requests {
		t.Fatalf("%d entries of the pages %+v", len(a.Log.Entries), a.Log.Pages)
	}
	post, site := a.Log.Entries[0], a.Log.Entries[1]
	if post.Request.Method != http.MethodPost || post.Request.URL != "http://consent.site.test/save" ||
		post.Response.Status != http.StatusSeeOther || post.Response.RedirectURL != "http://www.site.test/?ucbcb=1" ||
		post.Kind != "document" {
		t.Fatalf("the first entry is %+v", post)
	}
	carried := post.Request.PostData
	if carried == nil || !strings.HasSuffix(carried.Text, "set_eom=false&set_sc=true&set_aps=true") ||
		carried.MimeType != "application/x-www-form-urlencoded" || carried.Params[0] != (har.Pair{Name: "gl", Value: "FR"}) ||
		carried.Params[1] != (har.Pair{Name: "continue", Value: "http://www.site.test/"}) {
		t.Fatalf("the form carried %+v", carried)
	}
	if got := header(post.Request.Headers, "Cookie"); got != har.Redacted {
		t.Fatalf("the form's Cookie header reads %q", got)
	}
	if got := header(post.Response.Headers, "Set-Cookie"); got != har.Redacted || len(post.Response.Cookies) != 1 ||
		post.Response.Cookies[0].Name != "SOCS" || post.Response.Cookies[0].Value != har.Redacted {
		t.Fatalf("the answer's cookies are %q and %+v", got, post.Response.Cookies)
	}
	if site.Request.URL != "http://www.site.test/?ucbcb=1" ||
		!strings.Contains(site.Response.Content.Text, "<title>The site</title>") ||
		header(site.Request.Headers, "Cookie") != har.Redacted || len(site.Request.Cookies) != 2 {
		t.Fatalf("the page's entry is %+v", site)
	}
}

func TestTheNetworkBarSavesItsRequestsAsAHARFileOfTheDownloads(t *testing.T) {
	n, _ := consenting(t, http.StatusSeeOther)
	now := time.Date(2026, time.November, 18, 16, 30, 5, 0, time.UTC)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport, Now: func() time.Time { return now },
		Version: "9.9"})
	s.toggleTools()
	s.visit(consentAddress)
	s.clickAt(s.inPage("yes"))
	const landed = "http://www.site.test/?ucbcb=1"
	s.until("the choice led to the site", func() bool { return s.session().pages[landed] != nil })
	s.answered()

	button := s.node("dt-har")
	if label, _ := button.Attr("aria-label"); label != "Save all as HAR" || button.Tag().String() != "button" {
		t.Fatalf("the button is a %s named %q", button.Tag(), label)
	}
	requests := len(s.session().log.Rows())
	s.click("dt-har")
	file := s.archived(nil)
	if got, want := filepath.Base(file), "www.site.test-20261118-163005.har"; got != want {
		t.Fatalf("the file is named %s, want %s", got, want)
	}
	if got, want := s.status.Text(),
		"Saved www.site.test-20261118-163005.har (sanitized) in "+s.opts.Downloads; got != want {
		t.Fatalf("the bubble says %q, want %q", got, want)
	}
	items := s.saves.Items()
	if len(items) != 1 || items[0].State != downloads.Done || items[0].Path != file || items[0].Address != landed {
		t.Fatalf("the downloads list %+v", items)
	}
	s.clean()

	a := readFile(t, file)
	consented(t, a, requests)

	s.click("dt-har")
	if again := s.archived([]string{file}); again == file || len(s.archives()) != 2 {
		t.Fatalf("a second file of the same second is %s, among %v", again, s.archives())
	}
	if got := readFile(t, file); len(got.Log.Entries) != len(a.Log.Entries) {
		t.Fatal("the first file was written over")
	}
}

func TestARightClickOnARequestCopiesAndSavesItAsHAR(t *testing.T) {
	n, _ := consenting(t, http.StatusSeeOther)
	s := showWith(t, Options{Downloads: t.TempDir(), Transport: n.transport})
	s.toggleTools()
	s.visit(consentAddress)
	s.clickAt(s.inPage("yes"))
	s.until("the choice led to the site", func() bool { return s.session().pages["http://www.site.test/?ucbcb=1"] != nil })
	s.answered()
	row := s.requestRow("/save")

	s.rightClick(row)
	s.choose("Copy as HAR (sanitized)")
	a := read(t, []byte(s.win.Clipboard()))
	if len(a.Log.Entries) != 1 || a.Log.Entries[0].Request.URL != "http://consent.site.test/save" ||
		a.Log.Entries[0].Response.Cookies[0].Value != har.Redacted || len(a.Log.Pages) != 1 {
		t.Fatalf("the clipboard holds %+v", a.Log)
	}
	if got := s.status.Text(); got != "Copied as HAR, sanitized" {
		t.Fatalf("the bubble says %q", got)
	}
	if len(s.archives()) != 0 {
		t.Fatal("a copy wrote a file")
	}

	s.rightClick(row)
	s.choose("Copy as HAR (with sensitive data)")
	a = read(t, []byte(s.win.Clipboard()))
	if got := a.Log.Entries[0]; got.Response.Cookies[0].Value != "accepted" || a.Log.Comment != "" ||
		header(got.Request.Headers, "Cookie") != "seen=1" {
		t.Fatalf("the clipboard holds %+v, %q", got, a.Log.Comment)
	}

	s.rightClick(row)
	s.choose("Save as HAR (with sensitive data)")
	file := s.archived(nil)
	if !strings.HasPrefix(filepath.Base(file), "consent.site.test-") {
		t.Fatalf("the file is named %s", file)
	}
	if got := s.status.Text(); got != "Saved "+filepath.Base(file)+" (with sensitive data) in "+s.opts.Downloads {
		t.Fatalf("the bubble says %q", got)
	}
	a = readFile(t, file)
	if len(a.Log.Entries) != 1 || a.Log.Entries[0].Response.Cookies[0].Value != "accepted" {
		t.Fatalf("the file holds %+v", a.Log.Entries)
	}

	requests := len(s.session().log.Rows())
	s.rightClick(row)
	s.choose("Save all as HAR (sanitized)")
	if a = readFile(t, s.archived([]string{file})); len(a.Log.Entries) != requests ||
		a.Log.Entries[0].Response.Cookies[0].Value != har.Redacted {
		t.Fatalf("the file of all holds %d entries", len(a.Log.Entries))
	}
	s.clean()
}

func TestAHARFileHoldsWhatIsListedAndTheBodiesOfBeforeTheToolsOpened(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.visit(srv.URL + "/")
	s.until("the page's sheet, pictures and icon were asked for", func() bool {
		asked := strings.Join(s.requests(), ", ")
		return strings.Contains(asked, "/site.css") && strings.Contains(asked, "/map.png") &&
			strings.Contains(asked, "/favicon.ico")
	})
	s.answered()
	s.toggleTools()
	s.frame()
	requests := len(s.session().log.Rows())

	s.rightClick("dt-har")
	if got, want := labels(s.entries()),
		"[Save all as HAR (sanitized) - Save all as HAR (with sensitive data)]"; got != want {
		t.Fatalf("the button's menu is %s, want %s", got, want)
	}
	s.choose("Save all as HAR (with sensitive data)")
	all := readFile(t, s.archived(nil))
	if len(all.Log.Entries) != requests || len(all.Log.Pages) != 1 ||
		all.Log.Pages[0].Title != "A crypt" {
		t.Fatalf("%d entries of the pages %+v", len(all.Log.Entries), all.Log.Pages)
	}
	if page := all.Log.Entries[0].Response.Content; !strings.Contains(page.Text, "The lower vaults stay wet.") ||
		page.Truncated {
		t.Fatalf("the page asked for before the tools opened has the content %+v", page)
	}

	s.click("dt-chip-image")
	before := s.archives()
	s.click("dt-har")
	pictures := readFile(t, s.archived(before))
	if len(pictures.Log.Entries) == 0 || len(pictures.Log.Entries) >= len(all.Log.Entries) {
		t.Fatalf("with the pictures alone listed the file holds %d entries of %d", len(pictures.Log.Entries),
			len(all.Log.Entries))
	}
	for _, e := range pictures.Log.Entries {
		raw, err := base64.StdEncoding.DecodeString(e.Response.Content.Text)
		if e.Kind != "image" || e.Pageref != pictures.Log.Pages[0].ID || e.Response.Content.Encoding != "base64" ||
			err != nil || !strings.HasPrefix(string(raw), "\x89PNG") {
			t.Fatalf("a picture's entry is %+v", e)
		}
	}
	s.click("dt-chip-all")
	s.doc.SetValue(s.node("dt-filter"), "site.css")
	s.tools.State().Filter = "site.css"
	s.tools.Refresh()
	before = s.archives()
	s.click("dt-har")
	if sheet := readFile(t, s.archived(before)); len(sheet.Log.Entries) != 1 ||
		sheet.Log.Entries[0].Request.URL != srv.URL+"/site.css" {
		t.Fatalf("with a filter the file holds %+v", sheet.Log.Entries)
	}
	s.clean()
}

func TestTheBodiesOfAHARFileGoWithTheRequestsUnlessTheLogIsPreserved(t *testing.T) {
	srv, _ := crypt(t)
	s := show(t)
	s.toggleTools()
	s.visit(srv.URL + "/vaults")
	s.visit(srv.URL + "/notes.txt")
	s.answered()
	data, err := s.tools.HAR(0, false)
	if err != nil {
		t.Fatal(err)
	}
	if a := read(t, data); len(a.Log.Pages) != 1 || len(a.Log.Entries) != 1 ||
		a.Log.Entries[0].Response.Content.Text != "line one\nline two\n" {
		t.Fatalf("after a second page the file holds %+v", a.Log)
	}
	if held := s.session().log.Held(); held != int64(len("line one\nline two\n")) {
		t.Fatalf("the log holds %d bytes of bodies", held)
	}
	s.click("dt-preserve")
	s.visit(srv.URL + "/app.js")
	s.answered()
	data, _ = s.tools.HAR(0, false)
	a := read(t, data)
	if len(a.Log.Pages) != 2 || len(a.Log.Entries) != 2 || a.Log.Entries[0].Pageref == a.Log.Entries[1].Pageref ||
		a.Log.Entries[0].Response.Content.Text != "line one\nline two\n" ||
		!strings.HasPrefix(a.Log.Entries[1].Response.Content.Text, "let a = 1;") {
		t.Fatalf("with the log preserved the file holds %+v", a.Log)
	}

	before := s.archives()
	s.click("dt-clear")
	s.click("dt-har")
	for range 5 {
		s.frame()
	}
	if got := s.status.Text(); got != "No request to save" || len(s.archives()) != len(before) ||
		s.session().log.Held() != 0 {
		t.Fatalf("of an empty list the bubble says %q, with %d files", got, len(s.archives()))
	}
	s.rightClick("dt-rows")
	if got, want := labels(s.entries()),
		"[Save all as HAR (sanitized) (off) - Save all as HAR (with sensitive data) (off)]"; got != want {
		t.Fatalf("an empty table's menu is %s, want %s", got, want)
	}
}
