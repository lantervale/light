package dom

import (
	"reflect"
	"testing"
)

func TestAFontValueIsReadAsTheFontPropertyIs(t *testing.T) {
	for value, want := range map[string]font{
		"16px Lato": {[]string{"Lato"}, 400, false},
		`italic bold 1.5em/1.2 "Open Sans", serif`: {[]string{"Open Sans", "serif"}, 700, true},
		"300 12pt 'Material Icons'":                {[]string{"Material Icons"}, 300, false},
		"oblique small Georgia":                    {[]string{"Georgia"}, 400, true},
		"normal normal 100% a , b":                 {[]string{"a", "b"}, 400, false},
	} {
		got, err := fontOf(value)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %+v %v, want %+v", value, got, err, want)
		}
	}
	for _, value := range []string{"", "Lato", "16px", "bold Lato", "16px ,"} {
		if _, err := fontOf(value); err == nil {
			t.Errorf("%q is read as a font", value)
		}
	}
}
