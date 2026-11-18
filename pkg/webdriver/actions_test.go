package webdriver_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/lantervale/light/pkg/webdriver"
)

func TestActionsAreReadIntoSourcesAndTicks(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	a := d.ask("POST", "/session/{s}/actions", `{"actions":[
		{"type":"key","id":"keys","actions":[{"type":"keyDown","value":"a"},{"type":"keyUp","value":"a"},
			{"type":"pause","duration":20}]},
		{"type":"pointer","id":"mouse","parameters":{"pointerType":"mouse"},"actions":[
			{"type":"pointerMove","x":10.5,"y":20,"duration":100,"origin":{"`+webdriver.ElementKey+`":"e3"}},
			{"type":"pointerDown","button":0},{"type":"pointerUp","button":0},
			{"type":"pointerMove","x":1,"y":2,"origin":"pointer"}]},
		{"type":"wheel","id":"wheel","actions":[{"type":"scroll","x":1,"y":2,"deltaX":0,"deltaY":120}]},
		{"type":"none","id":"wait","actions":[{"type":"pause"}]}]}`)
	if a.status != 200 {
		t.Fatalf("the actions are answered %d %s", a.status, a.text)
	}
	want := []webdriver.Source{
		{ID: "keys", Type: "key", Pointer: "mouse", Actions: []webdriver.Action{
			{Type: "keyDown", Value: "a", Origin: "viewport"}, {Type: "keyUp", Value: "a", Origin: "viewport"},
			{Type: "pause", Duration: 20 * time.Millisecond, Origin: "viewport"}}},
		{ID: "mouse", Type: "pointer", Pointer: "mouse", Actions: []webdriver.Action{
			{Type: "pointerMove", X: 10.5, Y: 20, Duration: 100 * time.Millisecond, Element: "e3"},
			{Type: "pointerDown", Origin: "viewport"}, {Type: "pointerUp", Origin: "viewport"},
			{Type: "pointerMove", X: 1, Y: 2, Origin: "pointer"}}},
		{ID: "wheel", Type: "wheel", Pointer: "mouse", Actions: []webdriver.Action{
			{Type: "scroll", X: 1, Y: 2, DeltaY: 120, Origin: "viewport"}}},
		{ID: "wait", Type: "none", Pointer: "mouse", Actions: []webdriver.Action{{Type: "pause", Origin: "viewport"}}},
	}
	if !reflect.DeepEqual(d.browser.sources, want) {
		t.Fatalf("the sources are\n%+v, want\n%+v", d.browser.sources, want)
	}
	ticks := webdriver.Ticks(d.browser.sources)
	var sizes []int
	for _, tick := range ticks {
		sizes = append(sizes, len(tick))
	}
	if !reflect.DeepEqual(sizes, []int{4, 2, 2, 1}) || ticks[3][0].Source.ID != "mouse" ||
		ticks[1][1].Action.Type != "pointerDown" {
		t.Fatalf("the ticks hold %v actions", sizes)
	}
}

func TestActionsThatAreNoneOfTheStandardAreInvalid(t *testing.T) {
	d := drive(t)
	d.begin("{}")
	for _, body := range []string{
		`{}`, `{"actions":{}}`, `{"actions":[1]}`,
		`{"actions":[{"type":"key","actions":[]}]}`,
		`{"actions":[{"type":"gamepad","id":"a","actions":[]}]}`,
		`{"actions":[{"type":"key","id":"a"}]}`,
		`{"actions":[{"type":"key","id":"a","actions":[{"type":"keyDown"}]}]}`,
		`{"actions":[{"type":"key","id":"a","actions":[{"type":"keyDown","value":""}]}]}`,
		`{"actions":[{"type":"key","id":"a","actions":[{"type":"pointerDown","button":0}]}]}`,
		`{"actions":[{"type":"none","id":"a","actions":[{"type":"keyDown","value":"a"}]}]}`,
		`{"actions":[{"type":"none","id":"a","actions":[{"type":"pause","duration":-1}]}]}`,
		`{"actions":[{"type":"pointer","id":"a","parameters":{"pointerType":"paw"},"actions":[]}]}`,
		`{"actions":[{"type":"pointer","id":"a","actions":[{"type":"pointerDown"}]}]}`,
		`{"actions":[{"type":"pointer","id":"a","actions":[{"type":"pointerDown","button":-1}]}]}`,
		`{"actions":[{"type":"pointer","id":"a","actions":[{"type":"pointerMove","x":1}]}]}`,
		`{"actions":[{"type":"pointer","id":"a","actions":[{"type":"pointerMove","x":1,"y":1,"origin":"page"}]}]}`,
		`{"actions":[{"type":"pointer","id":"a","actions":[{"type":"pointerMove","x":1,"y":1,"origin":{}}]}]}`,
		`{"actions":[{"type":"wheel","id":"a","actions":[{"type":"scroll","x":1,"y":1}]}]}`,
		`{"actions":[{"type":"wheel","id":"a","actions":[
			{"type":"scroll","x":1,"y":1,"deltaX":0,"deltaY":0,"origin":"pointer"}]}]}`,
	} {
		d.browser.calls = nil
		if a := d.ask("POST", "/session/{s}/actions", body); a.code != "invalid argument" || len(d.browser.calls) != 0 {
			t.Errorf("%s is answered %d %s", body, a.status, a.text)
		}
	}
}
