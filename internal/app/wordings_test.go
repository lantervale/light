package app //nolint:testpackage // the screen is driven by what it holds: its elements, its tabs, its sessions

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/ui"
)

// written are the texts of the text nodes of n, each between bars.
func written(n *ui.Node) string {
	var out []string
	n.Walk(func(k *ui.Node) bool {
		if k.IsText() {
			out = append(out, k.Text())
		}
		return true
	})
	return strings.Join(out, "|")
}

// laidOut is a Document holding the elements, laid out.
func laidOut(body ...*ui.Node) *ui.Document {
	doc := ui.NewDocument()
	doc.Body().Append(body...)
	doc.Layout(geom.V2(800, 600))
	return doc
}

func TestAPassageIsGivenAsMarkdownAndItsTranslationWrittenBackNodeByNode(t *testing.T) {
	para := ui.P(ui.Text(" The new ferry is expected to "), ui.B(ui.Text("enter service in 2028")),
		ui.Text(", once the "), ui.A(ui.Attr("href", "/arms"), ui.Text("charging arms")), ui.Text(" are installed. "))
	doc := laidOut(para)
	units := wordingsOf(doc)
	if len(units) != 1 {
		t.Fatalf("the paragraph is %d passages", len(units))
	}
	u := units[0]
	if got := u.markdown(); got != "The new ferry is expected to **enter service in 2028**, once the [charging arms] "+
		"are installed." {
		t.Fatalf("the passage is given as %q", got)
	}
	for said, want := range map[string]string{
		"Le nouveau ferry devrait **entrer en service en 2028**, une fois les [bras de recharge] installés.": " Le " +
			"nouveau ferry devrait |entrer en service en 2028|, une fois les |bras de recharge| installés. ",
		"**En 2028**, le ferry arrive avec ses [bras](https://evil.example).": " |En 2028|, le ferry arrive avec " +
			"ses |bras|. ",
		"Les [bras de recharge] installés, le ferry **entrera en service**.": " Les bras de recharge installés, le " +
			"ferry entrera en service.|||| ",
		"Le ferry entrera en service en 2028.": " Le ferry entrera en service en 2028.|||| ",
		"<b>Le ferry</b> **arrive** [vite](javascript:alert(1)) <script>x</script>": " <b>Le ferry</b> |arrive| " +
			"|vite| <script>x</script> ",
	} {
		u.write(said)
		if got := written(para); got != want {
			t.Errorf("%q is written as\n%q, want\n%q", said, got, want)
		}
		if len(para.Children()) != 5 || para.Children()[1].Tag().String() != "b" {
			t.Fatalf("writing %q changed the paragraph's elements", said)
		}
		if href, _ := para.Children()[3].Attr("href"); href != "/arms" {
			t.Fatalf("writing %q changed the link to %q", said, href)
		}
		u.restore()
		if got := written(para); got != " The new ferry is expected to |enter service in 2028|, once the |charging "+
			"arms| are installed. " {
			t.Fatalf("put back after %q, the paragraph reads %q", said, got)
		}
	}
}

func TestAPassageKeepsItsCodeAndWhatAScriptWroteSince(t *testing.T) {
	para := ui.P(ui.Text("Run "), ui.Code(ui.Text("make all")), ui.Text(" then "), ui.Em(ui.Text("wait")), ui.Text("."))
	u := wordingsOf(laidOut(para))[0]
	if got := u.markdown(); got != "Run `make all` then *wait*." {
		t.Fatalf("the passage is given as %q", got)
	}
	u.write("Lancez `faire tout` puis *attendez*.")
	if got := written(para); got != "Lancez |make all| puis |attendez|." {
		t.Fatalf("the translation is written as %q", got)
	}
	para.Children()[4].SetText("!")
	u.restore()
	if got := written(para); got != "Run |make all| then |wait|!" {
		t.Fatalf("put back, the paragraph reads %q", got)
	}
}

func TestOnlyWhatAReaderReadsIsAPassage(t *testing.T) {
	doc := laidOut(
		ui.H1(ui.Text("Titre")),
		ui.P(ui.Text("Une ligne"), ui.Br(), ui.Text("puis une autre "), ui.Img(ui.Attr("alt", "x")), ui.Text(" enfin")),
		ui.Pre(ui.Text("du code")),
		ui.P(ui.Style("display: none"), ui.Text("caché")),
		ui.Div(ui.Style("white-space: pre"), ui.Text("aligné   ainsi")),
		ui.Textarea(ui.Text("tapé")),
		ui.Button(ui.Text("Envoyer")),
		ui.Ul(ui.Li(ui.Text("un")), ui.Li(ui.Text("2028")), ui.Li(ui.Text("  "))),
		ui.P(ui.Span(ui.Text("seul"))))
	var got []string
	for _, u := range wordingsOf(doc) {
		got = append(got, u.markdown())
	}
	if want := "Titre|Une ligne|puis une autre|enfin|Envoyer|un|seul"; strings.Join(got, "|") != want {
		t.Fatalf("the passages are %q, want %q", strings.Join(got, "|"), want)
	}
}
