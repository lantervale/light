// ---- HTML elements

class HTMLElement extends Element {
  constructor(key, ref, tag) {
    if (key !== token) illegal();
    super(key, ref, tag);
  }
  get style() { return styleOf(this); }
  set style(text) { this.setAttribute("style", text); }
  get dataset() { return datasetOf(this); }
  get innerText() { return this.textContent; }
  set innerText(text) { this.textContent = text; }
  get outerText() { return this.textContent; }
  get offsetWidth() { return Math.round(host("box", this[REF])[2]); }
  get offsetHeight() { return Math.round(host("box", this[REF])[3]); }
  get offsetLeft() { return Math.round(host("box", this[REF])[0] + window.scrollX); }
  get offsetTop() { return Math.round(host("box", this[REF])[1] + window.scrollY); }
  get offsetParent() { return this === document.body || !this.isConnected ? null : document.body; }
  get isContentEditable() { return false; }
  get contentEditable() { return this.getAttribute("contenteditable") || "inherit"; }
  set contentEditable(value) { this.setAttribute("contenteditable", value); }
  focus() { host("focus", this[REF]); }
  blur() { host("blur", this[REF]); }
  click() {
    const event = new MouseEvent("click", { bubbles: true, cancelable: true, composed: true });
    if (dispatch(this, event)) host("activate", this[REF]);
  }
  get nonce() { return this.getAttribute("nonce") || ""; }
  set nonce(value) { this.setAttribute("nonce", value); }
}
reflect(HTMLElement.prototype, {
  title: "title", lang: "lang", dir: "dir", hidden: ["hidden", false], tabIndex: ["tabindex", -1],
  accessKey: "accesskey", draggable: ["draggable", false], role: "role", inert: ["inert", false],
});
withHandlers(HTMLElement.prototype);
class HTMLUnknownElement extends HTMLElement {}

// absolute is a reference as an address, from the page's.
const absolute = (reference) => {
  if (reference === null || reference === "") return "";
  const parsed = host("url", String(reference), document.baseURI);
  return parsed ? parsed.href : String(reference);
};
// located makes a property an address, absolute when read.
const located = (prototype, properties) => {
  for (const name of Object.keys(properties)) {
    const attribute = properties[name];
    Object.defineProperty(prototype, name, {
      get() { return absolute(this.getAttribute(attribute)); },
      set(value) { this.setAttribute(attribute, value); },
      configurable: true, enumerable: true,
    });
  }
};
// formOf is the form a control is in.
const formOf = { get() { return this.closest("form"); }, configurable: true };

class HTMLAnchorElement extends HTMLElement {
  toString() { return this.href; }
}
located(HTMLAnchorElement.prototype, { href: "href" });
reflect(HTMLAnchorElement.prototype, { target: "target", rel: "rel", download: "download", hreflang: "hreflang", type: "type" });
for (const part of ["protocol", "host", "hostname", "port", "pathname", "search", "hash", "origin"]) {
  Object.defineProperty(HTMLAnchorElement.prototype, part, {
    get() {
      const parsed = host("url", this.getAttribute("href") || "", document.baseURI);
      return parsed ? parsed[part] : "";
    },
    configurable: true, enumerable: true,
  });
}
accessors(HTMLAnchorElement.prototype, { text: { get() { return this.textContent; }, set(v) { this.textContent = v; } } });

class HTMLImageElement extends HTMLElement {
  get complete() { return true; }
  get naturalWidth() { return this.offsetWidth; }
  get naturalHeight() { return this.offsetHeight; }
  get width() { return this.offsetWidth || Number(this.getAttribute("width")) || 0; }
  set width(value) { this.setAttribute("width", value); }
  get height() { return this.offsetHeight || Number(this.getAttribute("height")) || 0; }
  set height(value) { this.setAttribute("height", value); }
  get currentSrc() { return absolute(host("shownSrc", this[REF])); }
  decode() { return Promise.resolve(); }
}
located(HTMLImageElement.prototype, { src: "src" });
reflect(HTMLImageElement.prototype, { alt: "alt", srcset: "srcset", sizes: "sizes", loading: "loading", crossOrigin: "crossorigin" });

// valued is the value, the state and the selection of a form control,
// which ui keeps apart from its attributes.
const valued = {
  value: {
    get() { return host("value", this[REF]); },
    set(value) { host("setValue", this[REF], value === null ? "" : String(value)); },
  },
  defaultValue: {
    get() { return this.getAttribute("value") || ""; },
    set(value) { this.setAttribute("value", value); },
  },
  form: formOf,
  labels: { get() { return this.id ? document.querySelectorAll(`label[for="${CSS.escape(this.id)}"]`) : new NodeList(); } },
  validity: { get() { return { valid: true, valueMissing: false, typeMismatch: false, patternMismatch: false, customError: false }; } },
  validationMessage: { get() { return ""; } },
  willValidate: { get() { return false; } },
  selectionStart: { get() { return host("selection", this[REF])[0]; }, set(at) { this.setSelectionRange(at, this.selectionEnd); } },
  selectionEnd: { get() { return host("selection", this[REF])[1]; }, set(at) { this.setSelectionRange(this.selectionStart, at); } },
};
const valuedMethods = {
  checkValidity() { return true; },
  reportValidity() { return true; },
  setCustomValidity() {},
  select() { host("select", this[REF], 0, -1); },
  setSelectionRange(from, to) { host("select", this[REF], Number(from) || 0, Number(to) || 0); },
};
const control = { name: "name", disabled: ["disabled", false], required: ["required", false], autofocus: ["autofocus", false] };

class HTMLInputElement extends HTMLElement {
  get type() { return (this.getAttribute("type") || "text").toLowerCase(); }
  set type(value) { this.setAttribute("type", value); }
  get checked() { return host("checked", this[REF]); }
  set checked(on) { host("setChecked", this[REF], Boolean(on)); }
  get defaultChecked() { return this.hasAttribute("checked"); }
  set defaultChecked(on) { this.toggleAttribute("checked", Boolean(on)); }
  get valueAsNumber() { return this.value === "" ? NaN : Number(this.value); }
  set valueAsNumber(value) { this.value = String(value); }
  get files() { return null; }
  get list() { return null; }
  get indeterminate() { return false; }
  set indeterminate(_) {}
  stepUp(n = 1) { this.value = String(Number(this.value || 0) + n * (Number(this.step) || 1)); }
  stepDown(n = 1) { this.stepUp(-n); }
}
accessors(HTMLInputElement.prototype, valued);
define(HTMLInputElement.prototype, valuedMethods);
reflect(HTMLInputElement.prototype, {
  ...control, placeholder: "placeholder", readOnly: ["readonly", false], min: "min", max: "max", step: "step",
  pattern: "pattern", maxLength: ["maxlength", -1], minLength: ["minlength", -1], size: ["size", 20],
  autocomplete: "autocomplete", multiple: ["multiple", false], accept: "accept", alt: "alt",
});
located(HTMLInputElement.prototype, { src: "src" });

class HTMLTextAreaElement extends HTMLElement {
  get type() { return "textarea"; }
  get textLength() { return this.value.length; }
}
accessors(HTMLTextAreaElement.prototype, valued);
define(HTMLTextAreaElement.prototype, valuedMethods);
accessors(HTMLTextAreaElement.prototype, {
  defaultValue: { get() { return this.textContent; }, set(value) { this.textContent = value; } },
});
reflect(HTMLTextAreaElement.prototype, {
  ...control, placeholder: "placeholder", readOnly: ["readonly", false], rows: ["rows", 2], cols: ["cols", 20],
  maxLength: ["maxlength", -1], wrap: "wrap",
});

class HTMLButtonElement extends HTMLElement {
  get type() {
    const type = (this.getAttribute("type") || "submit").toLowerCase();
    return ["submit", "reset", "button"].includes(type) ? type : "submit";
  }
  set type(value) { this.setAttribute("type", value); }
}
accessors(HTMLButtonElement.prototype, { form: formOf });
reflect(HTMLButtonElement.prototype, { ...control, value: "value" });
define(HTMLButtonElement.prototype, valuedMethods);

class HTMLSelectElement extends HTMLElement {
  get type() { return this.multiple ? "select-multiple" : "select-one"; }
  get options() { return HTMLCollection.from(this.querySelectorAll("option")); }
  get length() { return this.options.length; }
  get selectedIndex() { return host("selected", this[REF]); }
  set selectedIndex(i) { host("setSelected", this[REF], Number(i)); }
  get selectedOptions() {
    const chosen = this.options[this.selectedIndex];
    return HTMLCollection.from(chosen ? [chosen] : []);
  }
  get value() {
    const chosen = this.options[this.selectedIndex];
    return chosen ? chosen.value : "";
  }
  set value(value) {
    const options = this.options;
    this.selectedIndex = options.findIndex((o) => o.value === String(value));
  }
  item(i) { return this.options.item(i); }
  add(option, before) { this.insertBefore(option, before instanceof Node ? before : null); }
}
accessors(HTMLSelectElement.prototype, { form: formOf });
reflect(HTMLSelectElement.prototype, { ...control, multiple: ["multiple", false], size: ["size", 0] });
define(HTMLSelectElement.prototype, valuedMethods);

class HTMLOptionElement extends HTMLElement {
  get value() {
    const value = this.getAttribute("value");
    return value === null ? this.textContent.trim() : value;
  }
  set value(value) { this.setAttribute("value", value); }
  get text() { return this.textContent.trim(); }
  set text(value) { this.textContent = value; }
  get label() { return this.getAttribute("label") || this.text; }
  get index() {
    const select = this.closest("select");
    return select ? select.options.indexOf(this) : 0;
  }
  get selected() {
    const select = this.closest("select");
    return select ? select.selectedIndex === this.index : this.hasAttribute("selected");
  }
  set selected(on) {
    const select = this.closest("select");
    if (select && on) select.selectedIndex = this.index;
    else this.toggleAttribute("selected", Boolean(on));
  }
  get defaultSelected() { return this.hasAttribute("selected"); }
}
accessors(HTMLOptionElement.prototype, { form: formOf });
reflect(HTMLOptionElement.prototype, { disabled: ["disabled", false] });

class HTMLFormElement extends HTMLElement {
  get elements() {
    const controls = this.querySelectorAll("input, select, textarea, button, fieldset, object, output");
    return HTMLFormControlsCollection.from(controls.filter((n) => n.getAttribute("type") !== "image"));
  }
  get length() { return this.elements.length; }
  submit() { host("submit", this[REF], 0); }
  requestSubmit(submitter) {
    const event = new SubmitEvent("submit", { bubbles: true, cancelable: true, submitter: submitter || null });
    if (dispatch(this, event)) host("submit", this[REF], submitter ? refOf(submitter, "the submitter") : 0);
  }
  reset() {
    if (!dispatch(this, new Event("reset", { bubbles: true, cancelable: true }))) return;
    for (const field of this.elements) {
      if (field instanceof HTMLInputElement && /^(checkbox|radio)$/.test(field.type)) field.checked = field.defaultChecked;
      else if (field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement) field.value = field.defaultValue;
    }
  }
  checkValidity() { return true; }
  reportValidity() { return true; }
}
byName(HTMLFormElement.prototype, (form, name) => {
  const control = form.elements.namedItem(name);
  return control || form.querySelector(`img[name="${CSS.escape(name)}"], img[id="${CSS.escape(name)}"]`);
});
located(HTMLFormElement.prototype, { action: "action" });
reflect(HTMLFormElement.prototype, {
  method: ["method", "get"], name: "name", target: "target", enctype: "enctype", noValidate: ["novalidate", false],
  acceptCharset: "accept-charset", autocomplete: "autocomplete",
});

class HTMLLabelElement extends HTMLElement {
  get control() {
    const id = this.getAttribute("for");
    return id ? document.getElementById(id) : this.querySelector("input, select, textarea, button");
  }
}
accessors(HTMLLabelElement.prototype, { form: formOf });
reflect(HTMLLabelElement.prototype, { htmlFor: "for" });

class HTMLTemplateElement extends HTMLElement {
  get content() { return wrap(host("content", this[REF])); }
}
class HTMLScriptElement extends HTMLElement {
  get text() { return this.textContent; }
  set text(source) { this.textContent = source; }
  static supports(type) { return type === "classic" || type === "module" || type === "importmap"; }
}
located(HTMLScriptElement.prototype, { src: "src" });
reflect(HTMLScriptElement.prototype, {
  type: "type", async: ["async", false], defer: ["defer", false], noModule: ["nomodule", false],
  crossOrigin: "crossorigin", integrity: "integrity", charset: "charset", referrerPolicy: "referrerpolicy",
});
class HTMLStyleElement extends HTMLElement {
  get sheet() { return null; }
}
reflect(HTMLStyleElement.prototype, { media: "media", type: "type" });
class HTMLLinkElement extends HTMLElement {
  get sheet() { return null; }
  get relList() { return new DOMTokenList(this, "rel"); }
}
located(HTMLLinkElement.prototype, { href: "href" });
reflect(HTMLLinkElement.prototype, { rel: "rel", media: "media", type: "type", as: "as", crossOrigin: "crossorigin" });
class HTMLMetaElement extends HTMLElement {}
reflect(HTMLMetaElement.prototype, { name: "name", content: "content", httpEquiv: "http-equiv" });
class HTMLTitleElement extends HTMLElement {
  get text() { return this.textContent; }
  set text(value) { this.textContent = value; }
}
class HTMLBaseElement extends HTMLElement {}
located(HTMLBaseElement.prototype, { href: "href" });
class HTMLDetailsElement extends HTMLElement {}
reflect(HTMLDetailsElement.prototype, { open: ["open", false] });
class HTMLDialogElement extends HTMLElement {
  show() { this.open = true; }
  showModal() { this.open = true; }
  close(value) {
    this.open = false;
    this.returnValue = value === undefined ? "" : String(value);
    dispatch(this, new Event("close"));
  }
}
reflect(HTMLDialogElement.prototype, { open: ["open", false] });
class HTMLCanvasElement extends HTMLElement {
  getContext() { return null; }
  toDataURL() { return "data:,"; }
}
reflect(HTMLCanvasElement.prototype, { width: ["width", 300], height: ["height", 150] });
// An iframe's window is the facade of 57-windows.js, whatever its
// document's origin: light runs each frame's scripts in an engine of its
// own, so a page reaches no document but its own.
class HTMLIFrameElement extends HTMLElement {
  get contentWindow() { return windowOf(host("window", "of", this[REF])); }
  get contentDocument() { return null; }
  get sandbox() {
    if (!this._sandbox) define(this, { _sandbox: new DOMTokenList(this, "sandbox") });
    return this._sandbox;
  }
  set sandbox(value) { this.setAttribute("sandbox", value); }
}
located(HTMLIFrameElement.prototype, { src: "src" });
reflect(HTMLIFrameElement.prototype, { srcdoc: "srcdoc", name: "name", width: "width", height: "height" });
class HTMLMediaElement extends HTMLElement {
  play() { return Promise.reject(new DOMException("light plays no media.", "NotSupportedError")); }
  pause() {}
  load() {}
  canPlayType() { return ""; }
  get paused() { return true; }
}
located(HTMLMediaElement.prototype, { src: "src" });
class HTMLVideoElement extends HTMLMediaElement {}
class HTMLAudioElement extends HTMLMediaElement {}
class HTMLTableElement extends HTMLElement {
  get rows() { return HTMLCollection.from(this.querySelectorAll("tr")); }
  get tBodies() { return HTMLCollection.from(this.querySelectorAll("tbody")); }
}
class HTMLTableRowElement extends HTMLElement {
  get cells() { return HTMLCollection.from(this.children.filter((c) => /^t[dh]$/.test(c.localName))); }
}
class HTMLTableCellElement extends HTMLElement {}
reflect(HTMLTableCellElement.prototype, { colSpan: ["colspan", 1], rowSpan: ["rowspan", 1] });
class HTMLProgressElement extends HTMLElement {}
reflect(HTMLProgressElement.prototype, { value: ["value", 0], max: ["max", 1] });

const named = (...names) => names.map((name) => ({ [name]: class extends HTMLElement {} })[name]);
const [HTMLDivElement, HTMLSpanElement, HTMLParagraphElement, HTMLHeadingElement, HTMLUListElement, HTMLOListElement,
  HTMLLIElement, HTMLBodyElement, HTMLHtmlElement, HTMLHeadElement, HTMLPreElement, HTMLBRElement, HTMLHRElement,
  HTMLFieldSetElement, HTMLLegendElement, HTMLQuoteElement, HTMLTableSectionElement, HTMLMeterElement,
  HTMLDListElement, HTMLTimeElement, HTMLDataElement, HTMLPictureElement, HTMLSourceElement, HTMLObjectElement,
  HTMLEmbedElement, HTMLSlotElement, HTMLOptGroupElement, HTMLOutputElement, HTMLMenuElement, HTMLFontElement] = named(
  "HTMLDivElement", "HTMLSpanElement", "HTMLParagraphElement", "HTMLHeadingElement", "HTMLUListElement",
  "HTMLOListElement", "HTMLLIElement", "HTMLBodyElement", "HTMLHtmlElement", "HTMLHeadElement", "HTMLPreElement",
  "HTMLBRElement", "HTMLHRElement", "HTMLFieldSetElement", "HTMLLegendElement", "HTMLQuoteElement",
  "HTMLTableSectionElement", "HTMLMeterElement", "HTMLDListElement", "HTMLTimeElement", "HTMLDataElement",
  "HTMLPictureElement", "HTMLSourceElement", "HTMLObjectElement", "HTMLEmbedElement", "HTMLSlotElement",
  "HTMLOptGroupElement", "HTMLOutputElement", "HTMLMenuElement", "HTMLFontElement");
withHandlers(HTMLBodyElement.prototype, ["load", "resize", "hashchange", "popstate", "beforeunload", "unload", "scroll"]);

const classes = {
  a: HTMLAnchorElement, img: HTMLImageElement, input: HTMLInputElement, textarea: HTMLTextAreaElement,
  button: HTMLButtonElement, select: HTMLSelectElement, option: HTMLOptionElement, form: HTMLFormElement,
  label: HTMLLabelElement, template: HTMLTemplateElement, script: HTMLScriptElement, style: HTMLStyleElement,
  link: HTMLLinkElement, meta: HTMLMetaElement, title: HTMLTitleElement, base: HTMLBaseElement,
  details: HTMLDetailsElement, dialog: HTMLDialogElement, canvas: HTMLCanvasElement, iframe: HTMLIFrameElement,
  video: HTMLVideoElement, audio: HTMLAudioElement, table: HTMLTableElement, tr: HTMLTableRowElement,
  td: HTMLTableCellElement, th: HTMLTableCellElement, progress: HTMLProgressElement, div: HTMLDivElement,
  span: HTMLSpanElement, p: HTMLParagraphElement, h1: HTMLHeadingElement, h2: HTMLHeadingElement,
  h3: HTMLHeadingElement, h4: HTMLHeadingElement, h5: HTMLHeadingElement, h6: HTMLHeadingElement,
  ul: HTMLUListElement, ol: HTMLOListElement, li: HTMLLIElement, body: HTMLBodyElement, html: HTMLHtmlElement,
  head: HTMLHeadElement, pre: HTMLPreElement, br: HTMLBRElement, hr: HTMLHRElement, fieldset: HTMLFieldSetElement,
  legend: HTMLLegendElement, blockquote: HTMLQuoteElement, q: HTMLQuoteElement, thead: HTMLTableSectionElement,
  tbody: HTMLTableSectionElement, tfoot: HTMLTableSectionElement, meter: HTMLMeterElement, dl: HTMLDListElement,
  time: HTMLTimeElement, data: HTMLDataElement, picture: HTMLPictureElement, source: HTMLSourceElement,
  object: HTMLObjectElement, embed: HTMLEmbedElement, slot: HTMLSlotElement, optgroup: HTMLOptGroupElement,
  output: HTMLOutputElement, menu: HTMLMenuElement, font: HTMLFontElement,
};
// known are the tags of HTML that have no class of their own.
const known = new Set(["section", "header", "footer", "nav", "main", "aside", "article", "strong", "em", "code",
  "kbd", "samp", "summary", "caption", "dt", "dd", "figure", "figcaption", "address", "center", "hgroup", "search",
  "b", "i", "u", "s", "small", "big", "sub", "sup", "abbr", "cite", "mark", "var", "del", "ins", "dfn", "bdi", "bdo",
  "noscript", "svg", "wbr", "ruby", "rt", "rp", "nobr"]);
const classOf = (tag) => classes[tag] || (known.has(tag) || tag.includes("-") ? HTMLElement : HTMLUnknownElement);

// An SVG element is in the page's tree, where scripts read and build
// it; light draws none.
class SVGElement extends Element {
  get style() { return styleOf(this); }
  set style(text) { this.setAttribute("style", text); }
  get dataset() { return datasetOf(this); }
  get ownerSVGElement() { return this.closest("svg"); }
  get className() { return { baseVal: this.getAttribute("class") || "", animVal: this.getAttribute("class") || "" }; }
  focus() {}
  blur() {}
  getBBox() { return new DOMRect(); }
}
withHandlers(SVGElement.prototype);

// ---- the document

// The page's document is the one its window shows. A document a script
// makes (35-documents.js) is whole but has no window: nothing of it is
// fetched or shown, and no script of it runs.
const infoOf = (doc) => doc[INFO] || (doc[INFO] = host("document", "info", doc[REF]));
// The names a script may give what it creates.
const validLocal = (name) => /^[A-Za-z][^\t\n\f\r \0/>]*$/.test(name) || /^[:_\u0080-\uFFFF][-.\w:\u0080-\uFFFF]*$/.test(name);
const validPrefix = (prefix) => prefix !== "" && !/[\t\n\f\r \0/>]/.test(prefix);
const xmlNameStart = ":A-Z_a-z\\u00C0-\\u00D6\\u00D8-\\u00F6\\u00F8-\\u02FF\\u0370-\\u037D\\u037F-\\u1FFF\\u200C\\u200D\\u2070-\\u218F" +
  "\\u2C00-\\u2FEF\\u3001-\\uD7FF\\uF900-\\uFDCF\\uFDF0-\\uFFFD\\uD800-\\uDB7F\\uDC00-\\uDFFF";
const xmlName = new RegExp(`^[${xmlNameStart}][-.0-9\\u00B7\\u0300-\\u036F\\u203F\\u2040${xmlNameStart}]*$`);
const invalid = (what, name) => { throw new DOMException(`The ${what} provided ('${name}') is not a valid name.`, "InvalidCharacterError"); };
// qualified is a qualified name in a namespace as light keeps it: the
// namespace as the tree names it, and the local name.
const qualified = (uri, name) => {
  uri = uri === null || uri === undefined || uri === "" ? null : String(uri);
  name = String(name);
  const colon = name.indexOf(":");
  const prefix = colon < 0 ? null : name.slice(0, colon);
  const local = colon < 0 ? name : name.slice(colon + 1);
  if ((prefix !== null && !validPrefix(prefix)) || !validLocal(local)) invalid("qualified name", name);
  if ((prefix !== null && uri === null) || (prefix === "xml" && uri !== XML_NAMESPACE) ||
    ((name === "xmlns" || prefix === "xmlns") !== (uri === XMLNS_NAMESPACE))) {
    throw new DOMException(`The namespace '${uri}' does not go with the name '${name}'.`, "NamespaceError");
  }
  return [spaceOf(uri), local, prefix];
};

class Document extends Node {
  constructor(key, ref) {
    if (key !== token) {
      const made = host("document", "new", "application/xml");
      super(token, made);
      wrappers.set(made, this);
    } else if (ref === undefined) {
      super(token, pageRef);
      wrappers.set(pageRef, this);
      this[PAGE] = true;
      this[INFO] = pageInfo;
    } else {
      super(token, ref);
    }
    if (!this[PAGE]) {
      strangers = true;
      xmlAbout = xmlAbout || infoOf(this)[1];
    }
    this.readyState = this[PAGE] ? "loading" : "complete";
    this.currentScript = null;
  }
  get nodeType() { return DOCUMENT_NODE; }
  get nodeName() { return "#document"; }
  get ownerDocument() { return null; }
  get isConnected() { return true; }
  get parentNode() { return null; }
  get children() { return HTMLCollection.from(this.childNodes.filter((n) => n instanceof Element)); }
  get firstElementChild() { return this.documentElement; }
  get lastElementChild() { return this.documentElement; }
  get childElementCount() { return this.children.length; }
  get nextSibling() { return null; }
  get previousSibling() { return null; }
  get textContent() { return null; }
  set textContent(_) {}
  get documentElement() { return wrap(host("document", "parts", this[REF])[0]); }
  get head() { return wrap(host("document", "parts", this[REF])[1]); }
  get body() { return wrap(host("document", "parts", this[REF])[2]); }
  set body(_) {}
  get scrollingElement() { return this[PAGE] ? this.documentElement : null; }
  get defaultView() { return this[PAGE] ? window : null; }
  get location() { return this[PAGE] ? window.location : null; }
  set location(address) { if (this[PAGE]) window.location.href = address; }
  get URL() { return this[PAGE] ? window.location.href : infoOf(this)[2] || "about:blank"; }
  get documentURI() { return this.URL; }
  get baseURI() {
    if (!this[PAGE]) return this.URL;
    const base = this.querySelector("base[href]");
    const href = base ? host("url", base.getAttribute("href"), window.location.href) : null;
    return href ? href.href : window.location.href;
  }
  get domain() { return this[PAGE] ? window.location.hostname : ""; }
  get referrer() { return ""; }
  get characterSet() { return "UTF-8"; }
  get charset() { return "UTF-8"; }
  get inputEncoding() { return "UTF-8"; }
  get compatMode() { return this[PAGE] || infoOf(this)[1] || this.doctype ? "CSS1Compat" : "BackCompat"; }
  get contentType() { return infoOf(this)[0]; }
  get doctype() { return this.childNodes.find((n) => n instanceof DocumentType) || null; }
  get hidden() { return !this[PAGE]; }
  get visibilityState() { return this[PAGE] ? "visible" : "hidden"; }
  get fullscreenElement() { return null; }
  get styleSheets() { return []; }
  get fonts() {
    const ready = Promise.resolve();
    return { ready, status: "loaded", load: () => Promise.resolve([]), check: () => true, addEventListener() {}, forEach() {} };
  }
  get title() {
    if (this[PAGE]) return host("title");
    const title = this.querySelector("title");
    return title ? title.textContent.replace(/[\t\n\f\r ]+/g, " ").trim() : "";
  }
  set title(text) {
    if (this[PAGE]) {
      host("setTitle", String(text));
      return;
    }
    let title = this.querySelector("title");
    if (!title && this.head) title = this.head.appendChild(this.createElement("title"));
    if (title) title.textContent = String(text);
  }
  get cookie() { return this[PAGE] ? host("cookie") : ""; }
  set cookie(text) { if (this[PAGE]) host("setCookie", String(text)); }
  get activeElement() { return (this[PAGE] && wrap(host("active"))) || this.body; }
  hasFocus() { return Boolean(this[PAGE]); }
  get scripts() { return HTMLCollection.from(this.querySelectorAll("script")); }
  get forms() { return HTMLCollection.from(this.querySelectorAll("form")); }
  get images() { return HTMLCollection.from(this.querySelectorAll("img")); }
  get links() { return HTMLCollection.from(this.querySelectorAll("a[href]")); }
  get anchors() { return HTMLCollection.from(this.querySelectorAll("a[name]")); }
  get all() { return HTMLCollection.from(this.querySelectorAll("*")); }
  get implementation() {
    if (!this._implementation) define(this, { _implementation: new DOMImplementation(token, this) });
    return this._implementation;
  }

  getElementById(id) { return wrap(host("document", "byId", this[REF], String(id))); }
  querySelector(selector) { return wrap(host("query", this[PAGE] ? 0 : this[REF], String(selector), false)); }
  querySelectorAll(selector) { return wrapAll(host("query", this[PAGE] ? 0 : this[REF], String(selector), true)); }
  getElementsByTagName(tag) { return tagged(this, tag); }
  getElementsByTagNameNS(uri, tag) { return tagged(this, tag, uri === "*" ? "*" : spaceOf(uri)); }
  getElementsByClassName(names) {
    const classes = String(names).split(/\s+/).filter(Boolean);
    if (!classes.length) return new HTMLCollection();
    return HTMLCollection.from(this.querySelectorAll(classes.map((c) => "." + CSS.escape(c)).join("")));
  }
  getElementsByName(name) { return this.querySelectorAll(`[name="${CSS.escape(String(name))}"]`); }
  // An element a document of HTML makes is named in lower case, and one
  // a document of XML makes as it is written: in HTML's namespace for
  // XHTML, in none for another type.
  createElement(tag) {
    tag = String(tag);
    if (!validLocal(tag)) invalid("tag name", tag);
    const [media, xml] = infoOf(this);
    return wrap(host("create", xml ? tag : tag.toLowerCase(), !xml || media === "application/xhtml+xml" ? "" : NO_NS, this[REF]));
  }
  createElementNS(uri, name) {
    const [space, local, prefix] = qualified(uri, name);
    prefixed = prefixed || prefix !== null;
    return wrap(host("create", local, space, this[REF], prefix));
  }
  createTextNode(text) { return wrap(host("createText", String(text), this[REF])); }
  createComment(text) { return wrap(host("createComment", text === undefined ? "" : String(text), this[REF])); }
  createDocumentFragment() { return wrap(host("create", FRAGMENT, "", this[REF])); }
  createCDATASection(text) {
    if (!infoOf(this)[1]) throw new DOMException("This operation is not supported for HTML documents.", "NotSupportedError");
    text = String(text);
    if (text.includes("]]>")) throw new DOMException("A CDATA section cannot hold ']]>'.", "InvalidCharacterError");
    return wrap(host("document", "cdata", text, this[REF]));
  }
  createProcessingInstruction(target, text) {
    target = String(target);
    text = String(text);
    if (!xmlName.test(target)) invalid("target", target);
    if (text.includes("?>")) throw new DOMException("A processing instruction cannot hold '?>'.", "InvalidCharacterError");
    return wrap(host("document", "pi", target, text, this[REF]));
  }
  createAttribute(name) {
    name = String(name);
    if (name === "" || /[\t\n\f\r \0/=>]/.test(name)) invalid("attribute name", name);
    return new Attr(null, infoOf(this)[1] ? name : name.toLowerCase(), "");
  }
  createAttributeNS(uri, name) {
    qualified(uri, name);
    return new Attr(null, String(name), "");
  }
  createEvent(kind) {
    const constructors = { customevent: CustomEvent, mouseevent: MouseEvent, mouseevents: MouseEvent,
      keyboardevent: KeyboardEvent, uievent: UIEvent, uievents: UIEvent };
    return new (constructors[String(kind).toLowerCase()] || Event)("");
  }
  createRange() { return new Range(); }
  createTreeWalker(root, show = 0xFFFFFFFF, filter = null) { return new TreeWalker(root, show, filter); }
  createNodeIterator(root, show = 0xFFFFFFFF, filter = null) { return new TreeWalker(root, show, filter); }
  // A node of another document comes into this one as a copy
  // (importNode) or itself (adoptNode); a document does neither.
  importNode(node, deep) {
    if (node instanceof Document) throw new DOMException("The node provided is a document, which may not be imported.", "NotSupportedError");
    return this.adoptNode(node.cloneNode(deep));
  }
  adoptNode(node) {
    if (node instanceof Document) throw new DOMException("The node provided is of type '#document', which may not be adopted.", "NotSupportedError");
    const ref = refOf(node);
    detach(node);
    host("document", "adopt", this[REF], ref);
    return node;
  }
  cloneNode(deep) { return wrap(host("document", "clone", this[REF], Boolean(deep))); }
  evaluate(...args) { return new XPathEvaluator().evaluate(...args); }
  elementFromPoint(x, y) { return this[PAGE] ? wrap(host("at", Number(x), Number(y))) : null; }
  elementsFromPoint(x, y) {
    const list = [];
    for (let n = this.elementFromPoint(x, y); n; n = n.parentElement) list.push(n);
    return list;
  }
  getSelection() { return this[PAGE] ? window.getSelection() : null; }
  execCommand() { return false; }
  queryCommandSupported() { return false; }
  write() { host("log", "warn", "document.write does nothing in light: the page is already built"); }
  writeln() { this.write(); }
  open() { return this; }
  close() {}
  exitFullscreen() { return Promise.resolve(); }
  append(...items) {
    if (this[PAGE] && this.documentElement) this.documentElement.append(...items);
    else if (items.length) insert(this, nodesOf(items), null);
  }
  contains(other) {
    return other === this || (other instanceof Node && other[REF] !== undefined && host("contains", this[REF], other[REF]));
  }
}
withHandlers(Document.prototype);

// Range is the little of it scripts use to build markup and select.
class Range {
  constructor() {
    this.startContainer = this.endContainer = this.commonAncestorContainer = document;
    this.startOffset = this.endOffset = 0;
    this.collapsed = true;
  }
  createContextualFragment(markup) {
    const within = this.startContainer instanceof Element ? this.startContainer.localName : "div";
    const fragment = document.createDocumentFragment();
    for (const node of wrapAll(host("fragment", String(markup), within))) fragment.appendChild(node);
    return fragment;
  }
  selectNode(node) { this.startContainer = this.endContainer = this.commonAncestorContainer = node.parentNode || node; }
  selectNodeContents(node) { this.startContainer = this.endContainer = this.commonAncestorContainer = node; }
  setStart(node, offset) { this.startContainer = node; this.startOffset = offset; }
  setEnd(node, offset) { this.endContainer = node; this.endOffset = offset; }
  setStartBefore(node) { this.setStart(node.parentNode, 0); }
  setStartAfter(node) { this.setStart(node.parentNode, 0); }
  setEndBefore(node) { this.setEnd(node.parentNode, 0); }
  setEndAfter(node) { this.setEnd(node.parentNode, 0); }
  collapse() { this.collapsed = true; }
  cloneRange() { return Object.assign(new Range(), this); }
  deleteContents() {
    if (this.commonAncestorContainer instanceof Element) this.commonAncestorContainer.replaceChildren();
  }
  getBoundingClientRect() {
    return this.startContainer instanceof Element ? this.startContainer.getBoundingClientRect() : new DOMRect();
  }
  getClientRects() { return []; }
  detach() {}
  toString() { return this.commonAncestorContainer.textContent || ""; }
}

const SHOW_ELEMENT = 1, SHOW_TEXT = 4, SHOW_COMMENT = 128;
const FILTER_ACCEPT = 1, FILTER_REJECT = 2, FILTER_SKIP = 3;
const NodeFilter = {
  SHOW_ALL: 0xFFFFFFFF, SHOW_ELEMENT, SHOW_TEXT, SHOW_COMMENT, FILTER_ACCEPT, FILTER_REJECT, FILTER_SKIP,
};
class TreeWalker {
  constructor(root, whatToShow, filter) {
    this.root = root;
    this.currentNode = root;
    this.whatToShow = whatToShow;
    this.filter = filter;
  }
  _accepts(node) {
    const bit = node.nodeType === ELEMENT_NODE ? SHOW_ELEMENT : node.nodeType === TEXT_NODE ? SHOW_TEXT : SHOW_COMMENT;
    if (!(this.whatToShow & bit)) return FILTER_SKIP;
    if (!this.filter) return FILTER_ACCEPT;
    return typeof this.filter === "function" ? this.filter(node) : this.filter.acceptNode(node);
  }
  // _next is the node after one in document order, its subtree skipped
  // when asked, never leaving the root.
  _next(node, skip) {
    if (!skip && node.firstChild) return node.firstChild;
    for (let n = node; n && n !== this.root; n = n.parentNode) {
      if (n.nextSibling) return n.nextSibling;
    }
    return null;
  }
  nextNode() {
    let node = this._next(this.currentNode, false);
    while (node) {
      const verdict = this._accepts(node);
      if (verdict === FILTER_ACCEPT) {
        this.currentNode = node;
        return node;
      }
      node = this._next(node, verdict === FILTER_REJECT);
    }
    return null;
  }
  firstChild() {
    for (const child of this.currentNode.childNodes) {
      if (this._accepts(child) === FILTER_ACCEPT) {
        this.currentNode = child;
        return child;
      }
    }
    return null;
  }
  nextSibling() {
    for (let n = this.currentNode.nextSibling; n; n = n.nextSibling) {
      if (this._accepts(n) === FILTER_ACCEPT) {
        this.currentNode = n;
        return n;
      }
    }
    return null;
  }
  parentNode() {
    const parent = this.currentNode.parentNode;
    if (!parent || this.currentNode === this.root) return null;
    this.currentNode = parent;
    return parent;
  }
}
