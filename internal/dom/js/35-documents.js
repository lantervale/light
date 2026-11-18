// ---- the documents scripts make
//
// A document a script makes is a tree of its own on light's side, with
// every way of the page's document to make nodes and find them. Its
// nodes are its own until they are put in another document. The one
// createDocument makes is an XMLDocument; every other is a Document.

class XMLDocument extends Document {
  constructor(key, ref) {
    if (key !== token) illegal();
    super(key, ref);
  }
}

class CDATASection extends Text {
  constructor(key, ref) {
    if (key !== token) illegal();
    super(key, ref);
  }
  get nodeType() { return CDATA_NODE; }
  get nodeName() { return "#cdata-section"; }
}

class ProcessingInstruction extends CharacterData {
  constructor(key, ref, target) {
    if (key !== token) illegal();
    super(key, ref);
    define(this, { target });
  }
  get nodeType() { return INSTRUCTION_NODE; }
  get nodeName() { return this.target; }
}

const PARSER_ERROR_NS = "http://www.mozilla.org/newlayout/xml/parsererror.xml";
const xmlTypes = ["text/xml", "application/xml", "application/xhtml+xml", "image/svg+xml"];

class DOMImplementation {
  constructor(key, owner) {
    if (key !== token) illegal();
    define(this, { _owner: owner });
  }
  hasFeature() { return true; }
  createDocumentType(name, publicId, systemId) {
    name = String(name);
    if (/[\t\n\f\r \0>]/.test(name)) invalid("doctype name", name);
    return wrap(host("document", "doctype", name, String(publicId), String(systemId), this._owner[REF]));
  }
  // createDocument makes a document of XML, of the type its root's
  // namespace is of.
  createDocument(uri, name, doctype = null) {
    uri = uri === null || uri === undefined || uri === "" ? null : String(uri);
    name = name === null ? "" : String(name);
    const made = wrap(host("document", "new", uri === HTML_NS ? "application/xhtml+xml"
      : uri === namespaces.svg ? "image/svg+xml" : "application/xml", true));
    const root = name === "" ? null : made.createElementNS(uri, name);
    if (doctype) made.appendChild(doctype);
    if (root) made.appendChild(root);
    return made;
  }
  createHTMLDocument(title) {
    const made = wrap(host("document", "html"));
    if (title !== undefined) {
      made.head.appendChild(made.createElement("title")).appendChild(made.createTextNode(String(title)));
    }
    return made;
  }
}

// parseDocument reads markup as a document of its own, by the rules of
// its type. XML that is not well-formed gives the document browsers
// give: one parsererror element saying why.
const parseDocument = (markup, type) => {
  if (type !== "text/html" && !xmlTypes.includes(type)) {
    throw new TypeError(`DOMParser reads no document of type '${type}'.`);
  }
  const [ref, why] = host("document", "parse", String(markup), type, document.URL);
  if (ref) return wrap(ref);
  const failed = wrap(host("document", "new", type, false, document.URL));
  failed.appendChild(failed.createElementNS(PARSER_ERROR_NS, "parsererror")).textContent = why;
  return failed;
};

const writtenAsXML = (node) => {
  if (!(node instanceof Node) || node[REF] === undefined) throw new TypeError("XMLSerializer writes a Node.");
  return host("document", "markup", node[REF]);
};

define(window, { XMLDocument, CDATASection, ProcessingInstruction, DOMImplementation });
