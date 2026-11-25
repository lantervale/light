// ---- attributes as properties

// A property that reflects an attribute reads it by the rules of its
// kind, as HTML says each is parsed, and writes it back as text. The
// kinds are below, then the elements of HTML that had no interface of
// their own, then one table of every interface's attributes.

const MAX_LONG = 2147483647;
const integerIn = (text) => {
  const found = /^[\t\n\f\r ]*([-+]?[0-9]+)/.exec(text);
  return found ? Number(found[1]) + 0 : null;
};
const realIn = (text) => {
  const found = /^[\t\n\f\r ]*([-+]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][-+]?[0-9]+)?)/.exec(text);
  const number = found ? Number(found[1]) : NaN;
  return Number.isFinite(number) ? number + 0 : null;
};
const asciiLower = (text) => text.replace(/[A-Z]/g, (c) => c.toLowerCase());
const fallbackOf = (element, fallback) => (typeof fallback === "function" ? fallback(element) : fallback);
const outOfRange = (name, value) => {
  throw new DOMException(`The value provided (${value}) is not one '${name}' may have.`, "IndexSizeError");
};
const finite = (value) => {
  const number = Number(value);
  if (!Number.isFinite(number)) throw new TypeError("The provided double value is non-finite.");
  return number;
};

const text = (attribute) => ({
  get() {
    const value = ownAttr(this, attribute);
    return value === null ? "" : value;
  },
  set(value) { this.setAttribute(attribute, value); },
});
// legacyText is text that takes null as none, as the attributes of
// old pages do.
const legacyText = (attribute) => ({
  get: text(attribute).get,
  set(value) { this.setAttribute(attribute, value === null ? "" : value); },
});
const flag = (attribute) => ({
  get() { return (ownAttr(this, attribute) !== null); },
  set(on) { this.toggleAttribute(attribute, Boolean(on)); },
});
// forceAsync is a script's async: true for one a script made until
// it is set, whatever its attribute says.
const forceAsync = {
  get() { return host("async", this[REF], false); },
  set(on) {
    host("async", this[REF], true);
    this.toggleAttribute("async", Boolean(on));
  },
};
// numbered is a number read from its attribute, where it is one and in
// range, and written back unless the kind says not to.
const numbered = (attribute, read, written) => ({
  get() {
    const value = ownAttr(this, attribute);
    return read(value === null ? null : value, this);
  },
  set(value) {
    const kept = written(value, this);
    if (kept !== undefined) this.setAttribute(attribute, String(kept));
  },
});
const whole = (attribute, fallback = 0) => numbered(attribute, (value, element) => {
  const number = value === null ? null : integerIn(value);
  return number === null || number < -MAX_LONG - 1 || number > MAX_LONG ? fallbackOf(element, fallback) : number;
}, (value) => value | 0);
const inRange = (value, least, fallback) => {
  const number = value === null ? null : integerIn(value);
  return number === null || number < least || number > MAX_LONG ? fallback : number;
};
// count is a number that is not negative, -1 where there is none.
const count = (attribute) => numbered(attribute, (value) => inRange(value, 0, -1), (value) => {
  const number = value | 0;
  return number < 0 ? outOfRange(attribute, number) : number;
});
const natural = (attribute, fallback = 0) => numbered(attribute, (value) => inRange(value, 0, fallback), (value) => {
  const number = value >>> 0;
  return number > MAX_LONG ? fallback : number;
});
// positive is a number above zero, which refuses zero; positiveOr
// takes its fallback for it.
const positive = (attribute, fallback = 1) => numbered(attribute, (value) => inRange(value, 1, fallback), (value) => {
  const number = value >>> 0;
  if (number === 0) outOfRange(attribute, number);
  return number > MAX_LONG ? fallback : number;
});
const positiveOr = (attribute, fallback) => numbered(attribute, (value) => inRange(value, 1, fallback), (value) => {
  const number = value >>> 0;
  return number === 0 || number > MAX_LONG ? fallback : number;
});
const clamped = (attribute, least, most, fallback) => numbered(attribute, (value) => {
  const number = value === null ? null : integerIn(value);
  return number === null || number < 0 ? fallback : Math.min(Math.max(number, least), most);
}, (value) => {
  const number = value >>> 0;
  return number > MAX_LONG ? fallback : number;
});
const real = (attribute, fallback = 0) => numbered(attribute, (value) => {
  const number = value === null ? null : realIn(value);
  return number === null ? fallback : number;
}, finite);
// positiveReal keeps what it had when given zero or less.
const positiveReal = (attribute, fallback) => numbered(attribute, (value) => {
  const number = value === null ? null : realIn(value);
  return number === null || number <= 0 ? fallback : number;
}, (value) => {
  const number = finite(value);
  return number > 0 ? number : undefined;
});
// keyword is one of a few words, whatever its case: missing is what an
// element without the attribute says, invalid what one says whose word
// is none of them, alias the words that stand for another, and a
// nullable one is removed by null.
const keyword = (attribute, words, { missing = "", invalid = missing, alias = {}, nullable = false } = {}) => ({
  get() {
    const value = ownAttr(this, attribute);
    if (value === null) return missing;
    const said = asciiLower(value);
    if (Object.hasOwn(alias, said)) return alias[said];
    return words.includes(said) ? said : invalid;
  },
  set(value) {
    if (nullable && (value === null || value === undefined)) this.removeAttribute(attribute);
    else this.setAttribute(attribute, value);
  },
});
// resolved is a reference as an address, from its element's document.
const resolved = (element, reference) => {
  const parsed = host("url", reference, element.ownerDocument.baseURI);
  return parsed ? parsed.href : reference;
};
const located = (attribute) => ({
  get() {
    const value = ownAttr(this, attribute);
    return value === null ? "" : resolved(this, value);
  },
  set(value) { this.setAttribute(attribute, value); },
});
// A form is sent to its document where it names no address.
const sentTo = (attribute) => ({
  get() {
    const value = ownAttr(this, attribute);
    return value === null || value === "" ? this.ownerDocument.URL : resolved(this, value);
  },
  set(value) { this.setAttribute(attribute, value); },
});
// reflects gives a prototype its properties: a name alone is text, and
// a kind without a getter or a setter keeps the one the property had.
const reflects = (prototype, properties) => {
  for (const name of Object.keys(properties)) {
    const kind = typeof properties[name] === "string" ? text(properties[name]) : properties[name];
    const had = Object.getOwnPropertyDescriptor(prototype, name) || {};
    Object.defineProperty(prototype, name, {
      get: kind.get || had.get, set: kind.set || had.set, configurable: true, enumerable: true,
    });
  }
};
const setter = (kind) => ({ set: kind.set });

// ---- the elements of HTML without an interface of their own

const [HTMLAreaElement, HTMLMapElement, HTMLParamElement, HTMLTrackElement, HTMLModElement, HTMLTableCaptionElement,
  HTMLTableColElement, HTMLDataListElement, HTMLMarqueeElement, HTMLFrameSetElement, HTMLFrameElement,
  HTMLDirectoryElement] = named("HTMLAreaElement", "HTMLMapElement", "HTMLParamElement", "HTMLTrackElement",
  "HTMLModElement", "HTMLTableCaptionElement", "HTMLTableColElement", "HTMLDataListElement", "HTMLMarqueeElement",
  "HTMLFrameSetElement", "HTMLFrameElement", "HTMLDirectoryElement");
Object.assign(classes, {
  area: HTMLAreaElement, map: HTMLMapElement, param: HTMLParamElement, track: HTMLTrackElement, ins: HTMLModElement,
  del: HTMLModElement, caption: HTMLTableCaptionElement, col: HTMLTableColElement, colgroup: HTMLTableColElement,
  datalist: HTMLDataListElement, marquee: HTMLMarqueeElement, frameset: HTMLFrameSetElement, frame: HTMLFrameElement,
  dir: HTMLDirectoryElement, listing: HTMLPreElement, xmp: HTMLPreElement,
});
withHandlers(HTMLFrameSetElement.prototype, [...windowHandled]);
for (const tag of ["acronym", "basefont", "noembed", "noframes", "plaintext", "rb", "rtc", "strike", "tt"]) known.add(tag);
define(window, {
  HTMLAreaElement, HTMLMapElement, HTMLParamElement, HTMLTrackElement, HTMLModElement, HTMLTableCaptionElement,
  HTMLTableColElement, HTMLDataListElement, HTMLMarqueeElement, HTMLFrameSetElement, HTMLFrameElement,
  HTMLDirectoryElement,
});

// ---- what every element says of itself

// A nonce a script sets is kept off the markup, where a sheet could
// read it.
const nonces = new WeakMap();
const nonce = {
  get() { return nonces.has(this) ? nonces.get(this) : text("nonce").get.call(this); },
  set(value) { nonces.set(this, String(value)); },
};
// tabStop is where the Tab key stops without being told to.
const tabStop = (element) => (/^(button|input|select|textarea|iframe|summary)$/.test(element.localName) ||
  (/^(a|area)$/.test(element.localName) && ownAttr(element, "href") !== null) ? 0 : -1);
const stated = (element, attribute) => {
  const value = ownAttr(element, attribute);
  return value === null ? null : asciiLower(value);
};
const everyElement = {
  title: "title", lang: "lang", accessKey: "accesskey", role: "role",
  dir: keyword("dir", ["ltr", "rtl", "auto"]),
  autofocus: flag("autofocus"), inert: flag("inert"),
  tabIndex: whole("tabindex", tabStop),
  nonce,
  hidden: {
    get() {
      const value = stated(this, "hidden");
      return value === "until-found" ? value : value !== null;
    },
    set(value) {
      if (typeof value === "string" && asciiLower(value) === "until-found") this.setAttribute("hidden", "until-found");
      else if (value === false || value === "" || value === null || value === undefined || value === 0 ||
        Number.isNaN(value)) this.removeAttribute("hidden");
      else this.setAttribute("hidden", "");
    },
  },
  draggable: {
    get() {
      const value = stated(this, "draggable");
      if (value === "true" || value === "false") return value === "true";
      return this.localName === "img" || (this.localName === "a" && ownAttr(this, "href") !== null);
    },
    set(on) { this.setAttribute("draggable", String(Boolean(on))); },
  },
  translate: {
    get() {
      const value = stated(this, "translate");
      if (value === "yes" || value === "") return true;
      if (value === "no") return false;
      const parent = this.parentElement;
      return parent && "translate" in parent ? parent.translate : true;
    },
    set(on) { this.setAttribute("translate", on ? "yes" : "no"); },
  },
  // light checks no spelling, so only an element told to be says so.
  spellcheck: {
    get() {
      const value = stated(this, "spellcheck");
      return value === "true" || value === "";
    },
    set(on) { this.setAttribute("spellcheck", String(Boolean(on))); },
  },
  contentEditable: {
    get() {
      const value = stated(this, "contenteditable");
      if (value === "" || value === "true") return "true";
      return value === "false" || value === "plaintext-only" ? value : "inherit";
    },
    set(value) {
      const said = asciiLower(String(value));
      if (said === "inherit") this.removeAttribute("contenteditable");
      else if (said === "true" || said === "false" || said === "plaintext-only") this.setAttribute("contenteditable", said);
      else throw new DOMException(`The value provided ('${value}') is not one of 'true', 'false', 'plaintext-only', or 'inherit'.`, "SyntaxError");
    },
  },
  autocapitalize: keyword("autocapitalize", ["none", "sentences", "words", "characters"],
    { alias: { on: "sentences", off: "none" } }),
  enterKeyHint: keyword("enterkeyhint", ["enter", "done", "go", "next", "previous", "search", "send"]),
  inputMode: keyword("inputmode", ["none", "text", "tel", "url", "email", "numeric", "decimal", "search"]),
  popover: keyword("popover", ["auto", "manual", "hint"],
    { missing: null, invalid: "manual", alias: { "": "auto" }, nullable: true }),
};
reflects(Element.prototype, { slot: "slot" });
reflects(HTMLElement.prototype, everyElement);
reflects(SVGElement.prototype, { autofocus: everyElement.autofocus, tabIndex: whole("tabindex", -1), nonce });

// ---- what each interface says

const referrerPolicy = keyword("referrerpolicy", ["", "no-referrer", "no-referrer-when-downgrade", "same-origin", "origin",
  "strict-origin", "origin-when-cross-origin", "strict-origin-when-cross-origin", "unsafe-url"]);
const crossOrigin = keyword("crossorigin", ["anonymous", "use-credentials"],
  { missing: null, invalid: "anonymous", alias: { "": "anonymous" }, nullable: true });
const loading = keyword("loading", ["lazy", "eager"], { missing: "eager" });
const encodings = ["application/x-www-form-urlencoded", "multipart/form-data", "text/plain"];
const enctype = keyword("enctype", encodings, { missing: encodings[0] });
const linked = {
  target: "target", download: "download", ping: "ping", rel: "rel", hreflang: "hreflang", type: "type",
  referrerPolicy, href: located("href"), coords: "coords", shape: "shape",
};
const aligned = { align: "align", ch: "char", chOff: "charoff", vAlign: "valign" };
const frameLike = {
  name: "name", scrolling: "scrolling", src: located("src"), frameBorder: "frameborder",
  longDesc: located("longdesc"), marginHeight: legacyText("marginheight"), marginWidth: legacyText("marginwidth"),
};
const control = { name: "name", disabled: flag("disabled") };
const submitter = {
  formAction: sentTo("formaction"), formEnctype: keyword("formenctype", encodings, { invalid: encodings[0] }),
  formNoValidate: flag("formnovalidate"), formTarget: "formtarget",
};
const typed = {
  autocomplete: "autocomplete", dirName: "dirname", maxLength: count("maxlength"), minLength: count("minlength"),
  placeholder: "placeholder", readOnly: flag("readonly"), required: flag("required"),
};
const media = {
  src: located("src"), crossOrigin, autoplay: flag("autoplay"), loop: flag("loop"), controls: flag("controls"),
  defaultMuted: flag("muted"), loading,
  preload: keyword("preload", ["none", "metadata", "auto"], { missing: "metadata", alias: { "": "auto" } }),
};
const cell = {
  ...aligned, colSpan: clamped("colspan", 1, 1000, 1), rowSpan: clamped("rowspan", 0, 65534, 1), headers: "headers",
  scope: keyword("scope", ["row", "col", "rowgroup", "colgroup"]), abbr: "abbr", axis: "axis", height: "height",
  width: "width", noWrap: flag("nowrap"), bgColor: legacyText("bgcolor"),
};
for (const [interfaceOf, properties] of [
  [HTMLBodyElement, {
    text: legacyText("text"), link: legacyText("link"), vLink: legacyText("vlink"), aLink: legacyText("alink"),
    bgColor: legacyText("bgcolor"), background: "background",
  }],
  [HTMLHeadingElement, { align: "align" }],
  [HTMLParagraphElement, { align: "align" }],
  [HTMLDivElement, { align: "align" }],
  [HTMLHRElement, { align: "align", color: "color", noShade: flag("noshade"), size: "size", width: "width" }],
  [HTMLPreElement, { width: whole("width") }],
  [HTMLQuoteElement, { cite: located("cite") }],
  [HTMLOListElement, { reversed: flag("reversed"), start: whole("start", 1), type: "type", compact: flag("compact") }],
  [HTMLUListElement, { compact: flag("compact"), type: "type" }],
  [HTMLLIElement, { value: whole("value"), type: "type" }],
  [HTMLDListElement, { compact: flag("compact") }],
  [HTMLMenuElement, { compact: flag("compact") }],
  [HTMLDirectoryElement, { compact: flag("compact") }],
  [HTMLAnchorElement, { ...linked, charset: "charset", name: "name", rev: "rev" }],
  [HTMLAreaElement, { ...linked, alt: "alt", noHref: flag("nohref") }],
  [HTMLDataElement, { value: "value" }],
  [HTMLTimeElement, { dateTime: "datetime" }],
  [HTMLBRElement, { clear: "clear" }],
  [HTMLHtmlElement, { version: "version" }],
  [HTMLScriptElement, {
    src: located("src"), type: "type", noModule: flag("nomodule"), charset: "charset", async: forceAsync,
    defer: flag("defer"), crossOrigin, integrity: "integrity", event: "event", htmlFor: "for", referrerPolicy,
  }],
  [HTMLSlotElement, { name: "name" }],
  [HTMLModElement, { cite: located("cite"), dateTime: "datetime" }],
  [HTMLDetailsElement, { open: flag("open"), name: "name" }],
  [HTMLDialogElement, { open: flag("open") }],
  [HTMLBaseElement, { href: located("href"), target: "target" }],
  [HTMLLinkElement, {
    href: located("href"), crossOrigin, rel: "rel", media: "media", integrity: "integrity", hreflang: "hreflang",
    type: "type", referrerPolicy, charset: "charset", rev: "rev", target: "target", disabled: flag("disabled"),
    as: keyword("as", ["fetch", "audio", "document", "embed", "font", "image", "manifest", "object", "report", "script",
      "sharedworker", "style", "track", "video", "worker", "xslt"]),
  }],
  [HTMLMetaElement, { name: "name", httpEquiv: "http-equiv", content: "content", media: "media", scheme: "scheme" }],
  [HTMLStyleElement, { media: "media", type: "type" }],
  [HTMLMarqueeElement, {
    behavior: keyword("behavior", ["scroll", "slide", "alternate"], { missing: "scroll" }),
    direction: keyword("direction", ["up", "right", "down", "left"], { missing: "left" }), bgColor: "bgcolor",
    height: "height", width: "width", hspace: natural("hspace"), vspace: natural("vspace"), loop: whole("loop", -1),
    scrollAmount: natural("scrollamount", 6), scrollDelay: natural("scrolldelay", 85), trueSpeed: flag("truespeed"),
  }],
  [HTMLFrameSetElement, { cols: "cols", rows: "rows" }],
  [HTMLFrameElement, { ...frameLike, noResize: flag("noresize") }],
  [HTMLFontElement, { color: legacyText("color"), face: "face", size: "size" }],
  [HTMLTableElement, {
    align: "align", border: "border", frame: "frame", rules: "rules", summary: "summary", width: "width",
    bgColor: legacyText("bgcolor"), cellPadding: legacyText("cellpadding"), cellSpacing: legacyText("cellspacing"),
  }],
  [HTMLTableCaptionElement, { align: "align" }],
  [HTMLTableColElement, { ...aligned, span: clamped("span", 1, 1000, 1), width: "width" }],
  [HTMLTableSectionElement, aligned],
  [HTMLTableRowElement, { ...aligned, bgColor: legacyText("bgcolor") }],
  [HTMLTableCellElement, cell],
  [HTMLImageElement, {
    alt: "alt", src: located("src"), srcset: "srcset", sizes: "sizes", crossOrigin, useMap: "usemap",
    isMap: flag("ismap"), width: setter(natural("width")), height: setter(natural("height")), referrerPolicy,
    decoding: keyword("decoding", ["async", "sync", "auto"], { missing: "auto" }), loading, name: "name",
    lowsrc: located("lowsrc"), align: "align", hspace: natural("hspace"), vspace: natural("vspace"),
    longDesc: located("longdesc"), border: legacyText("border"),
    fetchPriority: keyword("fetchpriority", ["high", "low", "auto"], { missing: "auto" }),
  }],
  [HTMLIFrameElement, {
    ...frameLike, srcdoc: "srcdoc", allow: "allow", allowFullscreen: flag("allowfullscreen"), width: "width",
    height: "height", referrerPolicy, align: "align", loading,
  }],
  [HTMLEmbedElement, { src: located("src"), type: "type", width: "width", height: "height", align: "align", name: "name" }],
  [HTMLObjectElement, {
    data: located("data"), type: "type", name: "name", useMap: "usemap", width: "width", height: "height",
    align: "align", archive: "archive", code: "code", declare: flag("declare"), hspace: natural("hspace"),
    standby: "standby", vspace: natural("vspace"), codeBase: located("codebase"), codeType: "codetype",
    border: legacyText("border"),
  }],
  [HTMLParamElement, { name: "name", value: "value", type: "type", valueType: "valuetype" }],
  [HTMLMediaElement, media],
  [HTMLVideoElement, {
    width: natural("width"), height: natural("height"), poster: located("poster"), playsInline: flag("playsinline"),
  }],
  [HTMLSourceElement, {
    src: located("src"), type: "type", srcset: "srcset", sizes: "sizes", media: "media", width: natural("width"),
    height: natural("height"),
  }],
  [HTMLTrackElement, {
    kind: keyword("kind", ["subtitles", "captions", "descriptions", "chapters", "metadata"],
      { missing: "subtitles", invalid: "metadata" }),
    src: located("src"), srclang: "srclang", label: "label", default: flag("default"),
  }],
  [HTMLCanvasElement, { width: natural("width", 300), height: natural("height", 150) }],
  [HTMLMapElement, { name: "name" }],
  [HTMLFormElement, {
    acceptCharset: "accept-charset", action: sentTo("action"), enctype, encoding: enctype, name: "name",
    autocomplete: keyword("autocomplete", ["on", "off"], { missing: "on" }),
    method: keyword("method", ["get", "post", "dialog"], { missing: "get" }), noValidate: flag("novalidate"),
    target: "target", rel: "rel",
  }],
  [HTMLFieldSetElement, control],
  [HTMLLegendElement, { align: "align" }],
  [HTMLLabelElement, { htmlFor: "for" }],
  [HTMLInputElement, {
    ...control, ...submitter, ...typed, accept: "accept", alt: "alt", max: "max", min: "min", step: "step",
    formMethod: keyword("formmethod", ["get", "post", "dialog"], { invalid: "get" }), multiple: flag("multiple"),
    pattern: "pattern", size: positive("size", 20), src: located("src"), switch: flag("switch"), align: "align",
    useMap: "usemap", width: natural("width"), height: natural("height"), defaultChecked: flag("checked"),
    defaultValue: "value",
    type: keyword("type", ["hidden", "text", "search", "tel", "url", "email", "password", "date", "month", "week", "time",
      "datetime-local", "number", "range", "color", "checkbox", "radio", "file", "submit", "image", "reset", "button"],
    { missing: "text" }),
  }],
  [HTMLButtonElement, {
    ...control, ...submitter, value: "value",
    formMethod: keyword("formmethod", ["get", "post", "dialog"], { invalid: "get" }),
    type: keyword("type", ["submit", "reset", "button"], { missing: "submit" }),
  }],
  [HTMLSelectElement, {
    ...control, autocomplete: "autocomplete", multiple: flag("multiple"), required: flag("required"),
    size: natural("size"),
  }],
  [HTMLOptGroupElement, { disabled: flag("disabled"), label: "label" }],
  [HTMLOptionElement, {
    disabled: flag("disabled"), label: setter(text("label")), defaultSelected: flag("selected"),
  }],
  [HTMLTextAreaElement, {
    ...control, ...typed, cols: positiveOr("cols", 20), rows: positiveOr("rows", 2), wrap: "wrap",
  }],
  [HTMLOutputElement, { name: "name" }],
  [HTMLProgressElement, { value: real("value"), max: positiveReal("max", 1) }],
  [HTMLMeterElement, {
    value: real("value"), min: real("min"), max: real("max", 1), low: real("low"), high: real("high", 1),
    optimum: real("optimum", 0.5),
  }],
]) reflects(interfaceOf.prototype, properties);

// A document says its root's direction and its body's colours as its
// own, and nothing where it has neither.
const forwarded = (holder, kind) => ({
  get() {
    const to = this[holder];
    return to instanceof HTMLElement ? kind.get.call(to) : "";
  },
  set(value) {
    const to = this[holder];
    if (to instanceof HTMLElement) kind.set.call(to, value);
  },
});
reflects(Document.prototype, {
  dir: forwarded("documentElement", everyElement.dir), fgColor: forwarded("body", legacyText("text")),
  linkColor: forwarded("body", legacyText("link")), vlinkColor: forwarded("body", legacyText("vlink")),
  alinkColor: forwarded("body", legacyText("alink")), bgColor: forwarded("body", legacyText("bgcolor")),
});
