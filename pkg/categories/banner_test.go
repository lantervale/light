package categories_test

import (
	"strings"
	"testing"

	"github.com/lantervale/light/pkg/categories"
)

func TestEachBannerSaysWhatItIsAndAWarningIsTheOneOfACategoryThatNamesNone(t *testing.T) {
	var said []string
	for _, b := range categories.Banners() {
		said = append(said, b.String())
	}
	if got := strings.Join(said, ", "); got != "Information, Warning, Danger" {
		t.Fatalf("the banners read %s", got)
	}
	for _, c := range categories.New().All() {
		if c.Banner != categories.Warning || c.Message != "" || c.Tag != "" || c.Label() != c.Name {
			t.Fatalf("%s is shipped as %+v", c.Name, c)
		}
	}
}

func TestAFileWrittenBeforeTheBannersIsReadAsWarningsAndOneAfterAsItSays(t *testing.T) {
	l, err := categories.Decode([]byte(`{"categories": [
		{"name": "Danger", "description": "asks for a card", "colour": "#ff6b6b", "shape": "triangle", "action": "warn"},
		{"name": "Odd", "description": "odd", "colour": "#5aa9e6", "shape": "dot", "action": "warn", "banner": "siren"},
		{"name": "Sales", "description": "sells", "colour": "#f0b354", "shape": "square", "action": "warn",
			"banner": "info", "message": "  Close the tab\n before you buy.  ", "tag": " Ad "}]}`))
	if err != nil {
		t.Fatal(err)
	}
	danger, _ := l.Named("Danger")
	odd, _ := l.Named("Odd")
	sales, _ := l.Named("Sales")
	if danger.Action != categories.Warn || danger.Banner != categories.Warning || danger.Message != "" ||
		danger.Label() != "Danger" || odd.Banner != categories.Warning {
		t.Fatalf("a file written before the banners is read as %+v, a banner light does not know as %+v", danger, odd)
	}
	if sales.Banner != categories.Info || sales.Message != "Close the tab before you buy." || sales.Tag != "Ad" ||
		sales.Label() != "Ad" {
		t.Fatalf("Sales is read as %+v", sales)
	}
	data, _ := l.Encode()
	again, err := categories.Decode(data)
	if kept, _ := again.Named("Sales"); err != nil || kept != sales {
		t.Fatalf("written and read again, Sales is %+v, %v", kept, err)
	}
}

func TestABannerAMessageAndATagAreEditedWithoutTheModelBeingAskedAgain(t *testing.T) {
	l := categories.New()
	asked, version := l.Asked(), l.Version()
	danger, _ := l.Named("Danger")
	danger.Banner, danger.Message, danger.Tag = categories.Danger, " Do not type\tyour card here ", "  Risk "
	if !l.Edit("Danger", danger) {
		t.Fatal("the banner is not edited")
	}
	got, _ := l.Named("Danger")
	if got.Banner != categories.Danger || got.Message != "Do not type your card here" || got.Tag != "Risk" ||
		got.Label() != "Risk" || got.Name != "Danger" {
		t.Fatalf("edited, Danger is %+v", got)
	}
	if l.Asked() != asked || l.Version() == version {
		t.Fatal("a banner, a message or a tag changed what the model is asked, or changed nothing")
	}
	if strings.Contains(l.Question(), "Risk") || strings.Contains(categories.Why(got), "Risk") ||
		strings.Contains(l.Question(), "card here") {
		t.Fatalf("the model is asked with the tag or the message:\n%s", l.Question())
	}
	if v, ok := l.Verdict("https://parcel.example/", "danger", "Because it asks for a card"); !ok ||
		v.Name != "Danger" || v.Label() != "Risk" {
		t.Fatalf("the answer is matched as %+v", v)
	}
}

func TestATagAndAMessageAreKeptShortAndEmptiedAreTheNameAndTheReason(t *testing.T) {
	l := categories.New()
	got, _ := l.Named("Danger")
	got.Banner = categories.Danger
	l.Edit("Danger", got)
	got.Banner, got.Tag, got.Message = "siren", strings.Repeat("é", 40), strings.Repeat("long ", 80)
	l.Edit("Danger", got)
	if got, _ = l.Named("Danger"); got.Banner != categories.Danger || len(got.Tag) > 16 || len(got.Message) > 160 ||
		!strings.HasPrefix(got.Tag, "éé") {
		t.Fatalf("a banner light does not know, a long tag and a long message give %+v", got)
	}
	got.Tag, got.Message = "", ""
	l.Edit("Danger", got)
	if got, _ = l.Named("Danger"); got.Label() != "Danger" || got.Message != "" {
		t.Fatalf("emptied, the tag and the message are %+v", got)
	}
}
