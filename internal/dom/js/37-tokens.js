// ---- token lists

// A token list is an attribute read as an ordered set of the words
// between its ASCII whitespace: it keeps nothing of its own, so it and
// the attribute follow each other, and an element gives the same list
// each time it is asked.
const tokenLists = new WeakMap();
const listOf = (list) => tokenLists.get(list) || illegalInvocation();
const illegalInvocation = () => { throw new TypeError("Illegal invocation"); };
const tokensIn = (text) => [...new Set((text || "").split(/[\t\n\f\r ]+/).filter(Boolean))];
const tokensOn = (list) => {
  const { element, attribute } = listOf(list);
  return tokensIn(ownAttr(element, attribute));
};
// A token a list is asked to change is checked before anything changes.
const tokenGiven = (token) => {
  token = String(token);
  if (token === "") throw new DOMException("The token provided must not be empty.", "SyntaxError");
  if (/[\t\n\f\r ]/.test(token)) {
    throw new DOMException(`The token provided ('${token}') contains HTML space characters, which are not valid in tokens.`,
      "InvalidCharacterError");
  }
  return token;
};
// tokensKept writes a set back to its attribute, and makes no attribute
// for an empty set where there was none.
const tokensKept = (list, tokens) => {
  const { element, attribute } = listOf(list);
  if (!tokens.length && ownAttr(element, attribute) === null) return;
  element.setAttribute(attribute, tokens.join(" "));
};
const tokenIndex = (key) => (typeof key === "string" && /^(0|[1-9]\d*)$/.test(key) ? Number(key) : -1);
const indexedTokens = {
  get(target, key, receiver) {
    const i = tokenIndex(key);
    return i < 0 ? Reflect.get(target, key, receiver) : tokensOn(receiver)[i];
  },
  has(target, key) {
    const i = tokenIndex(key);
    return i < 0 ? Reflect.has(target, key) : i < tokensIn(ownAttr(target.element, target.attribute)).length;
  },
  set(target, key, value, receiver) { return tokenIndex(key) < 0 && Reflect.set(target, key, value, receiver); },
  defineProperty(target, key, descriptor) { return tokenIndex(key) < 0 && Reflect.defineProperty(target, key, descriptor); },
  deleteProperty(target, key) {
    const i = tokenIndex(key);
    return i < 0 ? Reflect.deleteProperty(target, key) : i >= tokensIn(ownAttr(target.element, target.attribute)).length;
  },
  ownKeys(target) {
    const tokens = tokensIn(ownAttr(target.element, target.attribute));
    return tokens.map((_, i) => String(i)).concat(Reflect.ownKeys(target));
  },
  getOwnPropertyDescriptor(target, key) {
    const i = tokenIndex(key);
    if (i < 0) return Reflect.getOwnPropertyDescriptor(target, key);
    const value = tokensIn(ownAttr(target.element, target.attribute))[i];
    return value === undefined ? undefined : { value, writable: false, enumerable: true, configurable: true };
  },
};

class DOMTokenList {
  constructor(key, element, attribute, supported) {
    if (key !== token) illegal();
    const state = { element, attribute, supported };
    const list = new Proxy(Object.defineProperties(this, {
      element: { value: element, configurable: true }, attribute: { value: attribute, configurable: true },
    }), indexedTokens);
    tokenLists.set(list, state);
    return list;
  }
  get length() { return tokensOn(this).length; }
  get value() { return ownAttr(listOf(this).element, listOf(this).attribute) || ""; }
  set value(text) { listOf(this).element.setAttribute(listOf(this).attribute, String(text)); }
  item(i) {
    const found = tokensOn(this)[Number(i) >>> 0];
    return found === undefined ? null : found;
  }
  contains(asked) { return tokensOn(this).includes(String(asked)); }
  add(...given) {
    given = given.map(tokenGiven);
    tokensKept(this, tokensIn(tokensOn(this).concat(given).join(" ")));
  }
  remove(...given) {
    given = given.map(tokenGiven);
    tokensKept(this, tokensOn(this).filter((kept) => !given.includes(kept)));
  }
  toggle(asked, force) {
    asked = tokenGiven(asked);
    const tokens = tokensOn(this);
    if (tokens.includes(asked)) {
      if (force !== undefined && force) return true;
      tokensKept(this, tokens.filter((kept) => kept !== asked));
      return false;
    }
    if (force !== undefined && !force) return false;
    tokensKept(this, tokens.concat(asked));
    return true;
  }
  replace(old, asked) {
    if (String(old) === "" || String(asked) === "") tokenGiven("");
    old = tokenGiven(old);
    asked = tokenGiven(asked);
    const tokens = tokensOn(this);
    if (!tokens.includes(old)) return false;
    tokensKept(this, tokensIn(tokens.map((kept) => (kept === old ? asked : kept)).join(" ")));
    return true;
  }
  supports(asked) {
    const { supported, attribute } = listOf(this);
    if (!supported) throw new TypeError(`The '${attribute}' attribute has no supported tokens.`);
    return supported.includes(String(asked).toLowerCase());
  }
  toString() { return this.value; }
  get [Symbol.toStringTag]() { return "DOMTokenList"; }
}
define(DOMTokenList.prototype, {
  forEach: Array.prototype.forEach, keys: Array.prototype.keys, values: Array.prototype.values,
  entries: Array.prototype.entries,
});
Object.defineProperty(DOMTokenList.prototype, Symbol.iterator, {
  value: Array.prototype.values, writable: true, configurable: true,
});

// tokensOf is the one list of an attribute of an element, and supported
// the words light acts on in it, where the attribute has a list of them.
const tokensOf = (element, attribute, supported) => {
  let lists = tokenLists.get(element);
  if (!lists) tokenLists.set(element, lists = new Map());
  if (!lists.has(attribute)) lists.set(attribute, new DOMTokenList(token, element, attribute, supported));
  return lists.get(attribute);
};
// withTokens makes properties that are token lists, each set by setting
// its text.
const withTokens = (prototype, properties) => {
  for (const name of Object.keys(properties)) {
    const [attribute, supported] = [].concat(properties[name]);
    Object.defineProperty(prototype, name, {
      get() { return tokensOf(this, attribute, supported); },
      set(text) { tokensOf(this, attribute, supported).value = text; },
      configurable: true, enumerable: true,
    });
  }
};
const sandboxTokens = ["allow-scripts", "allow-same-origin", "allow-forms", "allow-popups", "allow-popups-to-escape-sandbox",
  "allow-downloads", "allow-modals", "allow-top-navigation", "allow-top-navigation-by-user-activation"];
withTokens(Element.prototype, { classList: "class" });
withTokens(HTMLLinkElement.prototype, { relList: ["rel", ["stylesheet", "alternate", "icon", "modulepreload"]], sizes: "sizes" });
withTokens(HTMLAnchorElement.prototype, { relList: ["rel", ["noopener"]] });
withTokens(HTMLAreaElement.prototype, { relList: ["rel", ["noopener"]] });
withTokens(HTMLFormElement.prototype, { relList: ["rel", ["noopener"]] });
withTokens(HTMLIFrameElement.prototype, { sandbox: ["sandbox", sandboxTokens] });
withTokens(HTMLOutputElement.prototype, { htmlFor: "for" });
