package app

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/assist"
	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
)

const (
	// assistFile is the file of the features the model helps with, in
	// the folder light is given.
	assistFile = "assist.json"
	// modelName is what the model is called.
	modelName = "Model on this device"
	// turnOn is what turns it on, in the menu and in the settings.
	turnOn    = "Turn on a model on this device…"
	cardNotch = 15
)

// feature is something of light's the model helps with, which its card
// turns on and off.
type feature struct {
	id, label string
}

// modelCard is the model's card and its marks: the robot of the header,
// there only while the model is on, the menu's rows, and the card that
// proposes a model, shows the one that runs and changes it.
type modelCard struct {
	root, slot, robot, dot, rows, state, ask *ui.Node
	// seq is the model's status as the marks show it, drawn whether they
	// were ever shown, and listed the rows the menu has: none, the one
	// that turns the model on, the one of a model that is on.
	seq    uint64
	drawn  bool
	listed string
	// pick is the model picked and file the file named, neither agreed
	// to yet; owning whether the field of a file shows.
	pick, file string
	owning     bool
	// problem is why the last thing asked did not happen.
	problem string
	// reading is whether a file is being looked into, by a model its
	// file turned on when revert; read gives what was found.
	reading, revert bool
	read            chan inspection
	typing          bool
	// unembedded is whether the proposal's line for the model that
	// embeds text was unticked, and embedAsked whether the card asks
	// before downloading it for a model that is on.
	unembedded, embedAsked bool
}

// inspection is what the model found in a file of the user's.
type inspection struct {
	file model.File
	err  error
}

// modelGiven reports whether this light was given a model at all, and
// modelOn whether its user turned it on.
func (p *Profile) modelGiven() bool { return p.opts.Model != nil && p.opts.Library != nil }
func (p *Profile) modelOn() bool    { return p.modelGiven() && p.config.Model }

// helps reports whether the model helps with the feature id: it is on,
// and the feature was not turned off.
func (p *Profile) helps(id string) bool { return p.modelOn() && p.assists.On(id) }

// sized is a size in bytes as a download's is said.
func sized(bytes int64) string {
	const mega, giga = 1e6, 1e9
	if bytes >= giga {
		return fmt.Sprintf("%.1f GB", float64(bytes)/giga)
	}
	return fmt.Sprintf("%.0f MB", float64(bytes)/mega)
}

// modelMark makes the place of the header's robot, and with it the
// card, closed, and the place of the menu's rows.
func (s *Screen) modelMark() *ui.Node {
	c := &s.card
	c.read = make(chan inspection, 1)
	c.dot = ui.Img(ui.ID("robot-dot"), ui.Attr("alt", ""))
	c.robot = ui.Button(ui.ID("robot"), glyph("robot"), c.dot, ui.OnClick(func() {
		if s.cardOpen() {
			s.closeCard(false)
			return
		}
		s.openCard()
	}))
	c.slot = ui.Div(ui.ID("model-mark"))
	c.rows = ui.Div(ui.ID("menu-model-rows"), ui.Class("group"))
	c.state = ui.Span(ui.ID("menu-model-state"), ui.Class("keys"))
	c.root = ui.Div(ui.ID("model"), ui.Attr("role", "dialog"), ui.Attr("aria-label", modelName),
		ui.OnBack(func(e ui.Event) {
			s.closeCard(true)
			e.StopPropagation()
		}))
	return c.slot
}

func (s *Screen) cardOpen() bool { return s.card.root.HasClass("shown") }

// openCard shows the card: the model that runs, or the one proposed.
func (s *Screen) openCard() {
	if !s.modelGiven() {
		return
	}
	s.menu.Close()
	s.closePanel(false)
	s.closeChoices()
	s.closeSheets(nil)
	s.list.RemoveClass("shown")
	s.recent.RemoveClass("shown")
	c := &s.card
	c.pick, c.file, c.owning, c.problem = "", "", false, ""
	c.root.AddClass("shown")
	s.fillCard()
	s.hang(c.root, s.cardAnchor(), cardNotch)
	if first := s.doc.ByID("model-done"); first != nil {
		s.doc.Focus(first)
	} else if first = s.doc.ByID("model-later"); first != nil {
		s.doc.Focus(first)
	}
}

// closeCard closes the card; refocus gives what it hangs from the focus.
func (s *Screen) closeCard(refocus bool) {
	if !s.cardOpen() {
		return
	}
	s.card.root.RemoveClass("shown")
	s.card.typing = false
	empty(s.card.root)
	if refocus {
		s.doc.Focus(s.cardAnchor())
	}
}

// cardAnchor is what the card hangs from: the robot, and the menu's
// button while the model is off and the header has none.
func (s *Screen) cardAnchor() *ui.Node {
	if s.modelOn() {
		return s.card.robot
	}
	return s.more
}

// entry is the catalogue's entry id.
func (s *Screen) entry(id string) (model.Entry, bool) {
	entries := s.opts.Library.Catalogue()
	i := slices.IndexFunc(entries, func(e model.Entry) bool { return e.ID == id })
	if i < 0 {
		return model.Entry{}, false
	}
	return entries[i], true
}

// proposed is the model light proposes for this device.
func (s *Screen) proposed() string {
	e, _ := s.opts.Library.Recommended()
	return e.ID
}

// chosen is the model the settings name: the one proposed when they
// name none, and none when they name a file.
func (s *Screen) chosen() string {
	switch {
	case s.config.ModelFile != "":
		return ""
	case s.config.ModelID != "":
		return s.config.ModelID
	}
	return s.proposed()
}

// there reports whether an entry and the library that runs it are on
// this device.
func (s *Screen) there(e model.Entry) bool {
	library, _ := s.entry(model.RuntimeID)
	return e.Install.State == model.Installed && library.Install.State == model.Installed
}

// coming are the entries being downloaded for the model id.
func (s *Screen) coming(id string) []model.Entry {
	var out []model.Entry
	embeds, _ := s.embedder()
	for _, of := range []string{id, model.RuntimeID, embeds.ID} {
		if e, ok := s.entry(of); ok && e.Install.State == model.Installing && !slices.ContainsFunc(out,
			func(o model.Entry) bool { return o.ID == e.ID }) {
			out = append(out, e)
		}
	}
	return out
}

// embedder is the model that embeds text, which finding by meaning and
// asking the history need, and whether it is still to download.
func (s *Screen) embedder() (model.Entry, bool) {
	for _, e := range s.opts.Library.Catalogue() {
		if e.Kind == model.KindEmbedding {
			return e, e.Install.State != model.Installed
		}
	}
	return model.Entry{}, false
}

// embeds reports whether the model embeds text.
func (p *Profile) embeds() bool {
	return p.modelOn() && p.opts.Model.Status().Capabilities.Embeddings
}

// alsoEmbedder is the proposal's line for the model that embeds text:
// ticked unless its reader unticks it, downloaded only once the
// proposal is agreed to, and then the total to download.
func (s *Screen) alsoEmbedder(block *ui.Node, e model.Entry) {
	embeds, missing := s.embedder()
	if !missing {
		return
	}
	c := &s.card
	var total int64
	library, _ := s.entry(model.RuntimeID)
	for _, part := range []model.Entry{e, library} {
		if part.Install.State != model.Installed {
			total += part.Size
		}
	}
	if !c.unembedded {
		total += embeds.Size
	}
	block.Append(
		ui.Button(ui.ID("model-embed"), ui.Attr("role", "checkbox"), ui.Attr("aria-checked", onOff(!c.unembedded)),
			ui.Span(ui.Class("box"), glyph("check")), ui.Span(ui.Text("Also download "+embeds.Name+" ("+
				sized(embeds.Size)+", "+embeds.Licence+", from "+host(embeds.Source)+
				"): needed to find by meaning and to ask your history")),
			ui.OnClick(func() {
				c.unembedded = !c.unembedded
				s.fillCard()
			})),
		ui.P(ui.ID("model-total"), ui.Text("To download: "+sized(total))))
}

// embedderOffer is what a model that is on says of the model that
// embeds text while it is not on the device: what needs it, the one
// action that downloads it, and the question asked before it does.
func (s *Screen) embedderOffer() *ui.Node {
	embeds, missing := s.embedder()
	if !missing || embeds.Install.State == model.Installing {
		return nil
	}
	c := &s.card
	offer := ui.Div(ui.ID("model-embed-need"))
	if !c.embedAsked {
		offer.Append(ui.P(ui.Text("Find by meaning and asking your history need "+embeds.Name+
			", which is not on this device.")), ui.Button(ui.ID("model-embed-get"),
			ui.Text("Download "+embeds.Name+" ("+sized(embeds.Size)+")"), ui.OnClick(func() {
				c.embedAsked = true
				s.fillCard()
			})))
		return offer
	}
	offer.Append(
		ui.P(ui.ID("model-embed-ask"), ui.Strong(ui.Text(embeds.Name)), ui.Text(" · "+sized(embeds.Size)+" · "+
			embeds.Licence+". Downloaded from "+host(embeds.Source)+" once you agree.")),
		ui.Div(ui.Class("asked"),
			ui.Button(ui.ID("model-embed-later"), ui.Text("Not now"), ui.OnClick(func() {
				c.embedAsked = false
				s.fillCard()
			})),
			ui.Button(ui.ID("model-embed-confirm"), ui.Class("agree"), ui.Text("Download"), ui.OnClick(func() {
				c.embedAsked = false
				if err := s.opts.Library.Install(embeds.ID); err != nil {
					c.problem = err.Error()
				}
				s.redrawModel()
				s.fillCard()
			}))))
	return offer
}

// standing is what the model does, as a class of the robot and as a
// word.
func (s *Screen) standing(status model.Status) (string, string) {
	switch status.State {
	case model.Ready:
		return "ready", "Ready"
	case model.Working:
		return "working", "Working"
	case model.Preparing:
		return "preparing", "Preparing"
	case model.Failed:
		return "failed", "Failed"
	case model.Off, model.Missing:
	}
	if len(s.coming(s.chosen())) > 0 {
		return "downloading", "Downloading"
	}
	return "missing", "Not installed"
}

// watchModel shows what changed of the model since the last frame: the
// robot, the menu's rows and the open card, and what a file looked into
// turned out to be.
func (s *Screen) watchModel() {
	if !s.modelGiven() {
		return
	}
	c := &s.card
	select {
	case found := <-c.read:
		s.fileRead(found)
	default:
	}
	status := s.opts.Model.Status()
	if c.drawn && status.Seq == c.seq {
		return
	}
	c.seq, c.drawn = status.Seq, true
	s.showModel(status)
}

func (s *Screen) showModel(status model.Status) {
	c := &s.card
	on := s.modelOn()
	class, word := s.standing(status)
	if on {
		for _, old := range slices.Clone(c.robot.Classes()) {
			c.robot.RemoveClass(old)
		}
		c.robot.AddClass(class)
		c.robot.SetAttr("aria-label", modelName+": "+strings.ToLower(word))
		c.dot.SetAttr("src", "robot-"+class+".svg")
		c.state.SetText(word)
	}
	if listed := fmt.Sprint(on, s.helps(answersFeature), s.helps(sortingFeature)); listed != c.listed {
		c.listed, c.ask = listed, nil
		empty(c.slot)
		empty(c.rows)
		toggle(c.slot, "shown", on)
		row := s.choice("model-on", turnOn, "", s.openCard)
		if on {
			c.slot.Append(c.robot)
			row = s.choice("model", modelName, "", s.openCard)
			row.Append(c.state)
		}
		if s.helps(answersFeature) {
			c.ask = s.choice("ask", "Ask about this page", "", s.askAbout)
			c.rows.Append(c.ask)
		}
		if s.helps(sortingFeature) {
			c.rows.Append(s.choice("sorted", sortedTitle, "", s.openSorting))
		}
		c.rows.Append(row, ui.Div(ui.Attr("role", "separator")))
	}
	if s.cardOpen() {
		s.fillCard()
	}
}

// redrawModel has every window show the model again at its next frame.
func (p *Profile) redrawModel() {
	for _, s := range p.screens {
		s.card.drawn = false
	}
}

// fillCard writes the open card again, the focus staying where it was.
func (s *Screen) fillCard() {
	c := &s.card
	focused := ""
	c.root.Walk(func(n *ui.Node) bool {
		if s.doc.Focused(n) {
			focused = n.ID()
		}
		return focused == ""
	})
	empty(c.root)
	status := s.opts.Model.Status()
	on := s.modelOn()
	title, sub := "A model on this device", "Off. Nothing is downloaded or started until you agree."
	if on {
		title, sub = s.cardTitle(status), s.running(status)
	}
	head := ui.Div(ui.Class("head"), ui.Span(ui.Class("site", "big", "robot"), glyph("robot-tile")),
		ui.Div(ui.Class("named"), ui.Strong(ui.ID("model-title"), ui.Text(title)),
			ui.Span(ui.ID("model-sub"), ui.Text(sub))))
	if on {
		head.Append(ui.Button(ui.ID("model-switch"), ui.Attr("role", "switch"), ui.Attr("aria-checked", "true"),
			ui.Attr("aria-label", modelName), ui.Span(glyph("check-dark")), ui.OnClick(func() {
				s.closeCard(false)
				s.configure(func(c *config.Config) { c.Model = false })
			})))
	}
	c.root.Append(ui.Span(ui.Class("notch")), head, ui.Div(ui.Class("part"), ui.Span(ui.Text("Model")), s.modelChips()))
	if c.owning {
		c.root.Append(s.fileField())
	}
	target, proposing := s.offered()
	settled := true
	if coming := s.coming(target.ID); len(coming) > 0 && c.file == "" {
		c.root.Append(s.progressOf(coming))
		settled = false
	} else if proposing {
		c.root.Append(s.proposal(target))
		settled = false
	}
	problem := c.problem
	if problem == "" && target.Install.State == model.InstallFailed {
		problem = "The download stopped: " + target.Install.Err + "."
	}
	if problem == "" && on && status.State == model.Failed {
		problem = status.Reason
	}
	if problem != "" {
		c.root.Append(ui.P(ui.ID("model-problem"), ui.Attr("role", "alert"), ui.Text(problem)))
	}
	if on && settled && len(s.features) > 0 {
		c.root.Append(ui.Div(ui.Class("part"), ui.Span(ui.Text("It helps with")), s.helpChips()))
		if offer := s.embedderOffer(); offer != nil {
			c.root.Append(offer)
		}
		if offer := s.forgetOffer("model-recall"); offer != nil {
			c.root.Append(offer)
		}
	}
	c.root.Append(ui.P(ui.ID("model-note"), ui.Text("What pages say stays on this device.")),
		s.cardFoot(target, proposing))
	if n := s.doc.ByID(focused); focused != "" && n != nil {
		s.doc.Focus(n)
	}
}

// cardTitle says what a model that is on does.
func (s *Screen) cardTitle(status model.Status) string {
	switch {
	case s.card.reading:
		return "Reading your file"
	case status.State == model.Ready:
		return "Ready, on this device"
	case status.State == model.Working:
		return "Working, on this device"
	case status.State == model.Preparing:
		return "Preparing"
	case status.State == model.Failed:
		return "It cannot run"
	case len(s.coming(s.chosen())) > 0:
		return "Downloading"
	}
	return "Not on this device yet"
}

// running says which model runs, how big it is and what computes it.
func (s *Screen) running(status model.Status) string {
	parts := []string{s.fileName(status), "your own file"}
	if e, ok := s.entry(s.chosen()); ok {
		parts = []string{e.Name, sized(e.Size)}
	}
	if s.config.ModelCPU {
		return strings.Join(append(parts, "processor"), " · ")
	}
	return strings.Join(append(parts, "graphics card"), " · ")
}

// fileName names the file of the user's the settings name: as the model
// read it, else as its folder does.
func (s *Screen) fileName(status model.Status) string {
	if status.Chosen.File == s.config.ModelFile && status.Chosen.Name != "" {
		return status.Chosen.Name
	}
	return filepath.Base(s.config.ModelFile)
}

// modelChips are the models to choose from as chips: those that chat, the
// one in use marked by a check, then a file of the user's own.
func (s *Screen) modelChips() *ui.Node {
	c := &s.card
	chips := ui.Div(ui.ID("model-choices"))
	current := s.chosen()
	if c.pick != "" || c.file != "" {
		current = c.pick
	}
	for _, e := range s.opts.Library.Catalogue() {
		if e.Kind != model.KindChat {
			continue
		}
		label, said := e.Name, e.Name
		if e.Recommended {
			label, said = label+" (proposed)", said+", proposed"
		}
		if s.there(e) {
			said += ", on this device"
		} else {
			said += ", " + sized(e.Size) + " to download"
		}
		chip := s.pill("model-pick-"+e.ID, label, said, e.ID == current, func() { s.pickModel(e) })
		if !s.there(e) {
			chip.Append(glyph("fetch"))
		}
		chips.Append(chip)
	}
	label, mine := "Your own file…", c.file != "" || c.pick == "" && s.config.ModelFile != ""
	switch {
	case c.file != "":
		label = filepath.Base(c.file)
	case mine:
		label = s.fileName(s.opts.Model.Status())
	}
	chips.Append(s.pill("model-own", label, "A model file of your own", mine, func() {
		c.owning, c.typing, c.problem = true, false, ""
		s.fillCard()
		s.doc.Focus(s.doc.ByID("model-file"))
	}))
	return chips
}

// pill is a chip that is chosen or not, with a check when it is.
func (s *Screen) pill(id, label, said string, chosen bool, click func()) *ui.Node {
	chip := ui.Button(ui.ID(id), ui.Class("folder"), ui.Attr("aria-pressed", strconv.FormatBool(chosen)),
		ui.Attr("aria-label", said), ui.OnClick(click))
	if chosen {
		chip.Append(glyph("check"))
	}
	chip.Append(ui.Span(ui.Text(label)))
	return chip
}

// pickModel takes a model of the catalogue: used at once when it is on
// this device and the model is on, proposed otherwise.
func (s *Screen) pickModel(e model.Entry) {
	c := &s.card
	c.file, c.owning, c.problem, c.pick = "", false, "", ""
	switch {
	case s.modelOn() && s.there(e):
		s.use(e.ID)
	case e.ID != s.chosen():
		c.pick = e.ID
	}
	s.fillCard()
}

// use has the settings name the model id, on.
func (s *Screen) use(id string) {
	if id == s.proposed() {
		id = ""
	}
	s.configure(func(c *config.Config) { c.Model, c.ModelID, c.ModelFile = true, id, "" })
}

// fileField is where the path of a file of the user's is typed.
func (s *Screen) fileField() *ui.Node {
	c := &s.card
	field := ui.Input(ui.ID("model-file"), ui.Placeholder("/path/to/a/model.gguf"),
		ui.Attr("aria-label", "The path of a model file"))
	ui.OnSubmit(func() {
		if path := strings.TrimSpace(s.doc.Value(field)); path != "" {
			c.file, c.pick = path, ""
		}
		c.owning, c.typing = false, false
		s.fillCard()
		s.doc.Focus(s.doc.ByID("model-confirm"))
	}).Apply(field)
	return ui.Div(ui.Class("part"), field)
}

// offered is the model the card's button is about, and whether there is
// something to agree to: the file named, the model picked, or the one
// chosen when it is off or not on this device.
func (s *Screen) offered() (model.Entry, bool) {
	c := &s.card
	if c.file != "" {
		return model.Entry{}, true
	}
	if c.pick != "" {
		e, _ := s.entry(c.pick)
		return e, true
	}
	e, known := s.entry(s.chosen())
	if !known {
		return e, !s.modelOn()
	}
	return e, !s.modelOn() || !s.there(e)
}

func host(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return address
	}
	return u.Host
}

// proposal says what agreeing does: which model, how big, under which
// licence, downloaded from where, with what else.
func (s *Screen) proposal(e model.Entry) *ui.Node {
	block := ui.Div(ui.ID("model-proposal"))
	if file := s.card.file; file != "" {
		block.Append(ui.P(ui.Strong(ui.Text(file))), ui.P(ui.Text("A file of your own: light reads what it is once "+
			"you agree, and never copies or deletes it. Nothing is downloaded.")))
		return block
	}
	if e.ID == "" {
		block.Append(ui.P(ui.Strong(ui.Text(s.fileName(s.opts.Model.Status()))), ui.Text(" · your own file")))
		return block
	}
	block.Append(ui.P(ui.Strong(ui.Text(e.Name)), ui.Text(" · "+sized(e.Size)+" · "+e.Licence)))
	library, _ := s.entry(model.RuntimeID)
	from := "Downloaded from " + host(e.Source)
	switch {
	case s.there(e):
		from = "It is already on this device: nothing is downloaded"
	case e.Install.State == model.Installed:
		from = "It is already on this device"
	}
	if library.Install.State != model.Installed {
		from += ", with " + library.Name + " (" + sized(library.Size) + ", " + library.Licence + "), which runs it, from " +
			host(library.Source)
	}
	block.Append(ui.P(ui.Text(from + ".")))
	if e.Why != "" {
		block.Append(ui.P(ui.Text(e.Why)))
	}
	s.alsoEmbedder(block, e)
	return block
}

// progressOf shows how far the downloads are, and what stops them.
func (s *Screen) progressOf(coming []model.Entry) *ui.Node {
	var done, total int64
	for _, e := range coming {
		done, total = done+e.Install.Done, total+e.Install.Total
	}
	part := 0
	if total > 0 {
		part = int(done * 100 / total)
	}
	return ui.Div(ui.ID("model-progress"),
		ui.Progress(ui.Attr("value", strconv.Itoa(part)), ui.Attr("max", "100"), ui.Attr("aria-label", "Download")),
		ui.Span(ui.ID("model-done-of"), ui.Text(percent(part, "")+" of "+sized(total))),
		ui.Button(ui.ID("model-cancel"), ui.Text("Cancel"), ui.OnClick(func() {
			for _, e := range coming {
				s.opts.Library.CancelInstall(e.ID)
			}
			s.redrawModel()
			s.fillCard()
		})))
}

// helpChips are the features the model helps with, each turned off and
// on by its chip.
func (s *Screen) helpChips() *ui.Node {
	chips := ui.Div(ui.ID("model-helps"))
	for _, f := range s.features {
		on := s.assists.On(f.id)
		said := f.label
		embeds, missing := s.embedder()
		if missing = missing && (f.id == findFeature || f.id == recallFeature); missing {
			said += ": needs " + embeds.Name
		}
		chips.Append(s.pill("model-helps-"+f.id, f.label, said, on, func() {
			s.card.embedAsked = s.card.embedAsked || missing && !on
			s.assists.Set(f.id, !on)
			s.assisted.touch()
			s.assistChanged(f.id)
			s.redrawModel()
			s.fillCard()
		}))
	}
	return chips
}

// cardFoot is the card's last row: what leaves the model as it is, then
// the one button that agrees, or Done.
func (s *Screen) cardFoot(target model.Entry, proposing bool) *ui.Node {
	on := s.modelOn()
	foot := ui.Div(ui.Class("foot"))
	if on {
		foot.Append(ui.Button(ui.ID("model-delete"), ui.Text("Turn off and delete the model"), ui.OnClick(s.deleteModel)))
	} else {
		foot.Append(ui.Button(ui.ID("model-later"), ui.Text("Not now"), ui.OnClick(func() { s.closeCard(true) })))
	}
	switch {
	case len(s.coming(target.ID)) > 0 && s.card.file == "" || s.card.reading:
		if !on {
			return foot
		}
	case proposing:
		foot.Append(ui.Button(ui.ID("model-confirm"), ui.Class("agree"), ui.Text(s.agreeing(target)),
			ui.OnClick(s.agree)))
		return foot
	}
	foot.Append(ui.Button(ui.ID("model-done"), ui.Class("agree"), ui.Text("Done"),
		ui.OnClick(func() { s.closeCard(true) })))
	return foot
}

// agreeing is what the card's button reads.
func (s *Screen) agreeing(target model.Entry) string {
	on := s.modelOn()
	switch {
	case s.card.file != "" && on:
		return "Use this file"
	case s.card.file != "":
		return "Turn on with this file"
	case target.Install.State == model.InstallFailed:
		return "Try again"
	case target.ID == "" || s.there(target):
		return "Turn on"
	case !on:
		return "Download and turn on"
	case s.card.pick != "":
		return "Download and use"
	}
	return "Download"
}

// agree does what the card proposes: the model is turned on with the
// model picked, which is downloaded when it is not on this device, or a
// file named is looked into.
func (s *Screen) agree() {
	c := &s.card
	c.problem = ""
	if c.file != "" {
		s.readFile(c.file)
		return
	}
	target, _ := s.offered()
	c.pick = ""
	if target.ID == "" {
		s.configure(func(c *config.Config) { c.Model = true })
	} else {
		s.use(target.ID)
		if !s.there(target) {
			if err := s.opts.Library.Install(target.ID); err != nil {
				c.problem = err.Error()
			}
		}
		s.fetchEmbedder()
	}
	s.redrawModel()
	s.watchModel()
}

// fetchEmbedder downloads the model that embeds text with a model
// agreed to, unless its line was unticked.
func (s *Screen) fetchEmbedder() {
	c := &s.card
	embeds, missing := s.embedder()
	if !missing || c.unembedded || c.problem != "" {
		return
	}
	if err := s.opts.Library.Install(embeds.ID); err != nil {
		c.problem = err.Error()
	}
}

// readFile turns the model on, which looking into a file needs, and
// has it look into the file at path off the frame.
func (s *Screen) readFile(path string) {
	c := &s.card
	c.file, c.reading, c.revert = "", true, !s.modelOn()
	s.configure(func(c *config.Config) { c.Model = true })
	library := s.opts.Library
	go func() {
		file, err := library.Inspect(context.Background(), path)
		file.Path = path
		c.read <- inspection{file, err}
	}()
	s.fillCard()
}

// fileRead takes what a file turned out to be: the model from now on,
// or why not, the model off again when the file had turned it on.
func (s *Screen) fileRead(found inspection) {
	c := &s.card
	c.reading = false
	if found.err != nil {
		c.problem = found.err.Error()
		if c.revert {
			s.configure(func(c *config.Config) { c.Model = false })
		}
	} else {
		s.configure(func(c *config.Config) { c.Model, c.ModelID, c.ModelFile = true, "", found.file.Path })
	}
	s.redrawModel()
}

// deleteModel turns the model off and removes everything light
// downloaded for it; a file of the user's own stays.
func (s *Screen) deleteModel() {
	s.configure(func(c *config.Config) { c.Model, c.ModelID, c.ModelFile = false, "", "" })
	if err := s.opts.Library.DeleteAll(); err != nil {
		s.card.problem = err.Error()
		s.redrawModel()
		s.fillCard()
		return
	}
	s.closeCard(false)
	s.redrawModel()
}

// followCard keeps the open card under what it hangs from, and closes
// it at a press on anything else and at Escape, as the bookmark's panel
// is (followPanel).
func (s *Screen) followCard() {
	if !s.cardOpen() {
		return
	}
	c := &s.card
	anchor := s.cardAnchor()
	s.hang(c.root, anchor, cardNotch)
	in := s.win.Input()
	pointing := in.Down(input.MouseLeft) || in.Released(input.MouseLeft)
	typing := s.doc.Focused(s.doc.ByID("model-file"))
	if in.Pressed(input.KeyEscape) || c.typing && !typing && !pointing && !s.focusedIn(s.doc.Body()) {
		if c.typing = false; c.owning {
			c.owning = false
			s.fillCard()
			return
		}
		s.closeCard(true)
		return
	}
	if c.typing = typing; !in.Pressed(input.MouseLeft) {
		return
	}
	for _, kept := range []*ui.Node{c.root, anchor} {
		if r, ok := s.doc.BoxOf(kept); ok && r.Contains(in.Mouse()) {
			return
		}
	}
	s.closeCard(false)
}

// modelSettings is the settings' part of the model: what turns it on
// while it is off, and how it runs once it is on.
func (s *Screen) modelSettings() *ui.Node {
	running := ui.Input(ui.ID("set-model"), ui.Type("checkbox"), ui.Attr("checked", ""))
	ui.On(running, ui.EventChange, func() {
		if !s.doc.Checked(running) {
			s.configure(func(c *config.Config) { c.Model = false })
		}
	})
	on := ui.Div(
		ui.Div(ui.Class("setting"), ui.Label(ui.Class("check"), running, ui.Text("Run a model on this device"))),
		ui.Div(ui.Class("setting"), ui.Button(ui.ID("set-model-choose"), ui.Text("Choose the model…"),
			ui.OnClick(s.openCard))),
		s.numberSetting("set-model-threads", "Threads", "What it computes on; 0 is light's own choice.", 0,
			config.MaxModelThreads, func(c *config.Config) *int { return &c.ModelThreads }),
		s.checkSetting("set-model-cpu", "Leave the graphics card out of it",
			func(c *config.Config) *bool { return &c.ModelCPU }),
		ui.P(ui.Class("hint"), ui.Text("What pages say stays on this device.")))
	embedded := ui.P(ui.ID("set-model-embed"), ui.Class("hint"))
	on.Append(embedded)
	off := ui.Div(
		ui.P(ui.Class("hint"), ui.Text("Off: nothing of it is downloaded or started until you turn it on and agree "+
			"to what it proposes.")),
		ui.Div(ui.Class("setting"), ui.Button(ui.ID("set-model-on"), ui.Text(turnOn), ui.OnClick(s.openCard))))
	held := ui.Div(ui.ID("model-settings"))
	show := func() {
		shown := off
		if s.modelOn() {
			shown = on
			s.doc.SetChecked(running, true)
			embeds, missing := s.embedder()
			said := embeds.Name + ", which finding by meaning and asking your history need, is on this device."
			if missing {
				said = embeds.Name + ", which finding by meaning and asking your history need, is not on this " +
					"device: the model's card downloads it."
			}
			if embeds.ID == "" {
				said = ""
			}
			if embedded.Text() != said {
				embedded.SetText(said)
			}
		}
		if len(held.Children()) == 0 || held.Children()[0] != shown {
			empty(held)
			held.Append(shown)
		}
	}
	s.fields = append(s.fields, show)
	show()
	return held
}

// assistFeatures reads the features the model helps with from the
// folder light was given.
func (p *Profile) assistFeatures() {
	p.assists = assist.New()
	p.assisted = &store{name: assistFile, write: func(path string) error { return assist.Save(path, p.assists) }}
	if p.opts.Dir == "" {
		return
	}
	var err error
	if p.assists, err = assist.Load(p.assisted.path(p.opts.Dir)); p.unreadable == nil {
		p.unreadable = err
	}
}
