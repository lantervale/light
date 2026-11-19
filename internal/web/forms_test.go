package web //nolint:testpackage // asks the builder what a form sends, which only a click on the page reaches

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

// consent is a page that asks for a choice as such pages are written:
// a form a button, each with its hidden fields, sent with a POST to
// another host, and a form of the page's language sent with a GET.
const consent = `<form action="https://save.test/save" method="POST" id="refuse" style="display:inline;">
<input type="hidden" name="gl" value="FR"><input type="hidden" name="continue" value="https://www.site.test/">
<input type="hidden" name="set_eom" value="true"><input type="submit" value="Tout refuser" id="no"/></form>
<form action="https://save.test/save" method="POST" id="accept" style="display:inline;">
<input type="hidden" name="gl" value="FR"><input type="hidden" name="continue" value="https://www.site.test/">
<input type="hidden" name="set_eom" value="false"><input type="hidden" name="set_sc" value="true">
<input type="submit" value="Tout accepter" id="yes"/></form>
<form action="/ml" method="get" id="language"><select name="hl"><option value="en">English</option>
<option value="fr" selected>Français</option></select><input type="hidden" name="oldhl" value="fr">
<input type="submit" value="Changer de langue" id="change"></form>`

// forms is the builder of the page whose HTML is body, at its address.
func forms(t *testing.T, body string) *builder {
	t.Helper()
	doc, _ := built(t, body, nil)
	at, err := url.Parse("https://www.site.test/ml?hl=fr#top")
	if err != nil {
		t.Fatal(err)
	}
	return &builder{doc: doc, base: at}
}

func TestAFormIsSentAsItsMethodItsTypeAndItsButtonSay(t *testing.T) {
	const fields = `<input name="q" value="é à"><input type="checkbox" name="on" checked>
		<input type="checkbox" name="off"><input type="radio" name="size" value="s"><input type="radio" name="size"
		value="l" checked><input name="gone" value="1" disabled><input value="nameless"><input type="file" name="doc">
		<input type="reset" name="reset" value="r"><input type="button" name="plain" value="p">
		<select name="pick"><option value="a">A</option><option value="b" selected>B</option></select>
		<textarea name="note">two
lines</textarea>`
	const sent = "q=%C3%A9+%C3%A0&on=on&size=l&doc=&pick=b&note=two%0D%0Alines"
	for _, c := range []struct {
		name, body, by string
		// address, kind and carried are what is sent, kind "" for a GET.
		address, kind, carried string
		err                    error
	}{
		{"a POST", `<form id="f" method="post" action="/save?x=1">` + fields + `<button name="go" value="1" id="b">Go
			</button><button name="other" value="2">Other</button></form>`, "b",
			"https://www.site.test/save?x=1", "application/x-www-form-urlencoded", sent + "&go=1", nil},
		{"a POST with no action", `<form id="f" method="POST"><input name="a" value="1"></form>`, "",
			"https://www.site.test/ml?hl=fr#top", "application/x-www-form-urlencoded", "a=1", nil},
		{"a GET", `<form id="f" action="/find?x=1#here"><input name="a" value="1"><input type="submit" id="b"></form>`,
			"b", "https://www.site.test/find?a=1#here", "", "", nil},
		{"a method no browser knows", `<form id="f" method="put" action="/find"><input name="a" value="1"></form>`, "",
			"https://www.site.test/find?a=1", "", "", nil},
		{"a button with its own address and method", `<form id="f" action="/find"><input name="a" value="1">
			<input type="submit" id="b" name="s" value="v" formaction="https://other.test/take" formmethod="post"></form>`,
			"b", "https://other.test/take", "application/x-www-form-urlencoded", "a=1&s=v", nil},
		{"a button that makes it a GET", `<form id="f" action="/save" method="post"><input name="a" value="1">
			<button id="b" formmethod="get"></button></form>`, "b", "https://www.site.test/save?a=1", "", "", nil},
		{"plain text", `<form id="f" action="/save" method="post" enctype="text/plain"><input name="a" value="1 2">
			<input name="b" value="é"></form>`, "", "https://www.site.test/save", "text/plain", "a=1 2\r\nb=é\r\n", nil},
		{"a type no browser knows", `<form id="f" action="/save" method="post" enctype="application/json">
			<input name="a" value="1"></form>`, "", "https://www.site.test/save", "application/x-www-form-urlencoded",
			"a=1", nil},
		{"controls of the form by its id", `<input name="before" value="0" form="f"><form id="f" action="/save"
			method="post"><input name="a" value="1"><input name="away" value="2" form="g"></form><form id="g"></form>
			<button id="b" form="f" name="by" value="3"></button>`, "b", "https://www.site.test/save",
			"application/x-www-form-urlencoded", "before=0&a=1&by=3", nil},
		{"multipart", `<form id="f" action="/save" method="post" enctype="multipart/form-data">
			<input name="a" value="1"></form>`, "", "", "", "", ErrMultipart},
		{"a button that makes it multipart", `<form id="f" action="/save" method="post"><input name="a" value="1">
			<button id="b" formenctype="multipart/form-data"></button></form>`, "b", "", "", "", ErrMultipart},
		{"a multipart GET", `<form id="f" action="/find" enctype="multipart/form-data"><input name="a" value="1">
			</form>`, "", "https://www.site.test/find?a=1", "", "", nil},
		{"a dialog's form", `<form id="f" method="dialog"><input name="a" value="1"></form>`, "", "", "", "", ErrDialog},
		{"an address light does not open", `<form id="f" method="post" action="mailto:a@site.test"></form>`, "", "",
			"", "", ErrNoAddress},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := forms(t, c.body)
			got, err := b.submission(b.doc.ByID("f"), b.doc.ByID(c.by))
			if !errors.Is(err, c.err) {
				t.Fatalf("the form answers %v, want %v", err, c.err)
			}
			kind, carried, origin := "", "", ""
			if got.Form != nil {
				kind, carried, origin = got.Form.Type, string(got.Form.Body), got.Form.Origin
			}
			if got.Address != c.address || kind != c.kind || carried != c.carried {
				t.Errorf("it is sent to %s as %q carrying %q, want %s as %q carrying %q", got.Address, kind, carried,
					c.address, c.kind, c.carried)
			}
			if c.kind == "" {
				return
			}
			referrer := "https://www.site.test/ml?hl=fr"
			if strings.HasPrefix(c.address, "https://other.test") {
				referrer = "https://www.site.test/"
			}
			if origin != "https://www.site.test" || got.Referrer != referrer {
				t.Errorf("it says it comes from %q, of the origin %q", got.Referrer, origin)
			}
		})
	}
}

func TestEachButtonOfAConsentPageSendsItsOwnForm(t *testing.T) {
	b := forms(t, consent)
	const common = "gl=FR&continue=https%3A%2F%2Fwww.site.test%2F&"
	for button, fields := range map[string]string{"no": "set_eom=true", "yes": "set_eom=false&set_sc=true"} {
		want := common + fields
		form := FormOf(b.doc.ByID(button))
		got, err := b.submission(form, b.doc.ByID(button))
		if err != nil || got.Form == nil {
			t.Fatalf("#%s: %v, %v", button, got, err)
		}
		if got.Address != "https://save.test/save" || string(got.Form.Body) != want {
			t.Errorf("#%s sends %q to %s, want %q", button, got.Form.Body, got.Address, want)
		}
		if got.Form.Origin != "https://www.site.test" || got.Referrer != "https://www.site.test/" {
			t.Errorf("#%s says it comes from %q, of the origin %q", button, got.Referrer, got.Form.Origin)
		}
	}
	got, err := b.submission(b.doc.ByID("language"), b.doc.ByID("change"))
	if err != nil || got.Form != nil || got.Address != "https://www.site.test/ml?hl=fr&oldhl=fr" {
		t.Errorf("the language is sent as %v, %v", got, err)
	}
}
