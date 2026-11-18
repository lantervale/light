// ---- XPath 1.0: document.evaluate and its kin
//
// An expression is read once into a small tree, each part knowing the
// type of its value, so what XPath refuses is refused before any node
// is looked at. A node-set is an array of nodes in document order, an
// attribute the one Attr light keeps for it here. The names of an HTML
// document's elements and attributes are matched as browsers match
// them: in lower case, whatever the expression's.

const XML_NS = "http://www.w3.org/XML/1998/namespace";
const XMLNS_NS = "http://www.w3.org/2000/xmlns/";
const attrNamespaces = { xlink: "http://www.w3.org/1999/xlink", xml: XML_NS, xmlns: XMLNS_NS };
const xpathKinds = ["node", "text", "comment", "processing-instruction"];
const xpathAxes = ["ancestor", "ancestor-or-self", "attribute", "child", "descendant", "descendant-or-self",
  "following", "following-sibling", "namespace", "parent", "preceding", "preceding-sibling", "self"];
const xpathReverse = new Set(["ancestor", "ancestor-or-self", "preceding", "preceding-sibling"]);
const asciiLower = (name) => name.replace(/[A-Z]/g, (c) => c.toLowerCase());

// ---- its words

const nameStart = "A-Z_a-z\\xC0-\\xD6\\xD8-\\xF6\\xF8-\\u02FF\\u0370-\\u037D\\u037F-\\u1FFF\\u200C\\u200D" +
  "\\u2070-\\u218F\\u2C00-\\u2FEF\\u3001-\\uD7FF\\uF900-\\uFDCF\\uFDF0-\\uFFFD\\u{10000}-\\u{EFFFF}";
const ncName = `[${nameStart}][${nameStart}\\-.0-9\\xB7\\u0300-\\u036F\\u203F\\u2040]*`;
const xpathWord = new RegExp([
  "(?<space>[ \\t\\r\\n]+)",
  "(?<number>\\d+(?:\\.\\d*)?|\\.\\d+)",
  "(?<literal>\"[^\"]*\"|'[^']*')",
  `(?<variable>\\$${ncName}(?::${ncName})?)`,
  `(?<name>${ncName}(?::(?:${ncName}|\\*))?)`,
  "(?<symbol>\\.\\.|::|//|!=|<=|>=|[()\\[\\].@,/|+\\-=<>*])",
].join("|"), "uy");
const xpathMarks = new Set(["(", ")", "[", "]", ".", "..", "@", ",", "::"]);
const xpathWordOperators = ["and", "or", "mod", "div"];

// xpathWords cuts an expression into its tokens. What a * or a name is
// hangs on its neighbours: after an operand they are operators, and a
// name before ( is a function or a node type, before :: an axis.
const xpathWords = (source, fail) => {
  const words = [];
  let at = 0;
  while (at < source.length) {
    xpathWord.lastIndex = at;
    const found = xpathWord.exec(source);
    if (!found) fail(`nothing of XPath begins as '${source.slice(at, at + 8)}' does`);
    at = xpathWord.lastIndex;
    const { space, number, literal, variable, name, symbol } = found.groups;
    if (space !== undefined) continue;
    const before = words[words.length - 1];
    const operand = before !== undefined && !["@", "::", "(", "[", ",", "op"].includes(before.type);
    if (number !== undefined) words.push({ type: "number", value: number });
    else if (literal !== undefined) words.push({ type: "literal", value: literal.slice(1, -1) });
    else if (variable !== undefined) words.push({ type: "variable", value: variable });
    else if (symbol === "*") words.push(operand ? { type: "op", value: "*" } : { type: "name", value: "*" });
    else if (symbol !== undefined) words.push(xpathMarks.has(symbol) ? { type: symbol } : { type: "op", value: symbol });
    else if (operand) {
      if (!xpathWordOperators.includes(name)) fail(`'${name}' stands where an operator must`);
      words.push({ type: "op", value: name });
    } else {
      const next = /^[ \t\r\n]*(\(|::)?/.exec(source.slice(at))[1];
      if (next === "::" && !xpathAxes.includes(name)) fail(`'${name}' is no axis`);
      const type = next === "::" ? "axis" : next !== "(" ? "name" : xpathKinds.includes(name) ? "kind" : "function";
      words.push({ type, value: name });
    }
  }
  return words;
};

// ---- values

const xpathNumeral = /^[ \t\r\n]*-?(?:\d+(?:\.\d*)?|\.\d+)[ \t\r\n]*$/;
const numberOfText = (text) => (xpathNumeral.test(text) ? Number(text) : NaN);
// textOfNumber writes a number as XPath does: no exponent, ever.
const textOfNumber = (number) => {
  const text = String(number);
  const parts = /^(-?)(\d)(?:\.(\d+))?e([+-]\d+)$/.exec(text);
  if (!parts) return text;
  const digits = parts[2] + (parts[3] || ""), point = 1 + Number(parts[4]);
  return parts[1] + (point <= 0 ? "0." + "0".repeat(-point) + digits : digits.padEnd(point, "0"));
};
const stringValue = (node) => {
  if (node instanceof Attr) return node.value;
  if (node !== document) return host("text", node[REF]);
  return childrenOf(node, true).map((child) => host("text", child[REF])).join("");
};
const xpathString = (value) => {
  if (typeof value === "string") return value;
  if (typeof value === "number") return textOfNumber(value);
  if (typeof value === "boolean") return String(value);
  return value.length ? stringValue(value[0]) : "";
};
const xpathNumber = (value) => {
  if (typeof value === "number") return value;
  if (typeof value === "boolean") return Number(value);
  return numberOfText(xpathString(value));
};
const xpathBoolean = (value) => {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0 && !Number.isNaN(value);
  return value.length > 0;
};
const xpathAs = { string: xpathString, number: xpathNumber, boolean: xpathBoolean, nodes: (v) => v, any: (v) => v };

const holds = (op, a, b) => {
  switch (op) {
    case "=": return a === b;
    case "!=": return a !== b;
    case "<": return a < b;
    case "<=": return a <= b;
    case ">": return a > b;
    default: return a >= b;
  }
};
// xpathCompares compares as XPath does: a node-set holds against a
// value when one of its nodes does, and the types of the rest decide
// whether booleans, numbers or strings are compared.
const xpathCompares = (op, a, b) => {
  const equality = op === "=" || op === "!=";
  if (typeof a === "object" && typeof b === "boolean") a = xpathBoolean(a);
  if (typeof b === "object" && typeof a === "boolean") b = xpathBoolean(b);
  const left = typeof a === "object", right = typeof b === "object";
  if (left || right) {
    const numbers = !equality || typeof a === "number" || typeof b === "number";
    const read = (value) => {
      if (typeof value !== "object") return [numbers ? xpathNumber(value) : xpathString(value)];
      return numbers ? value.map((node) => numberOfText(stringValue(node))) : value.map(stringValue);
    };
    const those = read(b);
    if (op === "=" && !numbers && those.length > 8) {
      const known = new Set(those);
      return read(a).some((x) => known.has(x));
    }
    return read(a).some((x) => those.some((y) => holds(op, x, y)));
  }
  if (!equality) return holds(op, xpathNumber(a), xpathNumber(b));
  if (typeof a === "boolean" || typeof b === "boolean") return holds(op, xpathBoolean(a), xpathBoolean(b));
  if (typeof a === "number" || typeof b === "number") return holds(op, xpathNumber(a), xpathNumber(b));
  return holds(op, a, b);
};

// ---- the tree, as XPath walks it

// attributeOf is the one Attr of an attribute of an element: given when
// a script handed its own as a context.
const xpathAttrs = new WeakMap();
const attributeOf = (element, name, value, given) => {
  let kept = xpathAttrs.get(element);
  if (!kept) xpathAttrs.set(element, kept = new Map());
  const attr = given || kept.get(name) || new Attr(element, name, value);
  if (kept.get(name) !== attr) {
    kept.set(name, attr);
    const colon = element[NS] ? name.indexOf(":") : -1;
    const prefix = colon > 0 && name.slice(0, colon) in attrNamespaces ? name.slice(0, colon) : null;
    attr.nodeType = 2;
    attr.prefix = prefix;
    attr.localName = prefix === null ? name : name.slice(colon + 1);
    attr.namespaceURI = prefix !== null ? attrNamespaces[prefix] : element[NS] && name === "xmlns" ? XMLNS_NS : null;
  }
  attr.value = attr.nodeValue = value;
  return attr;
};
const attributesOf = (element) => host("attrs", element[REF])
  .map(([name, value]) => attributeOf(element, name, value)).filter((attr) => attr.namespaceURI !== XMLNS_NS);

const holdsNodes = (node) => node === document || node instanceof Element || node instanceof DocumentFragment;
// The document type is no node of XPath's.
const childrenOf = (node, elements = false) => {
  if (node === document) {
    return host("top").map(wrap).filter((child) => (elements ? child instanceof Element : !(child instanceof DocumentType)));
  }
  return holdsNodes(node) ? host("children", node[REF], elements).map(wrap) : [];
};
const elementOf = (node) => (node instanceof Attr ? node.ownerElement : node);
const parentOf = (node) => {
  if (node instanceof Attr) return node.ownerElement;
  let parent = xpathParents.get(node);
  if (parent === undefined) xpathParents.set(node, parent = node.parentNode);
  return parent;
};
const descendantsOf = (node, into = []) => {
  for (const child of childrenOf(node)) {
    into.push(child);
    descendantsOf(child, into);
  }
  return into;
};
const ancestorsOf = (node) => {
  const up = [];
  for (let at = parentOf(node); at; at = parentOf(at)) up.push(at);
  return up;
};
const siblingsOf = (node, after) => {
  const parent = node instanceof Attr ? null : parentOf(node);
  if (!parent) return [];
  const all = childrenOf(parent), at = all.indexOf(node);
  return after ? all.slice(at + 1) : all.slice(0, at).reverse();
};
// Each axis gives its nodes in its own order: the reverse axes from the
// nearest node.
const xpathAxis = {
  ancestor: ancestorsOf,
  "ancestor-or-self": (node) => [node].concat(ancestorsOf(node)),
  attribute: (node) => (node instanceof Element ? attributesOf(node) : []),
  child: childrenOf,
  descendant: (node) => descendantsOf(node),
  "descendant-or-self": (node) => descendantsOf(node, [node]),
  following(node) {
    const found = node instanceof Attr ? descendantsOf(node.ownerElement) : [];
    for (let at = elementOf(node); at; at = parentOf(at)) {
      for (const next of siblingsOf(at, true)) {
        found.push(next);
        descendantsOf(next, found);
      }
    }
    return found;
  },
  "following-sibling": (node) => siblingsOf(node, true),
  namespace: () => [],
  parent(node) {
    const parent = parentOf(node);
    return parent ? [parent] : [];
  },
  preceding(node) {
    const found = [];
    for (let at = elementOf(node); at; at = parentOf(at)) {
      for (const before of siblingsOf(at, false)) {
        const under = descendantsOf(before);
        for (let i = under.length - 1; i >= 0; i--) found.push(under[i]);
        found.push(before);
      }
    }
    return found;
  },
  "preceding-sibling": (node) => siblingsOf(node, false),
  self: (node) => [node],
};

// What an evaluation learns of its tree, forgotten when it ends: its
// root, the parents it asked for, and the order of its nodes, which is
// one question to light while only elements and attributes are sorted,
// and a walk of the whole tree once a text or a comment is.
let xpathRoot = null, xpathOrder = null, xpathWhole = false, xpathParents = new Map();
const xpathForget = () => {
  xpathRoot = xpathOrder = null;
  xpathWhole = false;
  xpathParents = new Map();
};
const rootOf = (node) => {
  if (xpathRoot === null) {
    xpathRoot = elementOf(node);
    for (let up = parentOf(xpathRoot); up; up = parentOf(up)) xpathRoot = up;
  }
  return xpathRoot;
};
const ordered = (root, whole) => {
  const order = new Map([[root[REF], 0]]);
  const number = (refs) => {
    for (const ref of refs) {
      order.set(ref, order.size);
      if (whole && ref > 0) number(host("children", ref, false));
    }
  };
  if (!whole) number(holdsNodes(root) ? host("query", root[REF], "*", true) : []);
  else if (root === document) number(host("top"));
  else if (root[REF] > 0) number(host("children", root[REF], false));
  return order;
};
const orderOf = (node) => {
  if (!(node instanceof Attr)) return xpathOrder.get(node[REF]);
  const all = attributesOf(node.ownerElement);
  return xpathOrder.get(node.ownerElement[REF]) + (all.indexOf(node) + 1) / (all.length + 1);
};
// inOrder is a node-set of nodes found in any order, each once.
const inOrder = (nodes) => {
  if (nodes.length < 2) return nodes;
  const whole = nodes.some((node) => node instanceof CharacterData);
  if (xpathOrder === null || (whole && !xpathWhole)) {
    xpathOrder = ordered(rootOf(nodes[0]), whole);
    xpathWhole = whole;
  }
  const keys = new Map();
  for (const node of nodes) if (!keys.has(node)) keys.set(node, orderOf(node));
  return Array.from(keys.keys()).sort((a, b) => keys.get(a) - keys.get(b));
};
// apart reports whether no node of a node-set is in another: what is
// under each, taken in turn, is then in document order.
const apart = (nodes) => {
  for (let i = 1; i < nodes.length; i++) {
    const before = nodes[i - 1], node = nodes[i];
    if (node instanceof Attr || (holdsNodes(before) && (before === document || host("contains", before[REF], node[REF])))) {
      return false;
    }
  }
  return true;
};

// ---- steps

// xpathMatcher is a node test, for the nodes of an axis.
const xpathMatcher = (axis, { kind, local, lower, ns }) => {
  if (kind === "node") return () => true;
  if (kind === "text") return (node) => node instanceof Text;
  if (kind === "comment") return (node) => node instanceof Comment;
  if (kind !== "name" || axis === "namespace") return () => false;
  if (axis === "attribute") {
    if (local === "*") return (attr) => ns === null || attr.namespaceURI === ns;
    return (attr) => (ns === null && attr.namespaceURI === null && !attr.ownerElement[NS] ? attr.localName === lower
      : attr.localName === local && attr.namespaceURI === ns);
  }
  if (local === "*") return (node) => node instanceof Element && (ns === null || node.namespaceURI === ns);
  return (node) => node instanceof Element && (node[NS] ? node[TAG] === local && node.namespaceURI === ns
    : node[TAG] === lower && (ns === null || ns === HTML_NS));
};
// A step also knows the shorter ways to its nodes: the elements under
// a node are one question to light, an attribute of a name another. A
// grouped step is a // before a child step that counts positions: the
// elements under a node, each counted among those of its own parent.
const xpathStep = (axis, test, predicates, grouped = false) => {
  const step = { axis, test, predicates, grouped, matches: xpathMatcher(axis, test), reverse: xpathReverse.has(axis) };
  step.under = axis === "child" || axis === "descendant" || axis === "descendant-or-self";
  step.elements = test.kind === "name" && axis !== "attribute" && axis !== "namespace";
  if (step.elements && (axis === "descendant" || axis === "descendant-or-self")) {
    step.selector = test.ns === null && /^[a-z][a-z0-9]*$/.test(test.lower) ? test.lower : "*";
  }
  if (axis === "attribute" && test.kind === "name" && test.ns === null && test.local !== "*") step.attribute = test.lower;
  step.anywhere = axis === "descendant-or-self" && test.kind === "node" && predicates.length === 0;
  return step;
};
// positioned reports whether an expression reads the position or the
// size of its context.
const positioned = (e) => {
  switch (e.kind) {
    case "number": case "literal": return false;
    case "call": return e.name === "position" || e.name === "last" || e.args.some(positioned);
    case "path": return e.start !== null && positioned(e.start);
    case "negate": return positioned(e.operand);
    default: return positioned(e.left) || positioned(e.right);
  }
};

const sifted = (nodes, predicate) => {
  if (predicate.kind === "number") {
    const one = nodes[predicate.value - 1];
    return one === undefined ? [] : [one];
  }
  const size = nodes.length;
  if (predicate.returns === "number") return nodes.filter((node, i) => xpathValue(predicate, node, i + 1, size) === i + 1);
  return nodes.filter((node, i) => xpathBoolean(xpathValue(predicate, node, i + 1, size)));
};
const groupedFrom = (step, node) => {
  const groups = new Map();
  for (const one of host("query", node[REF], step.selector, true).map(wrap).filter(step.matches)) {
    const parent = parentOf(one), group = groups.get(parent);
    if (group) group.push(one);
    else groups.set(parent, [one]);
  }
  const found = [];
  for (let group of groups.values()) {
    for (const predicate of step.predicates) group = sifted(group, predicate);
    for (const one of group) found.push(one);
  }
  return inOrder(found);
};
const steppedFrom = (step, node) => {
  let nodes;
  if (step.grouped) return holdsNodes(node) ? groupedFrom(step, node) : [];
  if (step.selector && holdsNodes(node)) {
    nodes = host("query", node[REF], step.selector, true).map(wrap).filter(step.matches);
    if (step.axis === "descendant-or-self" && step.matches(node)) nodes.unshift(node);
  } else if (step.attribute && node instanceof Element && !node[NS]) {
    const value = host("attr", node[REF], step.attribute);
    nodes = value === null ? [] : [attributeOf(node, step.attribute, value)];
  } else if (step.axis === "child") {
    nodes = childrenOf(node, step.elements).filter(step.matches);
  } else {
    nodes = xpathAxis[step.axis](node).filter(step.matches);
  }
  for (const predicate of step.predicates) nodes = sifted(nodes, predicate);
  return step.reverse ? nodes.reverse() : nodes;
};
const stepped = (step, nodes) => {
  if (nodes.length === 1) return steppedFrom(step, nodes[0]);
  const found = [];
  for (const node of nodes) for (const one of steppedFrom(step, node)) found.push(one);
  if (step.axis === "self" || step.axis === "attribute") return found;
  if (step.under && found.some((one) => one instanceof CharacterData) && apart(nodes)) return found;
  return inOrder(found);
};

// ---- the core functions: what each returns, what it takes, and it

const firstNamed = (nodes, name) => (nodes.length ? name(nodes[0]) : "");
const xpathSpaces = /[ \t\r\n]+/;
const xpathFunctions = {
  last: ["number", "", (node, position, size) => size],
  position: ["number", "", (node, position) => position],
  count: ["number", "nodes", (node, position, size, nodes) => nodes.length],
  id: ["nodes", "any", (node, position, size, value) => {
    const ids = (typeof value === "object" ? value.map(stringValue).join(" ") : xpathString(value)).split(xpathSpaces);
    const root = rootOf(node);
    const found = [];
    for (const id of new Set(ids.filter(Boolean))) {
      if (root === document) found.push(document.getElementById(id));
      else if (root instanceof Element && root.id === id) found.push(root);
      else if (!(root instanceof CharacterData)) found.push(root.querySelector(`[id="${CSS.escape(id)}"]`));
    }
    return inOrder(found.filter(Boolean));
  }],
  "local-name": ["string", "nodes?", (node, position, size, nodes = [node]) => firstNamed(nodes, (n) => (n instanceof Element
    ? n[TAG] : n instanceof Attr ? n.localName : ""))],
  "namespace-uri": ["string", "nodes?", (node, position, size, nodes = [node]) => firstNamed(nodes, (n) => (
    n instanceof Element || n instanceof Attr ? n.namespaceURI || "" : ""))],
  name: ["string", "nodes?", (node, position, size, nodes = [node]) => firstNamed(nodes, (n) => (n instanceof Element
    ? n[TAG] : n instanceof Attr ? n.name : ""))],
  string: ["string", "any?", (node, position, size, value = [node]) => xpathString(value)],
  concat: ["string", "string string string*", (node, position, size, ...texts) => texts.join("")],
  "starts-with": ["boolean", "string string", (node, position, size, text, start) => text.startsWith(start)],
  contains: ["boolean", "string string", (node, position, size, text, part) => text.includes(part)],
  "substring-before": ["string", "string string", (node, position, size, text, mark) => {
    const at = text.indexOf(mark);
    return at < 0 ? "" : text.slice(0, at);
  }],
  "substring-after": ["string", "string string", (node, position, size, text, mark) => {
    const at = text.indexOf(mark);
    return at < 0 ? "" : text.slice(at + mark.length);
  }],
  substring: ["string", "string number number?", (node, position, size, text, start, length) => {
    const characters = Array.from(text);
    const from = Math.round(start), to = length === undefined ? Infinity : from + Math.round(length);
    return characters.filter((_, i) => i + 1 >= from && i + 1 < to).join("");
  }],
  "string-length": ["number", "string?", (node, position, size, text = stringValue(node)) => Array.from(text).length],
  "normalize-space": ["string", "string?", (node, position, size, text = stringValue(node)) => text
    .split(xpathSpaces).filter(Boolean).join(" ")],
  translate: ["string", "string string string", (node, position, size, text, from, to) => {
    const those = Array.from(from), these = Array.from(to);
    let out = "";
    for (const character of text) {
      const at = those.indexOf(character);
      if (at < 0) out += character;
      else if (at < these.length) out += these[at];
    }
    return out;
  }],
  boolean: ["boolean", "boolean", (node, position, size, value) => value],
  not: ["boolean", "boolean", (node, position, size, value) => !value],
  true: ["boolean", "", () => true],
  false: ["boolean", "", () => false],
  // The language of a node is its nearest xml:lang, the XML namespace's.
  lang: ["boolean", "string", (node, position, size, wanted) => {
    for (let at = elementOf(node); at; at = at.parentNode) {
      const said = at instanceof Element && attributesOf(at).find((a) => a.namespaceURI === XML_NS && a.localName === "lang");
      if (!said) continue;
      const lang = said.value.toLowerCase(), want = wanted.toLowerCase();
      return lang === want || lang.startsWith(want + "-");
    }
    return false;
  }],
  number: ["number", "any?", (node, position, size, value = [node]) => xpathNumber(value)],
  sum: ["number", "nodes", (node, position, size, nodes) => nodes.reduce((sum, n) => sum + numberOfText(stringValue(n)), 0)],
  floor: ["number", "number", (node, position, size, number) => Math.floor(number)],
  ceiling: ["number", "number", (node, position, size, number) => Math.ceil(number)],
  round: ["number", "number", (node, position, size, number) => Math.round(number)],
};

// ---- reading

const xpathLevels = [["or"], ["and"], ["=", "!="], ["<", "<=", ">", ">="], ["+", "-"], ["*", "div", "mod"]];
const xpathReturns = ["boolean", "boolean", "boolean", "boolean", "number", "number"];

// xpathTree reads an expression, and throws the SyntaxError or the
// NamespaceError the standard gives one that cannot be evaluated.
const xpathTree = (source, resolver) => {
  const fail = (why) => { throw new DOMException(`'${source}' is not a valid XPath expression: ${why}.`, "SyntaxError"); };
  const words = xpathWords(source, fail);
  let at = 0;
  const is = (type, value) => at < words.length && words[at].type === type && (value === undefined || words[at].value === value);
  const accept = (type, value) => (is(type, value) ? words[at++] : null);
  const take = (type) => accept(type) || fail(`'${type}' is missing`);
  const namespace = (prefix) => {
    let uri = null;
    if (typeof resolver === "function") uri = resolver(prefix);
    else if (resolver && typeof resolver.lookupNamespaceURI === "function") uri = resolver.lookupNamespaceURI(prefix);
    if (uri === null || uri === undefined || uri === "") {
      throw new DOMException(`'${source}' has a prefix, '${prefix}', that names no namespace.`, "NamespaceError");
    }
    return String(uri);
  };
  const nodes = (e, what) => (e.returns === "nodes" ? e : fail(`${what} is not a node-set`));

  const predicates = () => {
    const all = [];
    while (accept("[")) {
      all.push(or(0));
      take("]");
    }
    return all;
  };
  const test = () => {
    const word = words[at++];
    if (word && word.type === "kind") {
      take("(");
      if (word.value === "processing-instruction") accept("literal");
      take(")");
      return { kind: word.value };
    }
    if (!word || word.type !== "name") fail("a step has no node test");
    const colon = word.value.indexOf(":");
    const local = word.value.slice(colon + 1);
    return { kind: "name", local, lower: asciiLower(local), ns: colon < 0 ? null : namespace(word.value.slice(0, colon)) };
  };
  const step = () => {
    if (accept(".")) return xpathStep("self", { kind: "node" }, []);
    if (accept("..")) return xpathStep("parent", { kind: "node" }, []);
    let axis = "child";
    if (accept("@")) axis = "attribute";
    else if (is("axis")) {
      axis = words[at++].value;
      take("::");
    }
    return xpathStep(axis, test(), predicates());
  };
  // A // before a child step is one step: its descendant step when it
  // counts no position, a grouped one when it does, of elements.
  const trail = (steps) => {
    for (;;) {
      const next = step(), before = steps[steps.length - 1];
      const counts = next.predicates.some((p) => p.returns === "number" || positioned(p));
      if (before && before.anywhere && next.axis === "child" && (!counts || next.elements)) {
        steps[steps.length - 1] = xpathStep("descendant", next.test, next.predicates, counts);
      } else {
        steps.push(next);
      }
      if (accept("op", "//")) steps.push(xpathStep("descendant-or-self", { kind: "node" }, []));
      else if (!accept("op", "/")) return steps;
    }
  };
  const call = (name) => {
    if (!Object.hasOwn(xpathFunctions, name)) fail(`XPath has no function '${name}'`);
    const [returns, takes, run] = xpathFunctions[name];
    const kinds = takes ? takes.split(" ") : [];
    const args = [];
    take("(");
    if (!accept(")")) {
      do args.push(or(0)); while (accept(","));
      take(")");
    }
    const many = kinds.length > 0 && kinds[kinds.length - 1].endsWith("*");
    if (args.length < kinds.filter((k) => /\w$/.test(k)).length || (args.length > kinds.length && !many)) {
      fail(`${name}() does not take ${args.length} argument${args.length === 1 ? "" : "s"}`);
    }
    const as = args.map((arg, i) => kinds[Math.min(i, kinds.length - 1)].replace(/\W$/, ""));
    args.forEach((arg, i) => as[i] === "nodes" && nodes(arg, `what ${name}() is given`));
    return { kind: "call", name, run, args, as: as.map((kind) => xpathAs[kind]), returns };
  };
  const primary = () => {
    const word = words[at++];
    switch (word.type) {
      case "variable": return fail(`light binds no variable, and ${word.value} is one`);
      case "literal": return { kind: "literal", value: word.value, returns: "string" };
      case "number": return { kind: "number", value: Number(word.value), returns: "number" };
      case "function": return call(word.value);
      default: {
        const inner = or(0);
        take(")");
        return inner;
      }
    }
  };
  const path = () => {
    if (at >= words.length) fail("it ends where an operand must stand");
    if (["variable", "(", "literal", "number", "function"].includes(words[at].type)) {
      const start = primary();
      const filters = predicates();
      const steps = [];
      if (accept("op", "//")) {
        steps.push(xpathStep("descendant-or-self", { kind: "node" }, []));
        trail(steps);
      } else if (accept("op", "/")) {
        trail(steps);
      } else if (!filters.length) {
        return start;
      }
      return { kind: "path", start: nodes(start, "what is filtered or stepped from"), filters, steps, returns: "nodes" };
    }
    const steps = [];
    let absolute = false;
    if (accept("op", "//")) {
      absolute = true;
      steps.push(xpathStep("descendant-or-self", { kind: "node" }, []));
      trail(steps);
    } else if (accept("op", "/")) {
      absolute = true;
      if (["axis", "name", "kind", ".", "..", "@"].some((type) => is(type))) trail(steps);
    } else {
      trail(steps);
    }
    return { kind: "path", start: null, absolute, filters: [], steps, returns: "nodes" };
  };
  const union = () => {
    let left = path();
    while (accept("op", "|")) {
      left = { kind: "union", left: nodes(left, "a side of |"), right: nodes(path(), "a side of |"), returns: "nodes" };
    }
    return left;
  };
  const unary = () => (accept("op", "-") ? { kind: "negate", operand: unary(), returns: "number" } : union());
  const or = (level) => {
    if (level === xpathLevels.length) return unary();
    let left = or(level + 1);
    while (at < words.length && words[at].type === "op" && xpathLevels[level].includes(words[at].value)) {
      left = { kind: words[at++].value, left, right: or(level + 1), returns: xpathReturns[level] };
    }
    return left;
  };

  const tree = or(0);
  if (at < words.length) fail(`it goes on after its end, with '${words[at].value || words[at].type}'`);
  return tree;
};

// ---- evaluating

const xpathValue = (e, node, position, size) => {
  const of = (part) => xpathValue(part, node, position, size);
  switch (e.kind) {
    case "number": case "literal": return e.value;
    case "or": return xpathBoolean(of(e.left)) || xpathBoolean(of(e.right));
    case "and": return xpathBoolean(of(e.left)) && xpathBoolean(of(e.right));
    case "+": return xpathNumber(of(e.left)) + xpathNumber(of(e.right));
    case "-": return xpathNumber(of(e.left)) - xpathNumber(of(e.right));
    case "*": return xpathNumber(of(e.left)) * xpathNumber(of(e.right));
    case "div": return xpathNumber(of(e.left)) / xpathNumber(of(e.right));
    case "mod": return xpathNumber(of(e.left)) % xpathNumber(of(e.right));
    case "negate": return -xpathNumber(of(e.operand));
    case "union": {
      const left = of(e.left), right = of(e.right);
      return left.length && right.length ? inOrder(left.concat(right)) : left.length ? left : right;
    }
    case "call": return e.run(node, position, size, ...e.args.map((arg, i) => e.as[i](of(arg))));
    case "path": {
      let nodes = e.start ? of(e.start) : [e.absolute ? rootOf(node) : node];
      for (const filter of e.filters) nodes = sifted(nodes, filter);
      for (const step of e.steps) nodes = stepped(step, nodes);
      return nodes;
    }
    default: return xpathCompares(e.kind, of(e.left), of(e.right));
  }
};

const ANY_TYPE = 0, NUMBER_TYPE = 1, STRING_TYPE = 2, BOOLEAN_TYPE = 3, UNORDERED_NODE_ITERATOR_TYPE = 4,
  ORDERED_NODE_ITERATOR_TYPE = 5, UNORDERED_NODE_SNAPSHOT_TYPE = 6, ORDERED_NODE_SNAPSHOT_TYPE = 7,
  ANY_UNORDERED_NODE_TYPE = 8, FIRST_ORDERED_NODE_TYPE = 9;
const xpathTypeOf = { number: NUMBER_TYPE, string: STRING_TYPE, boolean: BOOLEAN_TYPE, object: UNORDERED_NODE_ITERATOR_TYPE };

// An iterator is over a document that has not changed since: a
// mutation observer of its own tells, let go of when the iteration
// ends, goes stale or is forgotten.
const xpathForgotten = new FinalizationRegistry((watch) => watch.disconnect());
const unwatched = (result) => {
  if (!result._watch) return;
  result._watch.disconnect();
  xpathForgotten.unregister(result);
  result._watch = null;
};
const staleOf = (result) => {
  if (result._watch && (result._seen.changed || result._watch.takeRecords().length > 0)) {
    result._stale = true;
    unwatched(result);
  }
  return result._stale;
};
const wrongType = (what) => new TypeError(`The result is not ${what}.`);

class XPathResult {
  constructor(key, value, type, context) {
    if (key !== token) illegal();
    this.resultType = type === ANY_TYPE ? xpathTypeOf[typeof value] : type;
    define(this, { _value: value, _at: 0, _stale: false, _watch: null, _seen: null });
    if (this.resultType === NUMBER_TYPE) this._value = xpathNumber(value);
    else if (this.resultType === STRING_TYPE) this._value = xpathString(value);
    else if (this.resultType === BOOLEAN_TYPE) this._value = xpathBoolean(value);
    else if (typeof value !== "object") throw wrongType("a node-set, and no other can be given as one");
    else if (this._iterates() && value.length > 0) {
      const seen = this._seen = { changed: false };
      this._watch = new MutationObserver(() => { seen.changed = true; });
      this._watch.observe(rootOf(context), { childList: true, attributes: true, characterData: true, subtree: true });
      xpathForgotten.register(this, this._watch, this);
    }
  }
  _iterates() { return this.resultType === UNORDERED_NODE_ITERATOR_TYPE || this.resultType === ORDERED_NODE_ITERATOR_TYPE; }
  _snapshot() { return this.resultType === UNORDERED_NODE_SNAPSHOT_TYPE || this.resultType === ORDERED_NODE_SNAPSHOT_TYPE; }
  get numberValue() {
    if (this.resultType !== NUMBER_TYPE) throw wrongType("a number");
    return this._value;
  }
  get stringValue() {
    if (this.resultType !== STRING_TYPE) throw wrongType("a string");
    return this._value;
  }
  get booleanValue() {
    if (this.resultType !== BOOLEAN_TYPE) throw wrongType("a boolean");
    return this._value;
  }
  get singleNodeValue() {
    if (this.resultType !== ANY_UNORDERED_NODE_TYPE && this.resultType !== FIRST_ORDERED_NODE_TYPE) throw wrongType("a single node");
    return this._value[0] || null;
  }
  get invalidIteratorState() { return this._iterates() && staleOf(this); }
  get snapshotLength() {
    if (!this._snapshot()) throw wrongType("a snapshot");
    return this._value.length;
  }
  iterateNext() {
    if (!this._iterates()) throw wrongType("an iterator");
    if (staleOf(this)) throw new DOMException("The document has changed since the result was returned.", "InvalidStateError");
    const next = this._value[this._at++] || null;
    if (next === null) unwatched(this);
    return next;
  }
  snapshotItem(index) {
    if (!this._snapshot()) throw wrongType("a snapshot");
    return this._value[index >>> 0] || null;
  }
}
const xpathTypes = {
  ANY_TYPE, NUMBER_TYPE, STRING_TYPE, BOOLEAN_TYPE, UNORDERED_NODE_ITERATOR_TYPE, ORDERED_NODE_ITERATOR_TYPE,
  UNORDERED_NODE_SNAPSHOT_TYPE, ORDERED_NODE_SNAPSHOT_TYPE, ANY_UNORDERED_NODE_TYPE, FIRST_ORDERED_NODE_TYPE,
};
define(XPathResult, xpathTypes);
define(XPathResult.prototype, xpathTypes);

// xpathResult evaluates a tree from a context node: a node of the
// page's, or an Attr of one of its elements.
const xpathResult = (tree, context, type) => {
  const attribute = context instanceof Attr && context.ownerElement instanceof Element;
  if (!attribute && (!(context instanceof Node) || context[REF] === undefined)) throw new TypeError("The context is not a Node.");
  if (context instanceof DocumentType) throw new DOMException("A document type is no context of XPath's.", "NotSupportedError");
  type = type === undefined ? ANY_TYPE : Number(type);
  if (!Object.values(xpathTypes).includes(type)) throw new DOMException(`${type} is no type of result.`, "NotSupportedError");
  if (attribute) context = attributeOf(context.ownerElement, context.name, context.value, context);
  xpathForget();
  try {
    return new XPathResult(token, xpathValue(tree, context, 1, 1), type, context);
  } finally {
    xpathForget();
  }
};

class XPathExpression {
  constructor(key, tree) {
    if (key !== token) illegal();
    define(this, { _tree: tree });
  }
  evaluate(contextNode, type) { return xpathResult(this._tree, contextNode, type); }
}
const xpathMethods = {
  createExpression(expression, resolver = null) { return new XPathExpression(token, xpathTree(String(expression), resolver)); },
  createNSResolver(nodeResolver) { return nodeResolver; },
  evaluate(expression, contextNode, resolver = null, type = ANY_TYPE) {
    return xpathResult(xpathTree(String(expression), resolver), contextNode, type);
  },
};
class XPathEvaluator {}
define(XPathEvaluator.prototype, xpathMethods);
define(Document.prototype, xpathMethods);
define(window, { XPathResult, XPathExpression });
