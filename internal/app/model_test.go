package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"
	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
)

// modelled is a light given a model, off as a new light's is, and that
// model: one whose files are all on the device.
func modelled(t *testing.T, opts Options) (*shown, *model.Fake) {
	t.Helper()
	fake := model.NewFake()
	if opts.Downloads == "" {
		opts.Downloads = t.TempDir()
	}
	opts.Model, opts.Library = fake, fake
	return showWith(t, opts), fake
}

// modelOn is a light whose model is on and ready.
func modelOn(t *testing.T) (*shown, *model.Fake) {
	t.Helper()
	s, fake := modelled(t, Options{})
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	return s, fake
}

// bare takes every file of the model off the device.
func bare(fake *model.Fake) {
	for _, e := range fake.Catalogue() {
		fake.SetInstalled(e.ID, false)
	}
}

// others are the models that chat and are not the one proposed.
func others(fake *model.Fake) []model.Entry {
	var out []model.Entry
	for _, e := range fake.Catalogue() {
		if e.Kind == model.KindChat && !e.Recommended {
			out = append(out, e)
		}
	}
	return out
}

func entryOf(t *testing.T, fake *model.Fake, id string) model.Entry {
	t.Helper()
	for _, e := range fake.Catalogue() {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("the catalogue has no %q", id)
	return model.Entry{}
}

// cardShown reports whether the model's card is open.
func (s *shown) cardShown() bool { return s.card.root.HasClass("shown") }

// pressed are the texts of the chips of the element id that are chosen.
func (s *shown) pressed(id string) string {
	s.t.Helper()
	var out []string
	for _, chip := range s.node(id).Children() {
		if on, _ := chip.Attr("aria-pressed"); on == "true" {
			out = append(out, chip.Text())
		}
	}
	return strings.Join(out, ", ")
}

// nothingOfTheModel fails when the screen shows anything of a model.
func (s *shown) nothingOfTheModel() {
	s.t.Helper()
	for _, id := range []string{"robot", "menu-model", "model-title", "set-model", "set-model-threads"} {
		if s.doc.ByID(id) != nil {
			s.t.Errorf("with the model off the screen has #%s", id)
		}
	}
	s.doc.Body().Walk(func(n *ui.Node) bool {
		if src, _ := n.Attr("src"); strings.Contains(src, "robot") && s.doc.ByID("model-title") == nil {
			s.t.Errorf("with the model off the screen shows %s in %s", src, n.Parent().ID())
		}
		return true
	})
}

func TestALightGivenNoModelShowsNothingOfOne(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, configFile), []byte(`{"model": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := showWith(t, Options{Dir: dir, Downloads: t.TempDir()})
	s.nothingOfTheModel()
	s.click("menu")
	if got := strings.Join(s.rows(), "\n"); strings.Contains(strings.ToLower(got), "model") {
		t.Fatalf("the menu of a light with no model lists\n%s", got)
	}
	s.press(input.KeyEscape)
	s.settings()
	if text := strings.ToLower(s.node("page").Text()); strings.Contains(text, "model on this device") ||
		s.doc.ByID("model-settings") != nil || s.doc.ByID("set-model-on") != nil {
		t.Fatal("the settings of a light with no model speak of one")
	}
	s.nothingOfTheModel()
	if s.helps("answers") {
		t.Fatal("a light with no model has it help")
	}
}

func TestAModelThatIsOffShowsOnlyTheExperimentalSettingThatTurnsItOn(t *testing.T) {
	s, fake := modelled(t, Options{})
	s.nothingOfTheModel()
	if fake.Settings().On || fake.Status().State != model.Off || fake.Warmed() != 0 || len(fake.Asked()) != 0 {
		t.Fatalf("a new light's model is %v, asked %d times", fake.Status().State, len(fake.Asked()))
	}
	s.click("menu")
	rows := s.rows()
	if strings.Contains(strings.Join(rows, "\n"), "odel") || s.doc.ByID("menu-model-on") != nil {
		t.Fatalf("the menu lists\n%s", strings.Join(rows, "\n"))
	}
	s.clean()
	s.press(input.KeyEscape)
	s.settings()
	if s.doc.ByID("set-model-on") == nil || s.node("set-model-on").Text() != "Turn on a model on this device…" {
		t.Fatal("the settings have no button that turns a model on")
	}
	text := s.node("page").Text()
	if at := strings.LastIndex(text, "Experimental"); at < 0 || at > strings.Index(text, "Turn on a model") ||
		strings.Index(text, "Requests kept") > at {
		t.Fatalf("the model is not under Experimental, last in the settings:\n%s", text)
	}
	s.nothingOfTheModel()
	s.clean()
	if s.helps("answers") {
		t.Fatal("a model that is off helps")
	}
}

func TestTurningTheModelOnProposesOneAndDownloadsNothingBeforeItIsAgreedTo(t *testing.T) {
	s, fake := modelled(t, Options{})
	bare(fake)
	fake.Manual = true
	proposed, _ := fake.Recommended()
	library, other := entryOf(t, fake, model.RuntimeID), others(fake)[0]
	s.turnsOn()
	if !s.cardShown() || s.choosing() {
		t.Fatalf("the row left the card open %v, the menu open %v", s.cardShown(), s.choosing())
	}
	card, button := s.box("model"), s.box("menu")
	if notch := s.card.root.Children()[0].Box().Center().X; notch < button.X || notch > button.X+button.W ||
		card.Y < button.Y+button.H {
		t.Fatalf("the card %v does not hang from the menu's button %v", card, button)
	}
	said := s.node("model").Text()
	host := func(e model.Entry) string {
		u, _ := url.Parse(e.Source)
		return u.Host
	}
	for _, want := range []string{"A model on this device", proposed.Name, sized(proposed.Size), proposed.Licence,
		host(proposed), library.Name, sized(library.Size), host(library), "Nothing is downloaded or started until you agree",
		"What pages say stays on this device."} {
		if !strings.Contains(said, want) {
			t.Errorf("the card does not say %q in %q", want, said)
		}
	}
	if got := s.pressed("model-choices"); got != proposed.Name+" (proposed)" {
		t.Fatalf("the model chosen is %q", got)
	}
	if s.doc.ByID("model-switch") != nil || s.doc.ByID("model-delete") != nil || s.doc.ByID("model-done") != nil {
		t.Fatal("the proposal has what a model that is on has")
	}
	s.clean()
	idle := func(when string) {
		t.Helper()
		for _, e := range fake.Catalogue() {
			if e.Install.State != model.NotInstalled {
				t.Fatalf("%s, %s is %v", when, e.Name, e.Install.State)
			}
		}
		if s.config.Model || fake.Settings().On || fake.Status().State != model.Off {
			t.Fatalf("%s, the model is on", when)
		}
	}
	idle("before an answer")
	s.click("model-later")
	if idle("after Not now"); s.cardShown() {
		t.Fatal("Not now leaves the card open")
	}
	s.nothingOfTheModel()

	s.turnsOn()
	s.click("model-pick-" + other.ID)
	if picked := s.node("model-proposal").Text(); !strings.Contains(picked, other.Name) ||
		!strings.Contains(picked, sized(other.Size)) {
		t.Fatalf("with another model picked the proposal says %q", picked)
	}
	idle("after another model was picked")
	s.click("model-pick-" + proposed.ID)
	if s.node("model-confirm").Text() != "Download and turn on" {
		t.Fatalf("the button reads %q", s.node("model-confirm").Text())
	}
}

func TestAModelAgreedToIsDownloadedShownComingAndCancelled(t *testing.T) {
	s, fake := modelled(t, Options{})
	bare(fake)
	fake.Manual = true
	proposed, _ := fake.Recommended()
	other := others(fake)[0]
	s.turnsOn()
	s.click("model-embed")
	s.click("model-confirm")
	if !s.config.Model || s.config.ModelID != "" || !fake.Settings().On {
		t.Fatalf("agreed to, the settings are on %v with %q", s.config.Model, s.config.ModelID)
	}
	if got := entryOf(t, fake, proposed.ID).Install.State; got != model.Installing {
		t.Fatalf("agreed to, the proposed model is %v", got)
	}
	if got := entryOf(t, fake, other.ID).Install.State; got != model.NotInstalled {
		t.Fatalf("agreed to, a model that was not chosen is %v", got)
	}
	if !s.cardShown() || s.doc.ByID("robot") == nil {
		t.Fatalf("agreed to, the card is open %v and the header has its mark %v", s.cardShown(),
			s.doc.ByID("robot") != nil)
	}
	card, mark := s.box("model"), s.box("robot")
	if notch := s.card.root.Children()[0].Box().Center().X; notch < mark.X || notch > mark.X+mark.W {
		t.Fatalf("once on the card %v does not hang from the mark %v", card, mark)
	}
	if got := s.node("model-title").Text(); got != "Downloading" {
		t.Fatalf("while it downloads the card is titled %q", got)
	}
	fake.Advance(proposed.ID, proposed.Size/2, nil)
	s.frame()
	s.frame()
	if got := s.node("model-done-of").Text(); got != "50 % of "+sized(proposed.Size) {
		t.Fatalf("half way the card says %q", got)
	}
	if label, _ := s.node("robot").Attr("aria-label"); label != "Model on this device: downloading" {
		t.Fatalf("half way the mark says %q", label)
	}
	s.clean()
	s.click("model-cancel")
	if got := entryOf(t, fake, proposed.ID).Install.State; got != model.NotInstalled {
		t.Fatalf("cancelled, the model is %v", got)
	}
	if s.doc.ByID("model-cancel") != nil || s.node("model-confirm").Text() != "Download" ||
		s.node("model-title").Text() != "Not on this device yet" {
		t.Fatalf("cancelled, the card is titled %q", s.node("model-title").Text())
	}
	s.click("model-confirm")
	fake.Advance(proposed.ID, proposed.Size, nil)
	fake.SetInstalled(model.RuntimeID, true)
	fake.SetState(model.Ready, "")
	s.frame()
	s.frame()
	if got := s.node("model-title").Text(); got != "Ready, on this device" || s.doc.ByID("model-proposal") != nil {
		t.Fatalf("installed, the card is titled %q", got)
	}
	s.clean()
}

func TestAnInstallThatFailsSaysWhyAndIsTriedAgain(t *testing.T) {
	s, fake := modelled(t, Options{})
	bare(fake)
	fake.Manual = true
	proposed, _ := fake.Recommended()
	s.turnsOn()
	s.click("model-embed")
	s.click("model-confirm")
	fake.Advance(proposed.ID, 0, errors.New("the network went away"))
	s.frame()
	s.frame()
	if got := s.node("model-problem").Text(); !strings.Contains(got, "the network went away") {
		t.Fatalf("the card says %q of a failed install", got)
	}
	if s.node("model-confirm").Text() != "Try again" {
		t.Fatalf("the button reads %q", s.node("model-confirm").Text())
	}
	s.clean()
	s.click("model-confirm")
	if got := entryOf(t, fake, proposed.ID).Install.State; got != model.Installing {
		t.Fatalf("tried again, the model is %v", got)
	}
}

func TestAModelAlreadyOnTheDeviceIsTurnedOnWithoutADownload(t *testing.T) {
	s, fake := modelled(t, Options{})
	s.settings()
	s.click("set-model-on")
	if !s.cardShown() || s.node("model-confirm").Text() != "Turn on" {
		t.Fatalf("the settings' button opened the card %v, its button %q", s.cardShown(),
			s.node("model-confirm").Text())
	}
	if said := s.node("model-proposal").Text(); !strings.Contains(said, "already on this device") {
		t.Fatalf("the proposal says %q", said)
	}
	s.click("model-confirm")
	s.frame()
	if !s.config.Model || fake.Status().State != model.Ready || s.node("model-title").Text() != "Ready, on this device" {
		t.Fatalf("turned on, the model is %v", fake.Status().State)
	}
	if s.doc.ByID("set-model-on") != nil || s.doc.ByID("set-model") == nil {
		t.Fatal("turned on, the settings still offer to turn it on")
	}
	s.clean()
}

func TestTheMarkInTheHeaderSaysWhatTheModelDoes(t *testing.T) {
	s, fake := modelOn(t)
	downloads, mark, menu := s.box("downloads"), s.box("robot"), s.box("menu")
	if mark.X < downloads.X+downloads.W || mark.X+mark.W > menu.X || mark.H != menu.H {
		t.Fatalf("the mark %v is not between the downloads %v and the menu %v", mark, downloads, menu)
	}
	shapes := map[string]bool{}
	for state, want := range map[model.State][2]string{
		model.Ready:     {"ready", "Ready"},
		model.Working:   {"working", "Working"},
		model.Preparing: {"preparing", "Preparing"},
		model.Missing:   {"missing", "Not installed"},
		model.Failed:    {"failed", "Failed"},
		model.Off:       {"missing", "Not installed"},
	} {
		if state == model.Off {
			continue
		}
		fake.SetState(state, "why")
		s.frame()
		s.frame()
		robot := s.node("robot")
		if label, _ := robot.Attr("aria-label"); label != "Model on this device: "+strings.ToLower(want[1]) ||
			!robot.HasClass(want[0]) || len(robot.Classes()) != 1 {
			t.Errorf("%v: the mark says %q with the classes %v", state, label, robot.Classes())
		}
		shapes[src(s.node("robot-dot"))] = true
		s.click("menu")
		if got := s.node("menu-model-state").Text(); got != want[1] {
			t.Errorf("%v: the menu's row says %q", state, got)
		}
		s.clean()
		s.press(input.KeyEscape)
	}
	if len(shapes) != 5 {
		t.Fatalf("the mark's dot has %d shapes for five states: they are told apart by colour alone", len(shapes))
	}
}

func TestTheMenuOfAModelThatIsOnOpensItsCard(t *testing.T) {
	s, _ := modelOn(t)
	s.click("menu")
	rows := s.rows()
	at := slices.Index(rows, "Model on this device Ready")
	if at < 1 || rows[at+1] != "-" || !slices.Contains(rows[:at], "Ask about this page (off)") ||
		slices.Contains(rows, "Turn on a model on this device…") {
		t.Fatalf("the menu lists\n%s", strings.Join(rows, "\n"))
	}
	s.press(input.KeyEscape)
	s.doc.Focus(s.more)
	s.frame()
	s.press(input.KeyEnter)
	for s.focused() != "menu-model" {
		before := s.focused()
		if s.press(input.KeyDown); s.focused() == before || s.focused() == "menu-tab" {
			t.Fatalf("the arrows never reach the model's row, from %q", before)
		}
	}
	s.press(input.KeyEnter)
	if !s.cardShown() || s.choosing() {
		t.Fatalf("Enter on the row left the card open %v, the menu open %v", s.cardShown(), s.choosing())
	}
}

func TestTheCardOfAModelThatIsOnSaysWhatRunsAndCloses(t *testing.T) {
	s, fake := modelOn(t)
	proposed, _ := fake.Recommended()
	s.click("robot")
	if !s.cardShown() {
		t.Fatal("a click on the mark does not open the card")
	}
	if got := s.node("model-title").Text(); got != "Ready, on this device" {
		t.Fatalf("the card is titled %q", got)
	}
	if got := s.node("model-sub").Text(); got != proposed.Name+" · "+sized(proposed.Size)+" · graphics card" {
		t.Fatalf("under its title the card says %q", got)
	}
	if on, _ := s.node("model-switch").Attr("aria-checked"); on != "true" {
		t.Fatalf("the switch says aria-checked=%q", on)
	}
	if got := s.pressed("model-choices"); got != proposed.Name+" (proposed)" {
		t.Fatalf("the model chosen is %q", got)
	}
	for _, id := range []string{"model-proposal", "model-later", "model-confirm", "model-progress"} {
		if s.doc.ByID(id) != nil {
			t.Errorf("the card of a ready model has #%s", id)
		}
	}
	remove, done := s.box("model-delete"), s.box("model-done")
	if s.node("model-delete").Text() != "Turn off and delete the model" || remove.X > done.X ||
		done.X+done.W > s.box("model").X+s.box("model").W {
		t.Fatalf("the foot has %q at %v and Done at %v", s.node("model-delete").Text(), remove, done)
	}
	s.clean()
	s.click("model-done")
	if s.cardShown() || !s.doc.Focused(s.node("robot")) {
		t.Fatal("Done leaves the card open, or the mark without the focus")
	}
	s.click("robot")
	s.click("robot")
	if s.cardShown() {
		t.Fatal("a second click on the mark leaves the card open")
	}
	s.click("robot")
	s.press(input.KeyEscape)
	if s.cardShown() {
		t.Fatal("Escape leaves the card open")
	}
	s.click("robot")
	s.clickAt(geom.V2(300, 500))
	if s.cardShown() {
		t.Fatal("a press elsewhere leaves the card open")
	}
}

func TestTheCardIsAloneOverThePage(t *testing.T) {
	srv, _ := crypt(t)
	s, _ := modelOn(t)
	s.visit(srv.URL + "/")
	s.click("robot")
	s.click("star")
	if s.cardShown() || !s.panelShown() {
		t.Fatalf("the star left the card open %v, the bookmark's panel open %v", s.cardShown(), s.panelShown())
	}
	s.click("robot")
	if !s.cardShown() || s.panelShown() {
		t.Fatalf("the mark left the card open %v, the bookmark's panel open %v", s.cardShown(), s.panelShown())
	}
	s.click("menu")
	if s.cardShown() || !s.choosing() {
		t.Fatalf("the menu left the card open %v", s.cardShown())
	}
	s.press(input.KeyEscape)
	s.click("robot")
	s.Open(configPage)
	s.frame()
	if s.cardShown() {
		t.Fatal("the card stays open over another page")
	}
}

func TestTheSwitchTurnsTheModelOffAndKeepsItsFiles(t *testing.T) {
	s, fake := modelOn(t)
	s.click("robot")
	s.click("model-switch")
	if s.config.Model || fake.Settings().On || fake.Status().State != model.Off || s.cardShown() {
		t.Fatalf("switched off, the model is %v and the card open %v", fake.Status().State, s.cardShown())
	}
	s.nothingOfTheModel()
	for _, e := range fake.Catalogue() {
		if e.Install.State != model.Installed {
			t.Fatalf("switched off, %s is %v", e.Name, e.Install.State)
		}
	}
	s.clean()
}

func TestTurningOffAndDeletingTheModelLeavesNothingOfIt(t *testing.T) {
	s, fake := modelOn(t)
	s.configure(func(c *config.Config) { c.ModelID = others(fake)[0].ID })
	s.click("robot")
	s.click("model-delete")
	if s.config.Model || s.config.ModelID != "" || fake.Settings().On || s.cardShown() {
		t.Fatalf("deleted, the settings are on %v with %q", s.config.Model, s.config.ModelID)
	}
	for _, e := range fake.Catalogue() {
		if e.Install.State != model.NotInstalled {
			t.Fatalf("deleted, %s is %v", e.Name, e.Install.State)
		}
	}
	s.nothingOfTheModel()
}

func TestAnotherModelIsUsedAtOnceWhenItIsThereAndAgreedToWhenItIsNot(t *testing.T) {
	s, fake := modelOn(t)
	fake.Manual = true
	proposed, _ := fake.Recommended()
	there, away := others(fake)[0], others(fake)[1]
	fake.SetInstalled(away.ID, false)
	s.click("robot")
	if on, _ := s.node("model-pick-" + there.ID).Attr("aria-label"); on != there.Name+", on this device" {
		t.Fatalf("a model that is there says %q", on)
	}
	if off, _ := s.node("model-pick-" + away.ID).Attr("aria-label"); off != away.Name+", "+sized(away.Size)+
		" to download" {
		t.Fatalf("a model that is not there says %q", off)
	}
	s.click("model-pick-" + there.ID)
	if s.config.ModelID != there.ID || fake.Status().Chosen.ID != there.ID ||
		s.pressed("model-choices") != there.Name || s.doc.ByID("model-proposal") != nil {
		t.Fatalf("a model that is there is not used at once: %q", s.config.ModelID)
	}
	s.click("model-pick-" + away.ID)
	if s.config.ModelID != there.ID || entryOf(t, fake, away.ID).Install.State != model.NotInstalled {
		t.Fatal("a model that is not there is used or downloaded before it is agreed to")
	}
	if said := s.node("model-proposal").Text(); !strings.Contains(said, away.Name) ||
		!strings.Contains(said, "huggingface.co") || s.node("model-confirm").Text() != "Download and use" {
		t.Fatalf("the proposal says %q, its button %q", said, s.node("model-confirm").Text())
	}
	s.clean()
	s.click("model-confirm")
	if s.config.ModelID != away.ID || entryOf(t, fake, away.ID).Install.State != model.Installing {
		t.Fatalf("agreed to, the model is %q", s.config.ModelID)
	}
	s.click("model-cancel")
	s.click("model-pick-" + proposed.ID)
	if s.config.ModelID != "" || fake.Status().State != model.Ready {
		t.Fatalf("back on the proposed model the settings name %q, the model is %v", s.config.ModelID,
			fake.Status().State)
	}
}

func TestAProposedModelThatIsMissingLeavesTheInstalledOnesToPick(t *testing.T) {
	s, fake := modelled(t, Options{})
	proposed, _ := fake.Recommended()
	there := others(fake)[0]
	fake.SetInstalled(proposed.ID, false)
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	if label, _ := s.node("robot").Attr("aria-label"); label != "Model on this device: not installed" {
		t.Fatalf("the mark says %q", label)
	}
	s.click("robot")
	if got := s.node("model-title").Text(); got != "Not on this device yet" {
		t.Fatalf("the card is titled %q", got)
	}
	if said := s.node("model-proposal").Text(); !strings.Contains(said, proposed.Name) ||
		s.node("model-confirm").Text() != "Download" {
		t.Fatalf("the proposal says %q", said)
	}
	s.clean()
	s.click("model-pick-" + there.ID)
	s.frame()
	if s.config.ModelID != there.ID || fake.Status().State != model.Ready ||
		s.node("model-title").Text() != "Ready, on this device" {
		t.Fatalf("an installed model picked, the model is %v with %q", fake.Status().State, s.config.ModelID)
	}
	if entryOf(t, fake, proposed.ID).Install.State != model.NotInstalled {
		t.Fatal("the proposed model was downloaded")
	}
}

func TestAFileOfTheUsersOwnIsLookedIntoOnlyOnceAgreedTo(t *testing.T) {
	s, fake := modelled(t, Options{})
	own := filepath.Join(t.TempDir(), "mine.gguf")
	fake.Files = map[string]model.File{own: {Name: "Mine 7B", Size: 4 << 30, Template: true}}
	s.turnsOn()
	if s.doc.ByID("model-file") != nil {
		t.Fatal("the field of a file shows before it is asked for")
	}
	s.click("model-own")
	if !s.doc.Focused(s.node("model-file")) {
		t.Fatal("“Your own file…” does not give its field the focus")
	}
	s.clean()
	s.types("/nowhere/none.gguf")
	s.press(input.KeyEnter)
	if said := s.node("model-proposal").Text(); !strings.Contains(said, "/nowhere/none.gguf") ||
		!strings.Contains(said, "Nothing is downloaded") || s.node("model-confirm").Text() != "Turn on with this file" {
		t.Fatalf("the proposal says %q, its button %q", said, s.node("model-confirm").Text())
	}
	if s.config.Model || fake.Settings().On {
		t.Fatal("naming a file turned the model on")
	}
	s.click("model-confirm")
	s.until("the file is refused", func() bool { return s.doc.ByID("model-problem") != nil })
	if got := s.node("model-problem").Text(); !strings.Contains(got, "it is not a model") {
		t.Fatalf("the card says %q of a file that is not a model", got)
	}
	if s.config.Model || s.config.ModelFile != "" || fake.Settings().On || !s.cardShown() {
		t.Fatalf("a refused file left the model on %v with %q", s.config.Model, s.config.ModelFile)
	}
	s.clean()
	s.click("model-own")
	s.doc.SetValue(s.node("model-file"), own)
	s.click("model-file")
	s.press(input.KeyEnter)
	s.click("model-confirm")
	s.until("the file is used", func() bool { return s.config.ModelFile == own })
	s.frame()
	if !s.config.Model || s.config.ModelID != "" || fake.Status().Chosen.File != own {
		t.Fatalf("the file is not the model: %+v", fake.Status().Chosen)
	}
	if got := s.pressed("model-choices"); got != "Mine 7B" {
		t.Fatalf("the model chosen is %q", got)
	}
	if got := s.node("model-sub").Text(); got != "Mine 7B · your own file · graphics card" {
		t.Fatalf("under its title the card says %q", got)
	}
	for _, e := range fake.Catalogue() {
		if e.Install.State == model.Installing {
			t.Fatalf("a file of the user's own had %s downloaded", e.Name)
		}
	}
	s.clean()
}

func TestTheModelHelpsWithTheFeaturesLeftOn(t *testing.T) {
	dir := t.TempDir()
	s, _ := modelled(t, Options{Dir: dir})
	s.features = []feature{{"answers", "Answers in the address bar"}, {"find", "Find by meaning"}}
	if s.helps("answers") {
		t.Fatal("a model that is off helps")
	}
	s.configure(func(c *config.Config) { c.Model = true })
	s.frame()
	s.frame()
	s.click("robot")
	if got := s.pressed("model-helps"); got != "Answers in the address bar, Find by meaning" || !s.helps("find") {
		t.Fatalf("the model helps with %q", got)
	}
	s.clean()
	s.click("model-helps-find")
	if got := s.pressed("model-helps"); got != "Answers in the address bar" || s.helps("find") || !s.helps("answers") {
		t.Fatalf("with one turned off the model helps with %q", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again, _ := modelled(t, Options{Dir: dir})
	if !again.config.Model || again.helps("find") || !again.helps("answers") {
		t.Fatalf("at the next run the model is on %v and helps to find %v", again.config.Model, again.helps("find"))
	}
	again.features = s.features
	again.click("robot")
	again.click("model-helps-find")
	if !again.helps("find") {
		t.Fatal("a feature turned on again does not help")
	}
}

func TestTheSettingsOfAModelThatIsOnChangeHowItRunsAndTurnItOff(t *testing.T) {
	s, fake := modelOn(t)
	s.settings()
	s.set("set-model-threads", "6")
	if fake.Settings().Threads != 6 {
		t.Fatalf("the model runs on %d threads", fake.Settings().Threads)
	}
	s.click("set-model-cpu")
	if fake.Settings().GPU {
		t.Fatal("the graphics card is still asked to compute")
	}
	s.clean()
	s.click("set-model-choose")
	if !s.cardShown() {
		t.Fatal("“Choose the model…” does not open the card")
	}
	if got := s.node("model-sub").Text(); !strings.HasSuffix(got, " · processor") {
		t.Fatalf("under its title the card says %q", got)
	}
	s.press(input.KeyEscape)
	s.click("set-model")
	if s.config.Model || fake.Settings().On || s.doc.ByID("set-model-on") == nil {
		t.Fatal("unchecked, the model is still on")
	}
	s.nothingOfTheModel()
	s.clean()
	s.configure(func(c *config.Config) { c.Appearance = config.AppearanceDark })
	s.frame()
	s.clean()
}

func TestTheModelFollowsTheSettingsOfEveryWindowAndClosesWithLight(t *testing.T) {
	s, fake := modelOn(t)
	s.newWindow(configPage)
	s.frame()
	s.frame()
	other := s.screens[1]
	if other.doc.ByID("robot") == nil {
		t.Fatal("a second window has no mark")
	}
	s.click("robot")
	s.click("model-switch")
	s.frame()
	if other.doc.ByID("robot") != nil || other.doc.ByID("set-model-on") == nil {
		t.Fatal("turned off in a window, the model still shows in the other")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	fake.Configure(model.Settings{On: true})
	_, err := fake.Generate(context.Background(), model.Request{Messages: []model.Message{{Text: "x"}}})
	if !errors.Is(err, model.ErrClosed) {
		t.Fatalf("after light closed the model answers %v", err)
	}
}

func TestTheCardStaysOpenWhileThePointerMovesOverThePage(t *testing.T) {
	s, _ := modelOn(t)
	s.Profile.visits.Add("https://gazette.test/ferry", "Council votes", morning())
	s.rewrite()
	s.frame()
	s.click("robot")
	at := s.node("continue").Children()[1].Children()[0].Box().Center()
	for i := range 20 {
		s.in.SetMouse(geom.V2(962-float32(i)*20, 64+float32(i)*22))
		s.frame()
	}
	s.in.SetMouse(at)
	s.frame()
	s.frame()
	if !s.cardShown() {
		t.Fatal("the pointer moving over the page closed the card")
	}
}

// turnsOn opens the model's card from the settings, where a model that
// is off is turned on.
func (s *shown) turnsOn() {
	s.t.Helper()
	s.settings()
	s.click("set-model-on")
}
