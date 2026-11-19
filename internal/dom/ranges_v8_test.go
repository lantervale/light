//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package dom_test

import "testing"

const rangePage = `<!DOCTYPE html><body><div id="a"><p id="p1">one <b id="b">two</b> three</p><p id="p2">four</p>` +
	`<p id="p3">five <i id="i">six</i></p></div><div id="z"></div>`

const rangeTools = `const $ = (id) => document.getElementById(id), at = (r) => [r.startContainer.id ||
	r.startContainer.nodeName, r.startOffset, r.endContainer.id || r.endContainer.nodeName, r.endOffset].join(" "),
	between = (sn, so, en, eo) => { const r = document.createRange(); r.setStart(sn, so); r.setEnd(en, eo); return r },
	thrown = (run) => { try { run(); return "none" } catch (e) { return e.name } };`

func TestARangeHoldsWhatIsBetweenItsEnds(t *testing.T) {
	for _, c := range []struct{ name, script, want string }{
		{"a new range is collapsed at the start of its document", `(() => { const r = new Range(), d =
			document.createRange(); return [at(r), r.collapsed, r.commonAncestorContainer === document, at(d),
			r instanceof AbstractRange, String(r), Object.prototype.toString.call(r)].join() })()`,
			"#document 0 #document 0,true,true,#document 0 #document 0,true,,[object Range]"},
		{"its ends are set, and an end before the start takes the start with it", `(() => { const r = between($("p1"), 0,
			$("p2"), 1), seen = [at(r), r.collapsed, r.commonAncestorContainer.id]; r.setEnd($("a"), 0); seen.push(at(r));
			r.setStart($("p3"), 1); seen.push(at(r)); r.selectNode($("p2")); seen.push(at(r));
			r.selectNodeContents($("p1")); seen.push(at(r)); r.collapse(true); seen.push(at(r));
			r.setStartAfter($("b")); r.setEndBefore($("p3")); seen.push(at(r)); r.setStartBefore($("p1"));
			r.setEndAfter($("p1")); seen.push(at(r)); return seen.join() })()`,
			"p1 0 p2 1,false,a,a 0 a 0,p3 1 p3 1,a 1 a 2,p1 0 p1 3,p1 0 p1 0,p1 2 a 2,a 0 a 1"},
		{"what it cannot hold is refused", `[() => new Range().setStart(document.doctype, 0), () => new Range()
			.setStart($("p1"), 9), () => new Range().selectNode(document), () => new Range().setStart(null, 0),
			() => new Range().compareBoundaryPoints(4, new Range()), () => new Range().compareBoundaryPoints(0,
			between(document.createElement("p"), 0, document.createElement("p"), 0)), () => new Range().comparePoint(
			document.createElement("p"), 0), () => between($("p1"), 0, $("p2"), 1).surroundContents(
			document.createElement("u")), () => new Range().surroundContents(document), () => new Range().insertNode(
			$("a")), () => new StaticRange({ startContainer: document.doctype, startOffset: 0, endContainer: document,
			endOffset: 0 }), () => new StaticRange({ startContainer: document })].map(thrown).join()`,
			"InvalidNodeTypeError,IndexSizeError,InvalidNodeTypeError,TypeError,NotSupportedError,WrongDocumentError," +
				"WrongDocumentError,InvalidStateError,InvalidNodeTypeError,HierarchyRequestError,InvalidNodeTypeError," +
				"TypeError"},
		{"ranges and points are compared", `(() => { const r = between($("p1").firstChild, 2, $("p2"), 1), o =
			between($("p1"), 0, $("p3"), 0); return [r.compareBoundaryPoints(Range.START_TO_START, o),
			r.compareBoundaryPoints(Range.START_TO_END, o), r.compareBoundaryPoints(Range.END_TO_END, o),
			r.compareBoundaryPoints(Range.END_TO_START, o), r.comparePoint($("p1"), 0), r.comparePoint($("b"), 0),
			r.comparePoint($("p3"), 0), r.isPointInRange($("b").firstChild, 1), r.isPointInRange($("i"), 0),
			r.intersectsNode($("b")), r.intersectsNode($("p3")), r.intersectsNode($("a")), Range.END_TO_START,
			r.END_TO_END].join() })()`, "1,1,-1,-1,-1,0,1,true,false,true,false,true,3,2"},
		{"its text is the text between its ends", `[between($("p1").firstChild, 1, $("i").firstChild, 2),
			between($("b").firstChild, 1, $("b").firstChild, 2), between($("a"), 1, $("a"), 3), between($("p1"), 1,
			$("p2").firstChild, 2)].map(String).join("|")`, "ne two threefourfive si|w|fourfive six|two threefo"},
		{"its content is copied, and the tree is left as it was", `(() => { const r = between($("p1").firstChild, 2,
			$("i").firstChild, 1), holder = document.createElement("div"); holder.append(r.cloneContents());
			return [holder.innerHTML, at(r), $("a").childNodes.length, between($("b").firstChild, 1, $("b").firstChild,
			3).cloneContents().textContent, new Range().cloneContents().childNodes.length].join("|") })()`,
			`<p id="p1">e <b id="b">two</b> three</p><p id="p2">four</p><p id="p3">five <i id="i">s</i></p>|` +
				"#text 2 #text 1|3|wo|0"},
		{"its content is taken out, and the range collapses where it was", `(() => { const r = between(
			$("p1").firstChild, 2, $("i").firstChild, 1), holder = document.createElement("div");
			holder.append(r.extractContents()); return [holder.innerHTML, $("a").innerHTML, at(r)].join("|") })()`,
			`<p id="p1">e <b id="b">two</b> three</p><p id="p2">four</p><p id="p3">five <i id="i">s</i></p>|` +
				`<p id="p1">on</p><p id="p3"><i id="i">ix</i></p>|a 1 a 1`},
		{"its content is deleted", `(() => { const r = between($("b").firstChild, 1, $("p3"), 1); r.deleteContents();
			const inner = between($("p1").firstChild, 1, $("p1").firstChild, 3); inner.deleteContents();
			return [$("a").innerHTML, at(r), at(inner)].join("|") })()`,
			`<p id="p1">o <b id="b">t</b></p><p id="p3"><i id="i">six</i></p>|a 1 a 1|#text 1 #text 1`},
		{"a node is put at its start, splitting the text there", `(() => { const r = between($("p2").firstChild, 2,
			$("p2").firstChild, 2), u = document.createElement("u"); r.insertNode(u); const whole = between($("p3"), 0,
			$("p3"), 1); whole.insertNode(document.createTextNode("!")); return [$("p2").innerHTML, at(r),
			$("p3").innerHTML, at(whole)].join("|") })()`,
			`fo<u></u>ur|#text 2 p2 2|!five <i id="i">six</i>|p3 0 p3 2`},
		{"its content is put in a node that takes its place", `(() => { const r = between($("p1").firstChild, 1,
			$("p1").lastChild, 2), u = document.createElement("u"); u.textContent = "gone"; r.surroundContents(u);
			return [$("p1").innerHTML, at(r)].join("|") })()`, `o<u>ne <b id="b">two</b> t</u>hree|p1 1 p1 2`},
		{"a copy of it stands alone, and a static one never moves", `(() => { const r = between($("p1"), 0, $("p2"), 1),
			c = r.cloneRange(), s = new StaticRange({ startContainer: $("p1"), startOffset: 1, endContainer: $("a"),
			endOffset: 9 }); c.collapse(); $("p1").remove(); return [at(r), at(c), at(s), s.collapsed, s instanceof
			AbstractRange, s instanceof Range, r.detach()].join() })()`,
			"a 0 p2 1,p2 1 p2 1,p1 1 a 9,false,true,false,"},
	} {
		s := run(t, rangePage)
		if got := s.eval(rangeTools + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
		s.quiet()
	}
}

func TestARangeFollowsTheTreeAsItChanges(t *testing.T) {
	for _, c := range []struct{ name, script, want string }{
		{"a node taken out takes the ends in it to where it was", `(() => { const r = between($("b").firstChild, 1,
			$("p3"), 1), later = between($("a"), 2, $("a"), 3); $("p1").remove(); return at(r) + "," + at(later) })()`,
			"a 0 p3 1,a 1 a 2"},
		{"a node put in moves the ends after it", `(() => { const r = between($("a"), 1, $("a"), 3), edge =
			between($("a"), 0, $("a"), 0); $("a").prepend(document.createElement("hr"), "t"); $("a").append("end");
			return at(r) + "," + at(edge) })()`, "a 3 a 5,a 0 a 0"},
		{"text changed moves the ends in it", `(() => { const text = $("p2").firstChild, r = between(text, 1, text, 3),
			seen = []; text.insertData(0, "ab"); seen.push(at(r)); text.deleteData(1, 3); seen.push(at(r));
			text.replaceData(0, 1, "xyz"); seen.push(at(r)); text.data = "new"; seen.push(at(r));
			text.appendData("more"); seen.push(at(r)); return seen.join() })()`,
			"#text 3 #text 5,#text 1 #text 2,#text 0 #text 4,#text 0 #text 0,#text 0 #text 0"},
		{"text split takes the ends past the cut to the new node", `(() => { const text = $("p2").firstChild, r =
			between(text, 1, text, 3), after = between($("p2"), 1, $("p2"), 1), rest = text.splitText(2);
			return [at(r), r.endContainer === rest, at(after), text.data, rest.data].join() })()`,
			"#text 1 #text 1,true,p2 2 p2 2,fo,ur"},
		{"text nodes joined keep the ends in them", `(() => { const p = $("p2"); p.append("more", "", "last");
			const r = between(p.childNodes[1], 2, p.childNodes[3], 1), mid = between(p, 2, p, 3); p.normalize();
			return [at(r), at(mid), p.childNodes.length, r.startContainer === p.firstChild].join() })()`,
			"#text 6 #text 9,#text 8 #text 8,1,true"},
		{"new content takes every end out", `(() => { const r = between($("b").firstChild, 1, $("p1"), 3), other =
			between($("i"), 0, $("p3"), 2); $("p1").textContent = "x"; $("p3").innerHTML = "<u>y</u>";
			return at(r) + "," + at(other) })()`, "p1 0 p1 0,p3 0 p3 0"},
	} {
		s := run(t, rangePage)
		if got := s.eval(rangeTools + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
		s.quiet()
	}
}

func TestTheTreeIsWalked(t *testing.T) {
	for _, c := range []struct{ name, script, want string }{
		{"a walker moves every way among what it shows", `(() => { const w = document.createTreeWalker($("a"),
			NodeFilter.SHOW_ELEMENT), seen = []; for (const move of ["firstChild", "firstChild", "parentNode",
			"nextSibling", "nextNode", "previousNode", "previousSibling", "lastChild", "nextNode", "nextNode",
			"nextNode", "nextNode", "parentNode", "parentNode", "parentNode"]) { const n = w[move]();
			seen.push(n ? n.id : null) } return [seen.join(" "), w.currentNode.id, w.root.id, w.whatToShow, w.filter,
			Object.prototype.toString.call(w)].join() })()`,
			"p1 b p1 p2 p3 p2 p1 b p2 p3 i  p3 a ,a,a,1,,[object TreeWalker]"},
		{"a filter skips a node or rejects all under it", `(() => { const names = (filter) => { const w =
			document.createTreeWalker($("a"), NodeFilter.SHOW_ELEMENT, filter), seen = []; while (w.nextNode())
			seen.push(w.currentNode.id); return seen.join(" ") }; return [names((n) => n.id === "p1" ?
			NodeFilter.FILTER_SKIP : NodeFilter.FILTER_ACCEPT), names({ acceptNode: (n) => n.id === "p1" ?
			NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT }), names(null)].join() })()`,
			"b p2 p3 i,p2 p3 i,p1 b p2 p3 i"},
		{"a filter that walks, or is no filter, is refused", `(() => { const w = document.createTreeWalker($("a"),
			NodeFilter.SHOW_ALL, () => w.nextNode()), bad = document.createTreeWalker($("a"), 1, {});
			return [thrown(() => w.nextNode()), thrown(() => bad.firstChild()), thrown(() => document.createTreeWalker(
			$("a"), 1, 5)), thrown(() => document.createTreeWalker()), thrown(() => new TreeWalker()),
			thrown(() => new NodeFilter()), NodeFilter.SHOW_COMMENT, NodeFilter.FILTER_SKIP].join() })()`,
			"InvalidStateError,TypeError,TypeError,TypeError,TypeError,TypeError,128,3"},
		{"an iterator goes forth and back over a pointer", `(() => { const it = document.createNodeIterator($("p1")),
			seen = []; for (const move of ["nextNode", "nextNode", "previousNode", "previousNode", "previousNode",
			"nextNode", "nextNode", "nextNode", "nextNode", "nextNode", "nextNode", "nextNode"]) { const n = it[move]();
			seen.push(n ? n.id || n.data : null) } return [seen.join("|"), it.referenceNode.data,
			it.pointerBeforeReferenceNode, it.root.id, it.whatToShow, it.detach(), Object.prototype.toString.call(it)]
			.join() })()`,
			"p1|one |one |p1||p1|one |b|two| three||, three,false,p1,4294967295,,[object NodeIterator]"},
		{"an iterator leaves a node that is taken out", `(() => { const it = document.createNodeIterator($("a"),
			NodeFilter.SHOW_ELEMENT), seen = []; it.nextNode(); it.nextNode(); it.nextNode(); $("p1").remove();
			seen.push(it.referenceNode.id, it.pointerBeforeReferenceNode, it.nextNode().id); it.previousNode();
			$("p2").remove(); seen.push(it.referenceNode.id, it.pointerBeforeReferenceNode); const last =
			document.createNodeIterator($("a"), 1); while (last.nextNode()); $("p3").remove();
			seen.push(last.referenceNode.id, last.pointerBeforeReferenceNode); return seen.join() })()`,
			"a,false,p2,p3,true,a,false"},
		{"while its filter is asked an iterator stands where it stood, and follows what the filter takes out", `(() => {
			const root = document.createElement("div"), [x, y, z] = ["x", "y", "z"].map((id) => { const e =
			document.createElement("i"); e.id = id; root.append(e); return e }), seen = [];
			const it = document.createNodeIterator(root, 1, (node) => { seen.push(it.referenceNode.id || "root");
			if (node === y) y.remove(); return 1 }); it.nextNode(); it.nextNode(); const got = it.nextNode();
			seen.push(got.id, it.referenceNode.id, it.pointerBeforeReferenceNode, it.nextNode().id);
			const again = document.createNodeIterator(root, 1, () => { again.nextNode(); return 1 });
			try { again.nextNode() } catch (e) { seen.push(e.name) } return seen.join() })()`,
			"root,root,x,x,y,x,false,z,InvalidStateError"},
	} {
		s := run(t, rangePage)
		if got := s.eval(rangeTools + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
		s.quiet()
	}
}
