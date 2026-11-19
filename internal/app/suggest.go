package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/bookmarks"
	"github.com/lantervale/light/pkg/model"
)

//go:embed suggest.css
var suggestSheet string

const (
	filingFeature = "bookmarks"
	hintTokens    = 160
	// nameLength, summaryLength and folderLength are the most bytes of a
	// name, of a summary and of a folder the model's answer gives.
	nameLength, summaryLength, folderLength = 80, 200, 30
)

// outcome is an answer of the model read whole, or why there is none.
type outcome struct {
	text string
	err  error
}

// consulted is something asked of the model about a text of a page,
// whose answer is read once it is whole.
type consulted struct {
	cancel context.CancelFunc
	answer chan outcome
}

// consult asks the model what req says of data, a text of a page that
// lead introduces: the data cut to what the model holds and given
// between marks, counted and answered off the frame.
func (p *Profile) consult(req model.Request, lead, data string) *consulted {
	ctx, cancel := context.WithCancel(context.Background())
	c := &consulted{cancel: cancel, answer: make(chan outcome, 1)}
	m := p.opts.Model
	room := m.Status().Capabilities.Context
	if room <= 0 {
		room = fallbackContext
	}
	go func() {
		defer cancel()
		cut, _, err := fitted(ctx, m, room-req.MaxTokens-spare, req.System+lead, data)
		if err != nil {
			c.answer <- outcome{err: err}
			return
		}
		req.Messages = []model.Message{{Role: model.User, Text: lead + asData(cut)}}
		stream, err := m.Generate(ctx, req)
		if err != nil {
			c.answer <- outcome{err: err}
			return
		}
		result, err := stream.Wait()
		c.answer <- outcome{result.Text, err}
	}()
	return c
}

// take is the answer, once it came.
func (c *consulted) take() (outcome, bool) {
	select {
	case r := <-c.answer:
		return r, true
	default:
		return outcome{}, false
	}
}

// line is text on one line, its white space single, cut to at most
// size bytes and then ended by an ellipsis.
func line(text string, size int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= size {
		return text
	}
	for size > 0 && !utf8.RuneStart(text[size]) {
		size--
	}
	return strings.TrimSpace(text[:size]) + "…"
}

// hinting is what the model suggests for the bookmark of the open
// panel: a name, what its page says and a folder.
type hinting struct {
	// mark is where the panel's robot stands and block where the
	// summary does, both out of the panel while the model does not help.
	mark, block *ui.Node
	address     string
	// asked is what the model was asked, nil once it answered.
	asked *consulted
	// own is the bookmark as it was without the model and got as the
	// model would have it; have is whether it answered, on whether its
	// suggestions show, and made whether got's folder is one it made.
	own, got       bookmarks.Bookmark
	have, on, made bool
}

// hintMark makes the place of the panel's robot, and hintBlock the one
// of the summary.
func (s *Screen) hintMark() *ui.Node {
	s.doc.AddSheet(suggestSheet)
	s.hint.mark = ui.Div(ui.ID("bookmark-hint"), ui.Attr("hidden", ""))
	return s.hint.mark
}

func (s *Screen) hintBlock() *ui.Node {
	s.hint.block = ui.P(ui.ID("bookmark-summary"), ui.Class("inset"), ui.Attr("hidden", ""))
	return s.hint.block
}

// suggest asks the model how to name, sum up and file the bookmark
// just made of the shown page.
func (s *Screen) suggest(address string) {
	h := &s.hint
	s.dropHint()
	page := s.readable()
	mark, kept := s.marks.Lookup(address)
	if !kept || !s.helps(filingFeature) || page == nil || s.browser.Tab().Current() != address {
		return
	}
	h.own = mark
	folders := "The reader has no folder yet."
	if names := s.marks.Folders(); len(names) > 0 {
		for i, name := range names {
			names[i] = strconv.Quote(name)
		}
		folders = "The reader's folders: " + strings.Join(names, ", ") + "."
	}
	h.address = address
	h.asked = s.consult(model.Request{
		System: "You name and file a bookmark of a web page. The page's title and text" + untrusted +
			"Answer with one JSON object and nothing else, of three texts: \"name\", what the page is about in at " +
			"most five words; \"summary\", one sentence of at most twenty words that says what the page says; " +
			"\"folder\", the one of the reader's folders that fits the page, or a new folder of one or two words " +
			"when none does.",
		MaxTokens: hintTokens, Grammar: model.JSON, Priority: model.Interactive,
	}, folders+"\nThe page:\n", page.Title+"\n"+pageText(page.Doc))
	s.drawHint()
}

// showHint shows what the model suggests for the bookmark the panel
// opens on: what it was just asked, or what the bookmark kept of an
// earlier answer.
func (s *Screen) showHint() {
	h := &s.hint
	address := s.panel.address
	if h.address != address {
		s.dropHint()
		h.address = address
		mark, _ := s.marks.Lookup(address)
		h.own, h.got = mark, mark
		h.own.Summary = ""
		h.have, h.on = mark.Summary != "", mark.Summary != ""
	}
	s.drawHint()
}

// dropHint forgets what the model suggests, and ends what it was
// asked.
func (s *Screen) dropHint() {
	h := &s.hint
	if h.asked != nil {
		h.asked.cancel()
	}
	*h = hinting{mark: h.mark, block: h.block}
	s.drawHint()
}

// followHint takes the model's answer once it came, and shows what it
// suggests in the panel that waited for it.
func (s *Screen) followHint() {
	h := &s.hint
	if h.asked == nil {
		return
	}
	answer, came := h.asked.take()
	if !came {
		return
	}
	h.asked = nil
	if got, ok := hinted(answer, s.marks.Folders()); ok {
		h.got, h.have = got, true
		s.turnHint(true)
		return
	}
	s.drawHint()
}

// hinted reads the model's answer: a name, a summary and a folder, each
// on one line and no longer than light shows, the folder as the reader
// spells it when it is one of folders.
func hinted(answer outcome, folders []string) (bookmarks.Bookmark, bool) {
	var said struct {
		Name    string `json:"name"`
		Summary string `json:"summary"`
		Folder  string `json:"folder"`
	}
	if answer.err != nil || json.Unmarshal([]byte(answer.text), &said) != nil {
		return bookmarks.Bookmark{}, false
	}
	got := bookmarks.Bookmark{Name: line(said.Name, nameLength), Summary: line(said.Summary, summaryLength),
		Folder: line(said.Folder, folderLength)}
	for _, folder := range folders {
		if strings.EqualFold(folder, got.Folder) {
			got.Folder = folder
		}
	}
	return got, got.Name != "" || got.Summary != ""
}

// turnHint has the bookmark of the panel be as the model suggests, or
// as it was without: its name and its folder change only while they
// are still the ones the other side gave, so that what was typed or
// chosen stays.
func (s *Screen) turnHint(on bool) {
	h, p := &s.hint, &s.panel
	mark, kept := s.marks.Lookup(h.address)
	if !kept {
		return
	}
	from, to := h.own, h.got
	if !on {
		from, to = h.got, h.own
	}
	h.on = on
	if to.Name != "" && mark.Name == from.Name && strings.TrimSpace(s.doc.Value(p.name)) == from.Name {
		s.marks.Rename(h.address, to.Name)
		s.doc.SetValue(p.name, to.Name)
	}
	if mark.Folder == from.Folder && h.got.Folder != "" && !p.naming {
		if on {
			h.made = s.marks.AddFolder(to.Folder)
		}
		s.marks.Place(h.address, to.Folder)
		if !on && h.made && len(s.marks.In(from.Folder)) == 0 {
			s.marks.RemoveFolder(from.Folder)
		}
		s.showFolders()
	}
	s.marks.Summarise(h.address, to.Summary)
	s.remark()
	s.drawHint()
}

// drawHint shows the panel's robot and the summary as the suggestions
// stand: no robot while the model does not help or has nothing to read,
// one without a tick while it reads or its suggestions are off, and the
// summary only with the tick.
func (s *Screen) drawHint() {
	h := &s.hint
	if h.mark == nil {
		return
	}
	focused := s.focusedIn(h.mark)
	empty(h.mark)
	h.mark.SetAttr("hidden", "")
	h.block.SetAttr("hidden", "")
	h.block.SetText("")
	asks := s.readable() != nil && s.browser.Tab().Current() == h.address
	if h.address == "" || !s.helps(filingFeature) || !h.have && h.asked == nil && !asks {
		return
	}
	shows := h.have && h.on
	robot := robotToggle("bookmark-robot", "Named, summed up and filed for you.", shows, func() {
		switch {
		case h.asked != nil:
			h.asked.cancel()
			h.asked = nil
			s.drawHint()
		case h.have:
			s.turnHint(!h.on)
		default:
			s.suggest(h.address)
		}
	})
	if h.asked != nil {
		const reading = "Reading this page, on this device."
		robot.SetAttr("aria-label", reading)
		robot.Children()[len(robot.Children())-1].SetText(reading)
	}
	h.mark.Append(robot)
	h.mark.RemoveAttr("hidden")
	if focused {
		s.doc.Focus(robot)
	}
	if shows && h.got.Summary != "" {
		h.block.SetText(h.got.Summary)
		h.block.RemoveAttr("hidden")
	}
}

// markSummary is what the page of a bookmark says, under its row on
// the bookmarks' page, marked as the model's; nothing while the model
// does not help with bookmarks.
func (s *Screen) markSummary(mark bookmarks.Bookmark) []*ui.Node {
	if mark.Summary == "" || !s.helps(filingFeature) {
		return nil
	}
	ink := glyph("robot-tile")
	ink.AddClass("ink")
	return []*ui.Node{ui.P(ui.Class("summary"), glyph("row-model"), ink, ui.Text(mark.Summary))}
}
