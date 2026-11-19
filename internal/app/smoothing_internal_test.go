package app

import "testing"

func TestALightATestRunnerDrivesDrawsItsPagesAndTheirFramesExactAndItsOwnLookAsEver(t *testing.T) {
	_, transport := framingOf(t, map[string]answer{
		"site.test/": html(`<title>Framing</title><body><p id="said">a page</p>
			<iframe id="same" src="/inner"></iframe><iframe id="inline" srcdoc="&lt;p id=in&gt;in place"></iframe>
			<iframe id="own" src="/own"></iframe>`),
		"site.test/inner": html(`<p id="in">an inner page</p>`),
		"site.test/own":   html(`<style>html { -webkit-font-smoothing: auto }</style><p id="in">its own</p>`),
	})
	const property = "-webkit-font-smoothing"
	for _, c := range []struct {
		driven, exact     bool
		page, frame, look string
	}{
		{false, false, "auto", "auto", "auto"}, {true, false, "auto", "auto", "auto"},
		{true, true, "antialiased", "antialiased", "auto"},
	} {
		s := showWith(t, Options{Transport: transport, Downloads: t.TempDir(), Driven: c.driven, ExactText: c.exact})
		doc := s.visit("https://site.test/")
		if got, _ := doc.ComputedValue(doc.ByID("said"), property); got != c.page {
			t.Errorf("driven %v, exact %v: a page's text is smoothed as %q, want %q", c.driven, c.exact, got, c.page)
		}
		for id, want := range map[string]string{"same": c.frame, "inline": c.frame, "own": "auto"} {
			s.framedText(id)
			in := doc.Frame(doc.ByID(id))
			if got, _ := in.ComputedValue(in.ByID("in"), property); got != want {
				t.Errorf("driven %v, exact %v: the text of the frame %s is smoothed as %q, want %q", c.driven, c.exact, id,
					got, want)
			}
		}
		if got, _ := s.doc.ComputedValue(s.field, property); got != c.look {
			t.Errorf("driven %v, exact %v: light's own address field is smoothed as %q, want %q", c.driven, c.exact, got, c.look)
		}
	}
}
