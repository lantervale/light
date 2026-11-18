package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/lantervale/light/internal/menu"
	"github.com/lantervale/light/pkg/downloads"
	"github.com/lantervale/light/pkg/netlog"
)

// bodiesFromTheStart is whether a tab's requests keep their bodies, for
// the HAR files, before its developer tools were ever opened: the
// request that led to a fault is then whole in the file, at the cost of
// up to netlog.BodyBudget of memory a tab. Without it a tab keeps them
// from the first time its tools are open, as Chrome records.
const bodiesFromTheStart = true

// keepBodies has session's log keep bodies when it is to.
func (s *Screen) keepBodies(session *session) {
	if bodiesFromTheStart || session.tools.Open {
		session.log.KeepBodies(netlog.BodyCap, netlog.BodyBudget)
	}
}

// titled is the title of the shown tab's page at address, "" for none.
func (s *Screen) titled(address string) string {
	if page := s.session().pages[address]; page != nil {
		return page.Title
	}
	return ""
}

// archive is the HAR file of the requests the developer tools list, or
// of request id alone when it is not 0, and whether there is one: none
// of no request.
func (s *Screen) archive(id int, sensitive bool) ([]byte, bool) {
	note := "No request to save"
	if len(s.tools.Listed()) > 0 {
		data, err := s.tools.HAR(id, sensitive)
		if err == nil {
			return data, true
		}
		note = err.Error()
	}
	s.tell(note)
	return nil, false
}

// saveHAR saves the HAR file of archive to the downloads folder, as a
// download named after the site and the time, never over another file.
// The status bubble says where once it is written.
func (s *Screen) saveHAR(id int, sensitive bool) {
	data, ok := s.archive(id, sensitive)
	if !ok {
		return
	}
	address := shownBy(s.browser.Tab().Current())
	if row, found := s.session().log.Row(id); found {
		address = row.Address
	}
	s.archived[s.saves.Write(address, archiveName(hostOf(address), s.opts.Now()), "application/json", data)] = sensitive
}

// archiveName is the name of a HAR file of host's requests saved at at.
func archiveName(host string, at time.Time) string {
	if host = strings.ReplaceAll(host, ":", "-"); host == "" {
		host = "requests"
	}
	return host + "-" + at.Format("20060102-150405") + ".har"
}

// copyHAR puts the HAR file of archive in the clipboard.
func (s *Screen) copyHAR(id int, sensitive bool) {
	if data, ok := s.archive(id, sensitive); ok {
		s.win.SetClipboard(string(data))
		s.tell("Copied as HAR, " + sensitivity(sensitive))
	}
}

// tell says note in the status bubble, over the address of the request
// under the pointer until it moves: a menu's entry is chosen over a row.
func (s *Screen) tell(note string) {
	s.session().note, s.telling, s.told = note, true, s.win.Input().Mouse()
	s.say()
}

func sensitivity(sensitive bool) string {
	if sensitive {
		return "with sensitive data"
	}
	return "sanitized"
}

// archiveEntries are the menu's entries that copy and save request id as
// a HAR file, and save those listed; with no request, those alone. Each
// is there twice: sanitized, then with the sensitive data.
func (s *Screen) archiveEntries(id int) []menu.Entry {
	var entries []menu.Entry
	none := len(s.tools.Listed()) == 0
	for _, sensitive := range []bool{false, true} {
		how := " (" + sensitivity(sensitive) + ")"
		entries = append(entries, menu.Separator())
		if id != 0 {
			entries = append(entries,
				menu.Entry{Label: "Copy as HAR" + how, Do: func() { s.copyHAR(id, sensitive) }},
				menu.Entry{Label: "Save as HAR" + how, Do: func() { s.saveHAR(id, sensitive) }})
		}
		entries = append(entries,
			menu.Entry{Label: "Save all as HAR" + how, Do: func() { s.saveHAR(0, sensitive) }, Disabled: none})
	}
	return entries
}

// said is what the status bubble says of a download in its state: of a
// HAR file that is written, its name, what it holds and where it is, as
// tell says it.
func (s *Screen) said(item downloads.Item) string {
	if sensitive, ok := s.archived[item.ID]; ok && item.State == downloads.Done {
		s.telling, s.told = true, s.win.Input().Mouse()
		return "Saved " + item.Name + " (" + sensitivity(sensitive) + ") in " + filepath.Dir(item.Path)
	}
	return said(item)
}
