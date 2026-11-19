package app //nolint:testpackage // the elements a text became are read as the screen holds them

import (
	"strings"
	"testing"

	"github.com/lantervale/ember/ui"
)

// shape is what elements a text of the model's became, each as its tag
// (or its class) and its text, with what is bold, slanted or code named.
func shape(nodes []*ui.Node) string {
	var out []string
	for _, n := range nodes {
		name := n.Tag().String()
		if n.HasClass("item") {
			name = "item"
		}
		var inside []string
		n.Walk(func(k *ui.Node) bool {
			if !k.IsText() && k != n {
				switch tag := k.Tag().String(); tag {
				case "strong", "em", "code":
					inside = append(inside, tag+"="+k.Text())
				}
			}
			return true
		})
		out = append(out, strings.TrimSpace(name+"["+n.Text()+"] "+strings.Join(inside, " ")))
	}
	return strings.Join(out, "\n")
}

func TestWhatTheModelWritesInMarkdownIsShownInItsShape(t *testing.T) {
	got := shape(markdown("## Navigateur non pris en charge\n\nVotre **navigateur** ne prend\npas en charge *cela*.\n" +
		"1. **Changez** de navigateur.\n2) Mettez `à jour`.\n- un point\n\n#pas un titre\n"))
	want := strings.Join([]string{
		"h4[Navigateur non pris en charge]",
		"p[Votre navigateur ne prend pas en charge cela.] strong=navigateur em=cela",
		"item[1.Changez de navigateur.] strong=Changez",
		"item[2.Mettez à jour.] code=à jour",
		"item[•un point]",
		"p[#pas un titre]",
	}, "\n")
	if got != want {
		t.Fatalf("the text became\n%s\nand not\n%s", got, want)
	}
}

func TestMarksThatAreNotClosedAndMarkupAreShownAsWritten(t *testing.T) {
	for text, want := range map[string]string{
		"Le **bateau":                     "p[Le **bateau]",
		"2 * 3 * 4":                       "p[2 * 3 * 4]",
		"<b>gras</b> [lien](https://x.y)": "p[<b>gras</b> [lien](https://x.y)]",
		"é**è**":                          "p[éè] strong=è",
		"":                                "",
	} {
		if got := shape(markdown(text)); got != want {
			t.Errorf("%q became %q, want %q", text, got, want)
		}
	}
}
