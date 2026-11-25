//go:build v8 && cgo && (((darwin || linux) && (amd64 || arm64)) || (windows && amd64))

package dom_test

import (
	"testing"

	"github.com/lantervale/light/internal/dom"
)

func TestAPickerIsAskedForOnlyAfterAPersonsPress(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><button id="b">b</button><input id="text"><input id="ro" readonly>
		<input id="box" type="checkbox" readonly><input id="off" type="date" disabled><select id="sel"><option>a</option>
		</select><select id="hid" style="display:none"><option>a</option></select>`)
	const tools = `const thrown = (id) => { try { document.getElementById(id).showPicker(); return "none" } catch (e) {
		return e.name } }, active = () => [navigator.userActivation.hasBeenActive, navigator.userActivation.isActive];`
	for script, want := range map[string]string{
		`[thrown("text"), thrown("sel"), thrown("off"), ...active()].join()`: "NotAllowedError,NotAllowedError," +
			"InvalidStateError,false,false",
		`[navigator.userActivation instanceof UserActivation, (() => { try { new UserActivation() } catch (e) {
			return e.name } })()].join()`: "true,TypeError",
	} {
		if got := s.eval(tools + script); got != want {
			t.Errorf("%s:\n got %s\nwant %s", script, got, want)
		}
	}
	for _, c := range []struct{ id, want string }{
		{"ro", "InvalidStateError,true,true"},
		{"hid", "NotSupportedError,true,true"},
		{"text", "none,true,false"},
		{"box", "none,true,false"},
		{"sel", "none,true,false"},
	} {
		s.rt.Click(s.byID("b"), 1, 1, dom.Keys{})
		if got := s.eval(tools + `[thrown("` + c.id + `"), ...active()].join()`); got != c.want {
			t.Errorf("#%s: %s, want %s", c.id, got, c.want)
		}
	}
	if got := s.eval(tools + `thrown("text")`); got != "NotAllowedError" {
		t.Errorf("a second showPicker() after one press: %s", got)
	}
	s.quiet()
}
