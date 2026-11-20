package frames_test

import (
	"testing"

	"github.com/lantervale/light/pkg/frames"
)

func TestAFrameUsesAFeatureOfItsOwnOriginOrWhereItsAllowAttributeSays(t *testing.T) {
	const site, widget, other = "https://site.test", "https://widget.test", "https://other.test"
	for _, c := range []struct {
		allow, feature, origin string
		want                   bool
	}{
		{"", "translator", site, true},
		{"", "translator", widget, false},
		{"translator", "translator", widget, true},
		{"translator", "language-detector", widget, false},
		{"translator; language-detector", "language-detector", widget, true},
		{"translator", "translator", other, false},
		{"translator 'src'", "translator", widget, true},
		{"translator *", "translator", other, true},
		{"translator 'none'", "translator", site, false},
		{"translator 'self'", "translator", widget, false},
		{"translator 'self'", "translator", site, true},
		{"TRANSLATOR https://other.test", "translator", other, true},
		{"translator https://other.test", "translator", widget, false},
		{"translator *", "translator", "null", true},
		{"translator", "translator", "null", false},
	} {
		if got := frames.Allows(c.allow, c.feature, site, widget, c.origin); got != c.want {
			t.Errorf("allow=%q lets a document of %s use %s: %t, want %t", c.allow, c.origin, c.feature, got, c.want)
		}
	}
}
