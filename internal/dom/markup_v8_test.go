//go:build v8

package dom_test

import "testing"

func TestMarkupKeepsWhatIsOutOfSightWhereItIsWritten(t *testing.T) {
	const page = `<div id="box"><p id="first">One</p></div>`
	for _, c := range []struct{ name, script, want string }{
		{"a style in a detached element's innerHTML", `const div = document.createElement("div");
			div.innerHTML = "<p>x</p><style id='made'>.wide { width: 100% }</style>";
			const style = div.childNodes[1];
			document.head.appendChild(style);
			[div.childNodes.length, style.localName, style.textContent, made.parentNode === document.head,
				div.innerHTML].join()`,
			"1,style,.wide { width: 100% },true,<p>x</p>"},
		{"a style in a connected element's innerHTML", `first.innerHTML = "a<style id='made'>p { top: 0 }</style>b";
			[first.childNodes.length, document.getElementById("made").localName, first.textContent].join()`,
			"3,style,ap { top: 0 }b"},
		{"a script in markup does not run", `window.ran = 0; const div = document.createElement("div");
			div.innerHTML = "<script>window.ran++<\/script>"; const held = div.childNodes.length;
			box.appendChild(div); first.insertAdjacentHTML("afterend", "<script>window.ran++<\/script>");
			first.innerHTML = "<script>window.ran++<\/script>";
			[held, window.ran].join()`,
			"1,0"},
		{"a link and a template's content", `const t = document.createElement("template");
			t.innerHTML = "<link rel='x' href='/y'><b>z</b>";
			[t.content.childNodes.length, t.content.firstChild.localName].join()`,
			"2,link"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := run(t, page)
			if got := s.eval(c.script); got != c.want {
				t.Errorf("%s\n is %q\nwant %q", c.script, got, c.want)
			}
			s.quiet()
		})
	}
}
