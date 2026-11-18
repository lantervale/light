// ---- stylesheets: the CSS object model over the sheets light keeps

// sheetHost is host for a sheet's operations: what light refuses of a
// rule or of a place is thrown as the DOMException browsers throw.
const sheetHost = (...args) => {
  try {
    return light(...args);
  } catch (error) {
    const named = /^(SyntaxError|IndexSizeError|InvalidStateError): (.*)$/s.exec(error.message);
    if (named) throw new DOMException(named[2], named[1]);
    throw error;
  }
};

// cssParts cuts the text of a block into what stands at its top level:
// rules, each ending with its } or its ;.
const cssParts = (text) => {
  const parts = [];
  let depth = 0, quote = "", start = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quote) {
      if (c === "\\") i++;
      else if (c === quote) quote = "";
    } else if (c === '"' || c === "'") quote = c;
    else if (c === "/" && text[i + 1] === "*") {
      const end = text.indexOf("*/", i + 2);
      i = end < 0 ? text.length : end + 1;
    } else if (c === "{" || c === "(" || c === "[") depth++;
    else if (c === "}" || c === ")" || c === "]") {
      depth--;
      if (depth === 0 && c === "}") {
        parts.push(text.slice(start, i + 1).trim());
        start = i + 1;
      }
    } else if (c === ";" && depth === 0) {
      const part = text.slice(start, i + 1).trim();
      if (part.startsWith("@")) parts.push(part);
      start = i + 1;
    }
  }
  return parts;
};
// cssBlock is a rule's text as what stands before its block and what
// the block holds, the whole text and null for a rule without one.
const cssBlock = (text) => {
  let quote = "";
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quote) {
      if (c === "\\") i++;
      else if (c === quote) quote = "";
    } else if (c === '"' || c === "'") quote = c;
    else if (c === "{") return [text.slice(0, i).trim(), text.slice(i + 1, text.lastIndexOf("}"))];
  }
  return [text.trim(), null];
};

const STYLE_RULE = 1, IMPORT_RULE = 3, MEDIA_RULE = 4, FONT_FACE_RULE = 5, KEYFRAMES_RULE = 7, SUPPORTS_RULE = 12;

// A rule keeps its text, and reads it only when a script asks for a
// part of it. One that changes writes its text back to its sheet.
class CSSRule {
  constructor(key, text, sheet, parent) {
    if (key !== token) illegal();
    this._text = text;
    this._sheet = sheet;
    this._parent = parent || null;
  }
  get cssText() { return this._text; }
  set cssText(_) {}
  get parentStyleSheet() { return this._sheet; }
  get parentRule() { return this._parent; }
  get type() { return 0; }
  // _changed has the rule's text, and those of the rules it stands in,
  // say what the rule now is.
  _changed(text) {
    this._text = text;
    if (this._parent) this._parent._rewrite();
    else if (this._sheet) this._sheet._rewrite(this);
  }
}
define(CSSRule, { STYLE_RULE, IMPORT_RULE, MEDIA_RULE, FONT_FACE_RULE, KEYFRAMES_RULE, SUPPORTS_RULE });

// ruleStyle is the declarations of a rule as scripts write them:
// rule.style.color, rule.style.setProperty(…), rule.style.cssText.
const ruleStyle = (rule, read, write) => {
  const view = (act) => {
    const map = declarations(read());
    return act({
      map,
      write() { write(Array.from(map, ([name, value]) => `${name}: ${value};`).join(" ")); },
    });
  };
  return new Proxy({}, {
    get(_, name) {
      if (typeof name !== "string") return undefined;
      if (name === "cssText") return read().trim();
      if (name === "length") return declarations(read()).size;
      if (name === "parentRule") return rule;
      if (name in styleMethods) return (...args) => view((s) => styleMethods[name](s, ...args));
      return view((s) => styleMethods.getPropertyValue(s, name === "cssFloat" ? "float" : name));
    },
    set(_, name, value) {
      if (typeof name !== "string") return true;
      if (name === "cssText") write(String(value));
      else view((s) => styleMethods.setProperty(s, name === "cssFloat" ? "float" : name, value));
      return true;
    },
    has(_, name) { return typeof name === "string"; },
  });
};

class CSSStyleRule extends CSSRule {
  get type() { return STYLE_RULE; }
  get selectorText() { return cssBlock(this._text)[0]; }
  set selectorText(selector) { this._changed(`${String(selector).trim()} { ${cssBlock(this._text)[1] || ""} }`); }
  get style() {
    if (!this._style) {
      this._style = ruleStyle(this, () => cssBlock(this._text)[1] || "",
        (body) => this._changed(`${cssBlock(this._text)[0]} { ${body} }`));
    }
    return this._style;
  }
  set style(text) { this.style.cssText = text; }
  get cssRules() { return (this._none = this._none || new CSSRuleList()); }
}

// A grouping rule holds rules: @media, @supports, @layer, @container.
class CSSGroupingRule extends CSSRule {
  get cssRules() {
    if (!this._rules) {
      this._rules = CSSRuleList.from(cssParts(cssBlock(this._text)[1] || "").map((text) => ruleOf(text, this._sheet, this)));
    }
    return this._rules;
  }
  get conditionText() { return cssBlock(this._text)[0].replace(/^@[\w-]+\s*/, ""); }
  insertRule(text, index = 0) {
    const rules = this.cssRules;
    if (index < 0 || index > rules.length) throw new DOMException("The index is out of range.", "IndexSizeError");
    const [part] = cssParts(String(text));
    if (!part) throw new DOMException(`Failed to parse the rule '${text}'.`, "SyntaxError");
    rules.splice(index, 0, ruleOf(part, this._sheet, this));
    this._rewrite();
    return index;
  }
  deleteRule(index) {
    const rules = this.cssRules;
    if (index < 0 || index >= rules.length) throw new DOMException("The index is out of range.", "IndexSizeError");
    rules.splice(index, 1);
    this._rewrite();
  }
  _rewrite() {
    this._changed(`${cssBlock(this._text)[0]} { ${this.cssRules.map((rule) => rule.cssText).join(" ")} }`);
  }
}
class CSSConditionRule extends CSSGroupingRule {}
class MediaList extends Array {
  static get [Symbol.species]() { return Array; }
  static of(text, write) {
    const list = new MediaList();
    for (const medium of String(text).split(",").map((m) => m.trim()).filter(Boolean)) list.push(medium);
    list._write = write;
    return list;
  }
  get mediaText() { return this.join(", "); }
  set mediaText(text) { if (this._write) this._write(String(text)); }
  item(i) { return this[i] === undefined ? null : this[i]; }
  appendMedium(medium) { if (!this.includes(medium)) this.mediaText = this.concat(medium).join(", "); }
  deleteMedium(medium) { this.mediaText = this.filter((m) => m !== medium).join(", "); }
  toString() { return this.mediaText; }
}
class CSSMediaRule extends CSSConditionRule {
  get type() { return MEDIA_RULE; }
  get media() {
    return MediaList.of(this.conditionText, (text) => {
      this._changed(`@media ${text} { ${cssBlock(this._text)[1] || ""} }`);
    });
  }
}
class CSSSupportsRule extends CSSConditionRule {
  get type() { return SUPPORTS_RULE; }
}
class CSSContainerRule extends CSSConditionRule {}
class CSSLayerBlockRule extends CSSGroupingRule {
  get name() { return this.conditionText; }
}
class CSSLayerStatementRule extends CSSRule {
  get nameList() { return this._text.replace(/^@layer\s*|;$/g, "").split(",").map((name) => name.trim()); }
}
class CSSKeyframesRule extends CSSGroupingRule {
  get type() { return KEYFRAMES_RULE; }
  get name() { return this.conditionText; }
}
class CSSFontFaceRule extends CSSRule {
  get type() { return FONT_FACE_RULE; }
  get style() {
    return ruleStyle(this, () => cssBlock(this._text)[1] || "",
      (body) => this._changed(`@font-face { ${body} }`));
  }
}
class CSSImportRule extends CSSRule {
  get type() { return IMPORT_RULE; }
  get href() {
    const named = /^@import\s+(?:url\(\s*)?["']?([^"')\s;]+)/.exec(this._text);
    return named ? named[1] : "";
  }
  get styleSheet() { return null; }
}

const atRules = {
  media: CSSMediaRule, supports: CSSSupportsRule, container: CSSContainerRule, keyframes: CSSKeyframesRule,
  "-webkit-keyframes": CSSKeyframesRule, "font-face": CSSFontFaceRule, import: CSSImportRule,
};
const ruleOf = (text, sheet, parent) => {
  const at = /^@([\w-]+)/.exec(text);
  if (!at) return new CSSStyleRule(token, text, sheet, parent);
  const name = at[1].toLowerCase();
  if (name === "layer") return new (text.includes("{") ? CSSLayerBlockRule : CSSLayerStatementRule)(token, text, sheet, parent);
  return new (atRules[name] || CSSRule)(token, text, sheet, parent);
};

class CSSRuleList extends Array {
  static get [Symbol.species]() { return Array; }
  static from(rules) {
    const list = new CSSRuleList();
    for (const rule of rules) list.push(rule);
    return list;
  }
  item(i) { return this[i] === undefined ? null : this[i]; }
}

// A sheet is one light keeps, by its number: its rules are read from
// light the first time a script asks, and again once light read the
// sheet's text again (its version).
const sheetObjects = new Map();
class StyleSheet {}
class CSSStyleSheet extends StyleSheet {
  constructor(options = {}, key, id) {
    super();
    if (key === token) {
      this._id = id;
    } else {
      const media = options.media === undefined ? "" : String(options.media.mediaText || options.media);
      this._id = host("newSheet", media, Boolean(options.disabled));
      this._made = true;
      this._off = Boolean(options.disabled);
      sheetObjects.set(this._id, this);
    }
    this._version = -1;
    this._rules = new CSSRuleList();
  }
  // _state is the sheet as light has it now, and reads its rules again
  // when they are no longer those this object knows.
  _state() {
    const state = host("sheetState", this._id);
    if (!state) return [this._version, 0, true, "", ""];
    if (state[0] !== this._version) {
      this._version = state[0];
      const texts = sheetHost("sheetRules", this._id);
      this._rules.length = 0;
      for (const text of texts) this._rules.push(ruleOf(text, this, null));
    }
    return state;
  }
  get cssRules() { this._state(); return this._rules; }
  get rules() { return this.cssRules; }
  get ownerNode() { return wrap(this._state()[1]); }
  get ownerRule() { return null; }
  get parentStyleSheet() { return null; }
  get type() { return "text/css"; }
  get title() { const owner = this.ownerNode; return owner ? owner.getAttribute("title") : null; }
  get href() { return this._state()[4] || null; }
  get disabled() { return this._made ? this._off : this._state()[2]; }
  set disabled(off) {
    this._off = Boolean(off);
    sheetHost("disableSheet", this._id, this._off);
  }
  get media() {
    const owner = this.ownerNode;
    return MediaList.of(this._state()[3], (text) => {
      if (owner) owner.setAttribute("media", text);
      else sheetHost("setSheetMedia", this._id, text);
    });
  }
  insertRule(text, index = 0) {
    const rules = this.cssRules;
    index = Number(index) || 0;
    if (index < 0 || index > rules.length) throw new DOMException("The index is out of range.", "IndexSizeError");
    text = String(text);
    sheetHost("insertRule", this._id, text, index);
    rules.splice(index, 0, ruleOf(text.trim(), this, null));
    return index;
  }
  deleteRule(index) {
    const rules = this.cssRules;
    index = Number(index) || 0;
    if (index < 0 || index >= rules.length) throw new DOMException("The index is out of range.", "IndexSizeError");
    sheetHost("deleteRule", this._id, index);
    rules.splice(index, 1);
  }
  addRule(selector, style, index) {
    this.insertRule(`${selector} { ${style || ""} }`, index === undefined ? this.cssRules.length : index);
    return -1;
  }
  removeRule(index = 0) { this.deleteRule(index); }
  replaceSync(text) {
    if (!this._made) throw new DOMException("Only a sheet a script made can be replaced.", "NotAllowedError");
    sheetHost("replaceSheet", this._id, String(text));
  }
  replace(text) {
    try {
      this.replaceSync(text);
      return Promise.resolve(this);
    } catch (error) {
      return Promise.reject(error);
    }
  }
  // _rewrite gives light the text of a top-level rule a script changed.
  _rewrite(rule) {
    const at = this._rules.indexOf(rule);
    if (at >= 0) sheetHost("setRule", this._id, at, rule.cssText);
  }
}
const sheetOf = (id) => {
  let sheet = sheetObjects.get(id);
  if (!sheet) {
    sheet = new CSSStyleSheet(undefined, token, id);
    sheetObjects.set(id, sheet);
  }
  return sheet;
};
// sheetOfElement is the sheet of a style element or a link, null for
// one that is not in the page or whose sheet has not come yet.
const sheetOfElement = (element) => {
  const found = host("sheet", element[REF]);
  return found ? sheetOf(found[0]) : null;
};

class StyleSheetList extends Array {
  static get [Symbol.species]() { return Array; }
  item(i) { return this[i] === undefined ? null : this[i]; }
}

Object.defineProperty(HTMLStyleElement.prototype, "sheet", { get() { return sheetOfElement(this); }, configurable: true });
Object.defineProperty(HTMLStyleElement.prototype, "disabled", {
  get() { const sheet = this.sheet; return sheet ? sheet.disabled : false; },
  set(off) { const sheet = this.sheet; if (sheet) sheet.disabled = off; },
  configurable: true,
});
Object.defineProperty(HTMLLinkElement.prototype, "sheet", { get() { return sheetOfElement(this); }, configurable: true });
reflect(HTMLLinkElement.prototype, { disabled: ["disabled", false] });

let adopted = Object.freeze([]);
Object.defineProperty(Document.prototype, "styleSheets", {
  get() {
    const list = new StyleSheetList();
    for (const id of host("sheets")) list.push(sheetOf(id));
    return list;
  },
  configurable: true,
});
Object.defineProperty(Document.prototype, "adoptedStyleSheets", {
  get() { return adopted; },
  set(sheets) {
    const list = Array.from(sheets);
    if (list.some((sheet) => !(sheet instanceof CSSStyleSheet) || !sheet._made)) {
      throw new DOMException("Only sheets a script made can be adopted.", "NotAllowedError");
    }
    host("adoptSheets", list.map((sheet) => sheet._id));
    adopted = Object.freeze(list);
  },
  configurable: true,
});

CSS.supports = (property, value) => (value === undefined
  ? host("supportsWhen", String(property)) : host("supports", String(property), String(value)));

define(window, {
  CSSStyleSheet, StyleSheet, StyleSheetList, MediaList, CSSRule, CSSRuleList, CSSStyleRule, CSSGroupingRule,
  CSSConditionRule, CSSMediaRule, CSSSupportsRule, CSSContainerRule, CSSLayerBlockRule, CSSLayerStatementRule,
  CSSKeyframesRule, CSSFontFaceRule, CSSImportRule,
});
