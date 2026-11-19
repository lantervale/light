//go:build v8

package dom_test

import "testing"

func TestInterfacesAreShapedAsTheirIDLSays(t *testing.T) {
	s := run(t, `<!DOCTYPE html><body><p id="p">text</p>`)
	const tools = `const thrown = (run) => { try { run(); return "none" } catch (e) { return e.name } },
		own = (o, k) => Object.getOwnPropertyDescriptor(o, k), p = document.getElementById("p");`
	for _, c := range []struct{ name, script, want string }{
		{"an operation is enumerable, counts its arguments and has their number as its length", `[
			own(Node.prototype, "appendChild").enumerable, Node.prototype.appendChild.length,
			Node.prototype.cloneNode.length, AbortController.prototype.abort.length, thrown(() => p.appendChild()),
			thrown(() => p.contains()), thrown(() => document.contains()), thrown(() => p.attributes[0].contains()),
			p.appendChild.name].join()`, "true,1,0,0,TypeError,TypeError,TypeError,TypeError,appendChild"},
		{"an operation or an attribute asked of another object refuses", `[
			thrown(() => Node.prototype.hasChildNodes.call({})), thrown(() => Node.prototype.hasChildNodes.call(null)),
			thrown(() => Node.prototype.nodeType), thrown(() => own(Node.prototype, "nodeType").get.call({})),
			thrown(() => own(Event.prototype, "cancelBubble").set.call({}, true)),
			thrown(() => own(Element.prototype, "id").get.call(document)), own(Node.prototype, "nodeType").get.call(p),
			thrown(() => EventTarget.prototype.addEventListener.call(null, "x", null)),
			own(Node.prototype, "nodeType").enumerable].join()`,
			"TypeError,TypeError,TypeError,TypeError,TypeError,TypeError,1,none,true"},
		{"constants, tags and what a with block leaves out", `[own(Node, "ELEMENT_NODE").writable,
			own(Node.prototype, "TEXT_NODE").value, Object.prototype.toString.call(document.createRange()),
			Element.prototype[Symbol.unscopables].remove, Document.length, Event.length, "prototype" in NodeFilter,
			NodeFilter.SHOW_TEXT, thrown(() => NodeFilter()), thrown(() => new NodeFilter())].join()`,
			"false,3,[object Range],true,0,1,false,4,TypeError,TypeError"},
		{"what is iterable is walked by its values, which enumerate", `[NodeList.prototype[Symbol.iterator] ===
			Array.prototype.values, NodeList.prototype.forEach === Array.prototype.forEach,
			DOMTokenList.prototype.entries === Array.prototype.entries, own(NodeList.prototype, "entries").enumerable,
			own(DOMTokenList.prototype, "toString").enumerable, [...p.childNodes].length, [...p.classList.keys()].length,
			(() => { const seen = []; p.childNodes.forEach((n, i, all) => seen.push(i, all === p.childNodes));
			return seen })()].join()`, "true,true,true,true,true,1,0,0,true"},
		{"the interfaces between and beside", `[Object.getPrototypeOf(Range.prototype) === NodeRange.prototype,
			Object.getPrototypeOf(NodeRange.prototype) === AbstractRange.prototype, new StaticRange({ startContainer: p,
			startOffset: 0, endContainer: p, endOffset: 1 }) instanceof NodeRange, own(NodeRange.prototype,
			"startContainer") !== undefined, own(AbstractRange.prototype, "startContainer"), p.baseURI ===
			document.baseURI, typeof document.doctype.before, own(ProcessingInstruction.prototype, "target") !==
			undefined, thrown(() => new MutationRecord()), (() => { const o = new MutationObserver(() => {});
			o.observe(p, { childList: true }); p.append("more"); const [r] = o.takeRecords();
			return [r instanceof MutationRecord, r.type, r.addedNodes.length, r.target === p,
			Object.keys(r).length] })()].join()`, "true,true,true,true,,true,function,true,TypeError,true,childList,1,true,0"},
	} {
		if got := s.eval(tools + "\n" + c.script); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
	s.quiet()
}
