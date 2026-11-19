package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/content"
	"github.com/lantervale/light/pkg/downloads"
	"github.com/lantervale/light/pkg/netlog"
)

const (
	// downloadsPage is where the downloads are listed.
	downloadsPage = own + downloadsName
	// downloadsFile is the downloads' file in the folder light is given.
	downloadsFile = "downloads.json"
	// maxDownloads is how many downloads the list keeps between runs.
	maxDownloads = 500
	// recentDownloads is how many the navigation row's list shows.
	recentDownloads = 5
)

// Files is what the system does with a file light downloaded, on the
// player's click only.
type Files interface {
	// Open opens the file with what the system opens its kind with.
	Open(path string) error
	// Reveal shows the file in its folder.
	Reveal(path string) error
}

// download saves what address gives, under name when one is given, as
// a request of the shown tab.
func (s *Screen) download(address, name string) {
	s.save(address, name, shownBy(s.browser.Tab().Current()))
}

// save is download for what the page at from led to, "" for none.
func (s *Screen) save(address, name, from string) {
	ctx := s.saving(netlog.With(context.Background(), s.session().log, "download", netlog.Other), from)
	s.saves.Start(ctx, address, name)
}

// sourceShown is the shown tab's page when it has a text to save: an
// HTML page or a text of the web.
func (s *Screen) sourceShown() *web.Page {
	page := s.session().pages[shownBy(s.browser.Tab().Current())]
	if page == nil || page.Err != nil || (page.Kind != content.Page && page.Kind != content.Text) {
		return nil
	}
	return page
}

// savePage saves the text the shown page's server sent, under the
// page's title.
func (s *Screen) savePage() {
	page := s.sourceShown()
	if page == nil {
		return
	}
	named := mime.FormatMediaType("attachment", map[string]string{"filename": page.Title})
	s.saves.Write(page.Address, content.SaveName(named, page.Address, page.Media), page.Media, []byte(page.Source))
}

// downloadsButton makes the navigation row's button, with the bar that
// fills as the downloads in progress come, and the list it opens.
func (s *Screen) downloadsButton() *ui.Node {
	s.progress = ui.Span(ui.ID("downloads-progress"))
	s.recent = ui.Div(ui.ID("downloads-recent"))
	s.downloads = s.button("downloads", saves, "download", func() {
		toggle(s.recent, "shown", !s.recent.HasClass("shown"))
		s.showRecent()
	})
	s.downloads.Append(ui.Span(ui.ID("downloads-track")), s.progress)
	return s.downloads
}

// watchDownloads shows what changed in the downloads since the last
// frame: the button's bar, the lists that are shown, and in the status
// bubble a download that began or ended.
func (s *Screen) watchDownloads() {
	version := s.saves.Version()
	if version == s.savesSeen {
		return
	}
	s.savesSeen = version
	s.saved.touch()
	items := s.saves.Items()
	s.showProgress(items)
	s.showRecent()
	if s.listed != nil {
		s.listDownloads(s.listed)
	}
	s.announce(items)
}

// showProgress fills the button's bar with the part received of the
// downloads in progress whose size is known.
func (s *Screen) showProgress(items []downloads.Item) {
	var received, total int64
	active := false
	for _, item := range items {
		if item.State != downloads.Active {
			continue
		}
		active = true
		if item.Total > 0 {
			received, total = received+item.Received, total+item.Total
		}
	}
	toggle(s.downloads, "busy", active)
	part := 0.0
	if total > 0 {
		part = float64(received) / float64(total)
	}
	s.progress.SetStyle(fmt.Sprintf("width: %g%%", 100*part))
}

// announce says in the status bubble that a download began, once its
// answer named it, or ended; of several at once, the newest is said.
func (s *Screen) announce(items []downloads.Item) {
	for _, item := range slices.Backward(items) {
		unnamed := item.Name == "" && item.State == downloads.Active
		if unnamed || s.announced[item.ID] == item.State {
			continue
		}
		s.announced[item.ID] = item.State
		s.session().note = s.said(item)
	}
	s.say()
}

// said is what the status bubble says of a download in its state.
func said(item downloads.Item) string {
	switch item.State {
	case downloads.Active:
		return "Downloading " + named(item) + "…"
	case downloads.Done:
		return "Downloaded " + named(item)
	case downloads.Cancelled:
		return "Cancelled " + named(item)
	case downloads.Failed:
		return named(item) + " was not downloaded: " + item.Err
	}
	return ""
}

// listing is a list of downloads as it was last built: for which
// downloads in which states, and the element saying where each stands.
// While those are the same only the elements' texts change, so a click
// begun on the list ends on the same element.
type listing struct {
	of     string
	states map[int]*ui.Node
}

// built reports whether the list still shows items, and has it say where
// each now stands; when it does not, the list is to be built again.
func (l *listing) built(s *Screen, items []downloads.Item) bool {
	of := ""
	for _, item := range items {
		of += fmt.Sprintf("%d %v %s %t,", item.ID, item.State, item.Name, item.Missing(onDisk))
	}
	if of != l.of || l.states == nil {
		l.of, l.states = of, map[int]*ui.Node{}
		return false
	}
	for _, item := range items {
		if text := s.state(item); l.states[item.ID].Text() != text {
			l.states[item.ID].SetText(text)
		}
	}
	return true
}

// state is the element saying where item stands, kept to be updated.
func (l *listing) state(s *Screen, item downloads.Item) *ui.Node {
	l.states[item.ID] = ui.Div(ui.Class("state"), ui.Text(s.state(item)))
	return l.states[item.ID]
}

// showRecent fills the button's list, while it is open, with the newest
// downloads and a way to them all.
func (s *Screen) showRecent() {
	if !s.recent.HasClass("shown") {
		s.recentShown = listing{}
		return
	}
	items := s.saves.Items()
	items = items[:min(recentDownloads, len(items))]
	if s.recentShown.built(s, items) {
		return
	}
	empty(s.recent)
	if len(items) == 0 {
		s.recent.Append(ui.P(ui.Class("none"), ui.Text("Nothing downloaded.")))
	}
	for _, item := range items {
		s.recent.Append(ui.Div(ui.Class("recent"),
			ui.Div(ui.Class("name"), ui.Text(named(item))), s.recentShown.state(s, item)))
	}
	s.recent.Append(ui.Div(ui.ID("all-downloads"), ui.Class("link"), ui.TabIndex(0), ui.Text("All downloads"),
		ui.OnClick(func() { s.Open(downloadsPage) })))
}

// named is the name a download is listed under: its file's, or until
// its answer names it, the one its address ends with.
func named(item downloads.Item) string {
	if item.Name == "" {
		return content.FileName("", item.Address)
	}
	return item.Name
}

// state says where a download stands: what came of what is to come, how
// fast and for how long still, or how it ended.
func (s *Screen) state(item downloads.Item) string {
	switch {
	case item.Missing(onDisk):
		return "File removed"
	case item.State == downloads.Done:
		return bytesOf(item.Received)
	case item.State == downloads.Cancelled:
		return "Cancelled"
	case item.State == downloads.Failed:
		return "Failed: " + item.Err
	}
	text := bytesOf(item.Received)
	if item.Total > 0 {
		text += " of " + bytesOf(item.Total)
	}
	if item.Speed > 0 {
		text += " · " + bytesOf(int64(item.Speed)) + "/s"
	}
	if left, ok := item.Remaining(); ok {
		text += " · " + left.Round(time.Second).String() + " left"
	}
	return text
}

func onDisk(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// downloadsBody is the downloads page: a button that clears the list,
// and the downloads under it, newest first.
func downloadsBody(s *Screen) []*ui.Node {
	s.listed, s.listedShown = ui.Div(ui.ID("downloads-list")), listing{}
	forget := ui.Button(ui.ID("downloads-clear"), ui.Text("Clear all"), ui.OnClick(s.saves.Clear))
	s.listDownloads(s.listed)
	return []*ui.Node{
		ui.Div(ui.ID("downloads-head"), ui.H1(ui.Text(saves)), forget),
		ui.P(ui.Class("kind"), ui.Text("Saved to "+s.downloadsDir()+". Clearing the list removes no file; "+
			"downloaded files are not scanned.")),
		s.listed,
	}
}

// listDownloads fills list with the downloads: each one's name, its
// address, where it stands, and what can be done with it.
func (s *Screen) listDownloads(list *ui.Node) {
	items := s.saves.Items()
	if s.listedShown.built(s, items) {
		return
	}
	empty(list)
	if len(items) == 0 {
		list.Append(ui.P(ui.ID("downloads-none"), ui.Text("Nothing downloaded.")))
	}
	for _, item := range items {
		list.Append(ui.Div(ui.ID("download-"+strconv.Itoa(item.ID)), ui.Class("download"),
			ui.Div(ui.Class("what"),
				ui.Div(ui.Class("name"), ui.Text(named(item))),
				ui.Div(ui.Class("address"), ui.Text(item.Address))),
			s.listedShown.state(s, item),
			s.actions(item)))
	}
}

// actions are the buttons of a download: those its state allows.
func (s *Screen) actions(item downloads.Item) *ui.Node {
	row := ui.Div(ui.Class("actions"))
	act := func(class, label string, do func()) {
		row.Append(ui.Button(ui.Class(class), ui.Text(label), ui.OnClick(do)))
	}
	if item.State == downloads.Done && !item.Missing(onDisk) && s.opts.Files != nil {
		act("open", "Open", func() { s.withFile(item.ID, s.opts.Files.Open) })
		act("reveal", "Show in folder", func() { s.withFile(item.ID, s.opts.Files.Reveal) })
	}
	act("copy", "Copy address", func() { s.win.SetClipboard(item.Address) })
	switch item.State {
	case downloads.Active:
		act("cancel", "Cancel", func() { s.saves.Cancel(item.ID) })
	case downloads.Failed, downloads.Cancelled:
		if onTheWeb(item.Address) {
			act("retry", "Retry", func() {
				s.saves.Retry(s.saving(netlog.With(context.Background(), s.session().log, "download", netlog.Other), ""),
					item.ID)
			})
		}
		fallthrough
	case downloads.Done:
		act("remove", "Remove from list", func() { s.saves.Remove(item.ID) })
	}
	return row
}

// withFile hands the system the file of download id, only one light
// downloaded whole and that is still there, and says what went wrong.
func (s *Screen) withFile(id int, do func(path string) error) {
	item, ok := s.saves.Item(id)
	if !ok || item.State != downloads.Done || item.Missing(onDisk) {
		return
	}
	if err := do(item.Path); err != nil {
		s.session().note = err.Error()
		s.say()
	}
}
