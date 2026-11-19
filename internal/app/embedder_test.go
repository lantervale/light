package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/config"
	"github.com/lantervale/light/pkg/model"
)

// embedding is the catalogue's model that embeds text.
func embedding(t *testing.T, fake *model.Fake) model.Entry {
	t.Helper()
	for _, e := range fake.Catalogue() {
		if e.Kind == model.KindEmbedding {
			return e
		}
	}
	t.Fatal("the catalogue has no embedding model")
	return model.Entry{}
}

// unembedded takes the embedding model off the device: the model does
// not embed.
func unembedded(t *testing.T, fake *model.Fake) model.Entry {
	t.Helper()
	e := embedding(t, fake)
	fake.SetInstalled(e.ID, false)
	caps := fake.Status().Capabilities
	caps.Embeddings = false
	fake.SetCapabilities(caps)
	return e
}

func installing(t *testing.T, fake *model.Fake, id string) bool {
	t.Helper()
	return entryOf(t, fake, id).Install.State == model.Installing
}

func TestTheProposalOffersTheEmbeddingModelAndDownloadsItOnlyWhenTicked(t *testing.T) {
	for _, ticked := range []bool{true, false} {
		s, fake := modelled(t, Options{Transport: &fenced{}})
		bare(fake)
		fake.Manual = true
		embeds := embedding(t, fake)
		proposed, _ := fake.Recommended()
		library := entryOf(t, fake, model.RuntimeID)
		s.pick("menu-model-on")
		also := s.node("model-embed")
		for _, part := range []string{"Also download " + embeds.Name, sized(embeds.Size), embeds.Licence, "huggingface.co",
			"needed to find by meaning and to ask your history"} {
			if !strings.Contains(also.Text(), part) {
				t.Fatalf("the proposal's line for the embedding model reads %q, without %q", also.Text(), part)
			}
		}
		if on, _ := also.Attr("aria-checked"); on != "true" {
			t.Fatal("the embedding model is not ticked in the proposal")
		}
		whole := proposed.Size + library.Size
		if got := s.node("model-total").Text(); got != "To download: "+sized(whole+embeds.Size) {
			t.Fatalf("ticked, the total reads %q", got)
		}
		if installing(t, fake, embeds.ID) || installing(t, fake, proposed.ID) {
			t.Fatal("something is downloaded before the proposal is agreed to")
		}
		s.clean()
		if !ticked {
			s.click("model-embed")
			if on, _ := s.node("model-embed").Attr("aria-checked"); on != "false" ||
				s.node("model-total").Text() != "To download: "+sized(whole) {
				t.Fatalf("unticked, the line is checked %q and the total reads %q", on, s.node("model-total").Text())
			}
		}
		s.click("model-confirm")
		if !installing(t, fake, proposed.ID) || installing(t, fake, embeds.ID) != ticked {
			t.Fatalf("ticked %v, the embedding model is downloading %v", ticked, installing(t, fake, embeds.ID))
		}
		if ticked {
			if got := s.node("model-done-of").Text(); !strings.HasSuffix(got, sized(whole+embeds.Size)) {
				t.Fatalf("the progress is of %q", got)
			}
			s.click("model-cancel")
			if installing(t, fake, proposed.ID) || installing(t, fake, embeds.ID) {
				t.Fatal("Cancel leaves a download going")
			}
		}
		s.clean()
	}
}

func TestAModelOnWithoutTheEmbeddingModelOffersItAndAsksBeforeDownloading(t *testing.T) {
	s, fake := modelOn(t)
	fake.Manual = true
	embeds := unembedded(t, fake)
	s.frame()
	s.click("robot")
	need := s.node("model-embed-need")
	if !strings.Contains(need.Text(), "need "+embeds.Name) || s.doc.ByID("model-embed-ask") != nil {
		t.Fatalf("the card says %q of the embedding model", need.Text())
	}
	for _, id := range []string{findFeature, recallFeature} {
		if label, _ := s.node("model-helps-" + id).Attr("aria-label"); !strings.Contains(label, "needs "+embeds.Name) {
			t.Fatalf("the chip of %s says %q", id, label)
		}
	}
	s.clean()
	s.click("model-embed-get")
	ask := s.node("model-embed-ask")
	if installing(t, fake, embeds.ID) {
		t.Fatal("the embedding model is downloaded before it is agreed to")
	}
	for _, part := range []string{embeds.Name, sized(embeds.Size), embeds.Licence, "huggingface.co"} {
		if !strings.Contains(ask.Text(), part) {
			t.Fatalf("the card asks %q, without %q", ask.Text(), part)
		}
	}
	s.clean()
	s.click("model-embed-later")
	if s.doc.ByID("model-embed-ask") != nil || installing(t, fake, embeds.ID) {
		t.Fatal("Not now leaves the question, or downloads")
	}
	s.click("model-helps-" + recallFeature)
	if !s.helps(recallFeature) || s.doc.ByID("model-embed-ask") == nil || installing(t, fake, embeds.ID) {
		t.Fatal("turning the remembering on without the embedding model does not ask for it")
	}
	s.click("model-embed-confirm")
	if !installing(t, fake, embeds.ID) || s.doc.ByID("model-progress") == nil {
		t.Fatal("agreed to, the embedding model is not downloading, or shows no progress")
	}
	s.clean()
	s.click("model-cancel")
	if installing(t, fake, embeds.ID) {
		t.Fatal("Cancel leaves the download going")
	}
	fake.SetInstalled(embeds.ID, true)
	s.frame()
	s.frame()
	if s.doc.ByID("model-embed-need") != nil {
		t.Fatal("with the embedding model on the device the card still asks for it")
	}
	s.click("model-delete")
	for _, e := range fake.Catalogue() {
		if e.Install.State == model.Installed {
			t.Fatalf("deleting the model leaves %s on the device", e.Name)
		}
	}
}

func TestWithoutTheEmbeddingModelTheHistoryIsFilteredPlainlyAndSettingsSayIt(t *testing.T) {
	s, fake := modelOn(t)
	embeds := unembedded(t, fake)
	s.assists.Set(recallFeature, true)
	s.redrawModel()
	s.Open(historyPage)
	s.frame()
	s.frame()
	if s.asking != nil && s.asking.on {
		t.Fatal("the history takes questions without a model that embeds")
	}
	if n := s.doc.ByID("history-ask"); n != nil && s.seen(n) {
		t.Fatal("the history shows its question card without a model that embeds")
	}
	s.click("history-filter")
	s.types("ferry")
	s.press(input.KeyEnter)
	if n := s.doc.ByID("history-ask"); n != nil && s.seen(n) {
		t.Fatal("Enter in the filter opens the question card without a model that embeds")
	}
	s.clean()
	s.settings()
	if got := s.node("set-model-embed").Text(); !strings.Contains(got, embeds.Name) ||
		!strings.Contains(got, "not on this device") {
		t.Fatalf("the settings say %q of the embedding model", got)
	}
	fake.SetInstalled(embeds.ID, true)
	s.configure(func(*config.Config) {})
	s.frame()
	if got := s.node("set-model-embed").Text(); !strings.Contains(got, "on this device") ||
		strings.Contains(got, "not on") {
		t.Fatalf("installed, the settings say %q of the embedding model", got)
	}
	s.clean()
}
