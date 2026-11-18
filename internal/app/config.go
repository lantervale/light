package app

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/internal/web"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/fetch"
	"github.com/lantervale/light/pkg/netlog"
	"github.com/lantervale/light/pkg/tabs"
)

const (
	// configPage is where the settings are shown and changed.
	configPage = own + configName
	// configFile is the settings' file in the folder light is given.
	configFile = "config.json"
	// settings is what the settings are called.
	settings = "Settings"
	// zoomStep is how far a tab's zoom keys move it, in percent.
	zoomStep = 10
)

// current is the loader of the settings as they stand; a load keeps the
// one it began with.
func (s *Screen) current() *web.Loader { return s.loader.Load() }

// Open asks through the client of the settings as they stand, so a
// download is sent as a page is.
func (s *Screen) open(ctx context.Context, address, accept string) (*fetch.Stream, error) {
	return s.current().Client.Open(ctx, address, accept)
}

// opener is a function as the downloads' fetch.
type opener func(ctx context.Context, address, accept string) (*fetch.Stream, error)

func (o opener) Open(ctx context.Context, address, accept string) (*fetch.Stream, error) {
	return o(ctx, address, accept)
}

// configure changes the settings: what is out of bounds falls back to
// its default, and what changed is applied at once and written.
func (s *Screen) configure(change func(c *config.Config)) {
	c := s.config
	change(&c)
	if c = c.Valid(); c != s.config {
		s.config = c
		s.settings.touch()
		s.apply()
		s.shade()
	}
	for _, refresh := range s.fields {
		refresh()
	}
}

// apply has the settings hold: the requests and the pages loaded from
// now on, the pages already loaded where they can, the downloads'
// folder, the history's and the requests' caps.
func (s *Screen) apply() {
	c := s.config
	s.loader.Store(&web.Loader{
		Client: fetch.New(&netlog.Transport{Base: s.opts.Transport}, fetch.Agent(c.UserAgent),
			fetch.Redirects(c.Redirects)),
		NoPictures: !c.Pictures, NoRefresh: !c.MetaRefresh, Zoom: float32(c.Zoom) / 100,
	})
	s.browser.Start = c.StartPage
	s.visits.SetLimit(c.HistoryLimit)
	s.saves.SetDir(s.downloadsDir())
	for _, session := range s.sessions {
		session.log.SetLimit(c.Requests)
		s.zoomPages(session)
	}
}

// shade draws the shown page dark when the settings say so. A page of
// the web is left as its site made it.
func (s *Screen) shade() {
	toggle(s.view, "dark", s.config.DarkMode && s.host == nil)
}

// logo is the logo the start page shows, by the settings.
func (s *Screen) logo() string {
	if s.config.DarkMode {
		return darkLogo
	}
	return logo
}

// downloadsDir is the folder downloads are saved to: the one the run was
// given, else the settings', else the system's.
func (s *Screen) downloadsDir() string {
	switch {
	case s.opts.Downloads != "":
		return s.opts.Downloads
	case s.config.Downloads != "":
		return s.config.Downloads
	}
	return s.opts.DefaultDownloads
}

// timeout is how long a page and its sheets may take.
func (s *Screen) timeout() time.Duration { return time.Duration(s.config.Timeout) * time.Second }

// zoomOf is the zoom of a tab's pages, in percent: its own once its keys
// set one, else the settings'.
func (s *Screen) zoomOf(session *session) int {
	if session.zoom != 0 {
		return session.zoom
	}
	return s.config.Zoom
}

// zoomPages shows the pages a tab holds at its zoom.
func (s *Screen) zoomPages(session *session) {
	for _, page := range session.pages {
		if page.Doc != nil {
			page.Doc.SetZoom(float32(s.zoomOf(session)) / 100)
		}
	}
}

// zoom moves the shown tab's own zoom by steps of zoomStep, within the
// settings' bounds; no step is back to the settings' zoom.
func (s *Screen) zoom(steps int) {
	session := s.session()
	session.zoom = 0
	if steps != 0 {
		session.zoom = min(max(s.zoomOf(session)+steps*zoomStep, config.MinZoom), config.MaxZoom)
	}
	s.zoomPages(session)
	session.note = percent(s.zoomOf(session), "Zoom ")
	s.say()
}

func percent(value int, before string) string { return fmt.Sprintf("%s%d %%", before, value) }

// configBody is the settings' page: each setting a field that applies
// as it changes, and a button back to the defaults.
func configBody(s *Screen) []*ui.Node {
	s.fields = nil
	body := []*ui.Node{ui.Div(ui.ID("config-head"), ui.H1(ui.Text(settings)),
		ui.Button(ui.ID("set-reset"), ui.Text("Reset to defaults"), ui.OnClick(func() {
			s.configure(func(c *config.Config) { *c = config.Default() })
		})))}
	if s.misread != nil {
		body = append(body, ui.P(ui.ID("config-problem"), ui.Text("The settings' file is not in use: "+
			s.misread.Error()+". It is written again when a setting changes.")))
	}
	return append(body,
		ui.H2(ui.Text("Start")),
		s.textSetting("set-start-page", "Start page", "What a new tab and the home button show: "+tabs.Home+
			" or an address.", func(c *config.Config) *string { return &c.StartPage }, tabs.Resolve),
		ui.H2(ui.Text("Appearance")),
		s.checkSetting("set-dark-mode", "Dark mode (light's own pages; the start page's logo switches it too)",
			func(c *config.Config) *bool { return &c.DarkMode }),
		ui.H2(ui.Text(saves)),
		s.textSetting("set-downloads", "Folder", "Where files are saved. Empty is the system's: "+
			s.opts.DefaultDownloads+".", func(c *config.Config) *string { return &c.Downloads }, same),
		s.given(),
		ui.H2(ui.Text(visited)),
		s.checkSetting("set-keep-history", "Keep the pages visited",
			func(c *config.Config) *bool { return &c.KeepHistory }),
		s.numberSetting("set-history-limit", "Visits kept", "", config.MinHistory, config.MaxHistory,
			func(c *config.Config) *int { return &c.HistoryLimit }),
		ui.Div(ui.Class("setting"), ui.Button(ui.ID("set-clear-history"), ui.Text("Clear history"),
			ui.OnClick(s.clearHistory))),
		ui.H2(ui.Text("Pages")),
		s.zoomSetting(),
		s.checkSetting("set-pictures", "Load pictures (pages loaded from now on)",
			func(c *config.Config) *bool { return &c.Pictures }),
		s.checkSetting("set-meta-refresh", "Follow a page that refreshes elsewhere at once (from now on)",
			func(c *config.Config) *bool { return &c.MetaRefresh }),
		ui.H2(ui.Text("Network")),
		s.numberSetting("set-timeout", "Request timeout", "Seconds a page and its sheets may take.",
			config.MinTimeout, config.MaxTimeout, func(c *config.Config) *int { return &c.Timeout }),
		s.numberSetting("set-redirects", "Redirects followed", "For one navigation.",
			config.MinRedirects, config.MaxRedirects, func(c *config.Config) *int { return &c.Redirects }),
		s.textSetting("set-user-agent", "User-Agent", "What light says it is to a server.",
			func(c *config.Config) *string { return &c.UserAgent }, same),
		ui.H2(ui.Text("Developer tools")),
		s.checkSetting("set-preserve-log", "Preserve the log in new tabs",
			func(c *config.Config) *bool { return &c.PreserveLog }),
		s.numberSetting("set-requests", "Requests kept", "In each tab's Network panel.",
			config.MinRequests, config.MaxRequests, func(c *config.Config) *int { return &c.Requests }),
	)
}

func same(text string) string { return text }

// given says the run was given a downloads folder, which the setting
// does not change.
func (s *Screen) given() *ui.Node {
	note := ui.P(ui.ID("downloads-given"), ui.Class("hint"))
	if s.opts.Downloads != "" {
		note.SetText("This run was given a folder, used whatever the setting: " + s.opts.Downloads + ".")
	}
	return note
}

// setting is one row of the page: a label, its field, and what it is.
func setting(id, label, hint string, field *ui.Node) *ui.Node {
	row := ui.Div(ui.Class("setting"), ui.Label(ui.Attr("for", id), ui.Text(label)), field)
	if hint != "" {
		row.Append(ui.Span(ui.Class("hint"), ui.Text(hint)))
	}
	return row
}

// entered has field apply when it is left changed or Enter is pressed
// in it, and shown again after any change of the settings.
func (s *Screen) entered(field *ui.Node, apply, refresh func()) {
	ui.On(field, ui.EventChange, apply)
	ui.On(field, ui.EventSubmit, apply)
	s.fields = append(s.fields, refresh)
}

// textSetting is a setting that is a text, kept as read makes of what
// is typed.
func (s *Screen) textSetting(id, label, hint string, of func(*config.Config) *string,
	read func(string) string) *ui.Node {
	field := ui.Input(ui.ID(id), ui.Value(*of(&s.config)))
	s.entered(field, func() {
		typed := s.doc.Value(field)
		s.configure(func(c *config.Config) {
			if *of(c) = typed; typed != "" {
				*of(c) = read(typed)
			}
		})
	}, func() { s.doc.SetValue(field, *of(&s.config)) })
	return setting(id, label, hint, field)
}

// numberSetting is a setting that is a whole number between low and
// high.
func (s *Screen) numberSetting(id, label, hint string, low, high int, of func(*config.Config) *int) *ui.Node {
	field := ui.Input(ui.ID(id), ui.Type("number"), ui.Attr("min", strconv.Itoa(low)),
		ui.Attr("max", strconv.Itoa(high)), ui.Value(strconv.Itoa(*of(&s.config))))
	s.entered(field, func() {
		typed, err := strconv.Atoi(s.doc.Value(field))
		if err != nil {
			typed = low - 1
		}
		s.configure(func(c *config.Config) { *of(c) = typed })
	}, func() { s.doc.SetValue(field, strconv.Itoa(*of(&s.config))) })
	return setting(id, label, fmt.Sprintf("%s %d to %d.", hint, low, high), field)
}

// checkSetting is a setting that is on or off.
func (s *Screen) checkSetting(id, label string, of func(*config.Config) *bool) *ui.Node {
	field := ui.Input(ui.ID(id), ui.Type("checkbox"))
	if *of(&s.config) {
		field.SetAttr("checked", "")
	}
	ui.On(field, ui.EventChange, func() {
		on := s.doc.Checked(field)
		s.configure(func(c *config.Config) { *of(c) = on })
	})
	s.fields = append(s.fields, func() { s.doc.SetChecked(field, *of(&s.config)) })
	return ui.Div(ui.Class("setting"), ui.Label(ui.Class("check"), field, ui.Text(label)))
}

// zoomSetting is the zoom of pages of the web: a slider, and what it
// stands at.
func (s *Screen) zoomSetting() *ui.Node {
	const id = "set-zoom"
	shown := ui.Span(ui.ID("zoom-shown"), ui.Text(percent(s.config.Zoom, "")))
	field := ui.Input(ui.ID(id), ui.Type("range"), ui.Attr("min", strconv.Itoa(config.MinZoom)),
		ui.Attr("max", strconv.Itoa(config.MaxZoom)), ui.Attr("step", strconv.Itoa(zoomStep)),
		ui.Value(strconv.Itoa(s.config.Zoom)))
	apply := func() {
		typed, _ := strconv.ParseFloat(s.doc.Value(field), 64)
		s.configure(func(c *config.Config) { c.Zoom = int(typed) })
	}
	ui.On(field, ui.EventInput, apply)
	s.entered(field, apply, func() {
		s.doc.SetValue(field, strconv.Itoa(s.config.Zoom))
		shown.SetText(percent(s.config.Zoom, ""))
	})
	return ui.Div(ui.Class("setting"), ui.Label(ui.Attr("for", id), ui.Text("Zoom of pages of the web")), field,
		shown)
}
