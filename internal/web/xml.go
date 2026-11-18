package web

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

// The namespaces XML documents name, and what the tree calls them: the
// elements of XHTML have none there, as those HTML is read into, SVG's
// and MathML's the short names HTML's parser gives them, any other its
// address, and an element of no namespace NoNamespace.
const (
	xhtmlNamespace  = "http://www.w3.org/1999/xhtml"
	svgAddress      = "http://www.w3.org/2000/svg"
	mathAddress     = "http://www.w3.org/1998/Math/MathML"
	xmlAddress      = "http://www.w3.org/XML/1998/namespace"
	xmlnsAddress    = "http://www.w3.org/2000/xmlns/"
	xlinkAddress    = "http://www.w3.org/1999/xlink"
	mathNamespace   = "math"
	xmlPrefix       = "xml"
	xmlnsPrefix     = "xmlns"
	xlinkPrefix     = "xlink"
	doctypePublic   = "public"
	doctypeSystem   = "system"
	cdataOpen       = "<![CDATA["
	xmlDeclaration  = "xml"
	doctypeKeyword  = "DOCTYPE"
	publicKeyword   = "PUBLIC"
	systemKeyword   = "SYSTEM"
	maxDeclaration  = 256
	entityReference = `&([A-Za-z][A-Za-z0-9]*);`
)

// What the Namespace of a node that is no element says of it, and of an
// element that it has none.
const (
	// CDATASection marks a text node written as a CDATA section.
	CDATASection = "cdata"
	// Instruction marks a comment node that is a processing instruction:
	// its Data is the instruction's, the key of its one attribute its
	// target.
	Instruction = "pi"
	// NoNamespace is the Namespace of an element that has none.
	NoNamespace = "-"
)

// Space is what the tree calls the namespace at address.
func Space(address string) string {
	switch address {
	case xhtmlNamespace:
		return ""
	case svgAddress:
		return svgNamespace
	case mathAddress:
		return mathNamespace
	case "":
		return NoNamespace
	}
	return address
}

// NamespaceOf is the address of the namespace the tree calls space, ""
// for none.
func NamespaceOf(space string) string {
	switch space {
	case "":
		return xhtmlNamespace
	case svgNamespace:
		return svgAddress
	case mathNamespace:
		return mathAddress
	case NoNamespace:
		return ""
	}
	return space
}

// IsXML reports whether a document of that media type is read by XML's
// rules.
func IsXML(media string) bool {
	return media == "application/xhtml+xml" || media == "application/xml" || media == "text/xml" ||
		strings.HasSuffix(media, "+xml")
}

// XMLError is why a document is not well-formed XML, and where.
type XMLError struct {
	Line, Column int
	Reason       string
}

func (e *XMLError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Reason)
}

// XMLScope is what a piece of XML is read within: the namespace of
// elements written without a prefix, the namespaces of the prefixes
// declared around it, and whether the names of HTML's entities stand
// (a document with an XHTML doctype).
type XMLScope struct {
	Default  string
	Prefixes map[string]string
	Entities bool
}

// ParseXML reads a whole document by XML's rules into the tree HTML is
// read into: one root element, its namespaces resolved, CDATA sections
// as text, nothing guessed. What is not well-formed is an *XMLError.
func ParseXML(source string) (*html.Node, error) {
	root := &html.Node{Type: html.DocumentNode}
	p := newXMLParser(source, XMLScope{})
	p.whole = true
	if err := p.read(root); err != nil {
		return nil, err
	}
	return root, nil
}

// ParseXMLFragment reads markup as what an element holds, within scope.
func ParseXMLFragment(markup string, scope XMLScope) ([]*html.Node, error) {
	holder := &html.Node{Type: html.DocumentNode}
	if err := newXMLParser(markup, scope).read(holder); err != nil {
		return nil, err
	}
	var nodes []*html.Node
	for c := holder.FirstChild; c != nil; c = holder.FirstChild {
		holder.RemoveChild(c)
		nodes = append(nodes, c)
	}
	return nodes, nil
}

type xmlParser struct {
	source string
	d      *xml.Decoder
	// whole is a document, with its one root, and not what an element
	// holds.
	whole bool
	// rooted is whether the root was read, and typed the doctype.
	rooted, typed bool
	open          []xmlOpen
	scope         XMLScope
	// named are HTML's entities the source names.
	named map[string]string
}

// xmlOpen is an element that is not closed yet: its name as written,
// and the scope around it.
type xmlOpen struct {
	name  xml.Name
	node  *html.Node
	outer XMLScope
}

func newXMLParser(source string, scope XMLScope) *xmlParser {
	p := &xmlParser{source: source, scope: scope, d: xml.NewDecoder(strings.NewReader(source))}
	p.d.Strict = true
	p.d.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	if scope.Entities {
		p.d.Entity = p.entities()
	}
	return p
}

var entityNames = regexp.MustCompile(entityReference)

// entities are the entities of HTML that the source names, by name.
func (p *xmlParser) entities() map[string]string {
	if p.named == nil {
		p.named = map[string]string{}
		for _, found := range entityNames.FindAllStringSubmatch(p.source, -1) {
			if value := html.UnescapeString(found[0]); value != found[0] {
				p.named[found[1]] = value
			}
		}
	}
	return p.named
}

func (p *xmlParser) fail(reason string, args ...any) error {
	line, column := p.d.InputPos()
	return &XMLError{Line: line, Column: column, Reason: fmt.Sprintf(reason, args...)}
}

func (p *xmlParser) failed(err error) error {
	var syntax *xml.SyntaxError
	if errors.As(err, &syntax) {
		return p.fail("%s", syntax.Msg)
	}
	return p.fail("%s", err.Error())
}

func (p *xmlParser) read(into *html.Node) error {
	for {
		at := p.d.InputOffset()
		token, err := p.d.RawToken()
		if errors.Is(err, io.EOF) {
			return p.ended()
		}
		if err != nil {
			return p.failed(err)
		}
		parent := into
		if len(p.open) > 0 {
			parent = p.open[len(p.open)-1].node
		}
		switch t := token.(type) {
		case xml.StartElement:
			err = p.start(parent, t)
		case xml.EndElement:
			err = p.end(t)
		case xml.CharData:
			err = p.text(parent, string(t), strings.HasPrefix(p.source[at:], cdataOpen))
		case xml.Comment:
			parent.AppendChild(&html.Node{Type: html.CommentNode, Data: string(t)})
		case xml.ProcInst:
			err = p.instruction(parent, t, at)
		case xml.Directive:
			err = p.directive(parent, string(t))
		}
		if err != nil {
			return err
		}
	}
}

func (p *xmlParser) ended() error {
	switch {
	case len(p.open) > 0:
		return p.fail("the document ends in the element %s, which is not closed", spelled(p.open[len(p.open)-1].name))
	case p.whole && !p.rooted:
		return p.fail("the document has no element")
	}
	return nil
}

// spelled is a name as its markup writes it.
func spelled(name xml.Name) string {
	if name.Space != "" {
		return name.Space + ":" + name.Local
	}
	return name.Local
}

// outside reports whether what comes now is out of the document's
// root: before it, or after it.
func (p *xmlParser) outside() bool { return p.whole && len(p.open) == 0 }

func (p *xmlParser) text(parent *html.Node, text string, cdata bool) error {
	if p.outside() {
		if !cdata && strings.TrimSpace(text) == "" {
			return nil
		}
		return p.fail("text outside the document's root element")
	}
	n := &html.Node{Type: html.TextNode, Data: text}
	if cdata {
		n.Namespace = CDATASection
	}
	parent.AppendChild(n)
	return nil
}

func (p *xmlParser) instruction(parent *html.Node, t xml.ProcInst, at int64) error {
	if strings.EqualFold(t.Target, xmlDeclaration) {
		if !p.whole || at != 0 || t.Target != xmlDeclaration {
			return p.fail("an XML declaration is only at the start of a document")
		}
		return nil
	}
	parent.AppendChild(&html.Node{Type: html.CommentNode, Namespace: Instruction, Data: string(t.Inst),
		Attr: []html.Attribute{{Key: t.Target}}})
	return nil
}

func (p *xmlParser) directive(parent *html.Node, text string) error {
	name, rest, _ := strings.Cut(strings.TrimSpace(text), " ")
	switch {
	case name != doctypeKeyword:
		return p.fail("<!%s is not read here", name)
	case !p.whole || p.rooted || p.typed:
		return p.fail("a doctype is only before the document's root element, once")
	}
	p.typed = true
	n := doctype(rest)
	if n == nil {
		return p.fail("the doctype is not one")
	}
	parent.AppendChild(n)
	if givesEntities(attr(n, doctypePublic)) {
		p.scope.Entities = true
		p.d.Entity = p.entities()
	}
	return nil
}

// doctype is the node of what a doctype says after its keyword: a
// name, then PUBLIC with two identifiers or SYSTEM with one, as HTML's
// parser keeps them. What it declares in brackets is not read.
func doctype(text string) *html.Node {
	fields := quotedFields(text)
	if len(fields) == 0 || fields[0] == "" {
		return nil
	}
	n := &html.Node{Type: html.DoctypeNode, Data: fields[0]}
	switch {
	case len(fields) >= 3 && fields[1] == publicKeyword:
		n.Attr = append(n.Attr, html.Attribute{Key: doctypePublic, Val: fields[2]})
		if len(fields) >= 4 {
			n.Attr = append(n.Attr, html.Attribute{Key: doctypeSystem, Val: fields[3]})
		}
	case len(fields) >= 3 && fields[1] == systemKeyword:
		n.Attr = append(n.Attr, html.Attribute{Key: doctypeSystem, Val: fields[2]})
	}
	return n
}

// quotedFields are the words of text up to a bracket, what quotes hold
// one word each.
func quotedFields(text string) []string {
	var fields []string
	for text = strings.TrimSpace(text); text != "" && text[0] != '['; text = strings.TrimSpace(text) {
		if quote := text[0]; quote == '"' || quote == '\'' {
			end := strings.IndexByte(text[1:], quote)
			if end < 0 {
				return nil
			}
			fields, text = append(fields, text[1:1+end]), text[2+end:]
			continue
		}
		end := strings.IndexAny(text, " \t\r\n[")
		if end < 0 {
			end = len(text)
		}
		fields, text = append(fields, text[:end]), text[end:]
	}
	return fields
}

func (p *xmlParser) start(parent *html.Node, t xml.StartElement) error {
	if p.outside() && p.rooted {
		return p.fail("the document has a second root element, %s", spelled(t.Name))
	}
	p.rooted = true
	if strings.Contains(t.Name.Local, ":") {
		return p.fail("%s is not a name: nothing stands before its colon, or after it", spelled(t.Name))
	}
	outer := p.scope
	if err := p.declare(t); err != nil {
		return err
	}
	space, err := p.resolve(t.Name.Space, true)
	if err != nil {
		return p.fail("the prefix of %s is not declared", spelled(t.Name))
	}
	n := &html.Node{Type: html.ElementNode, Data: t.Name.Local, Namespace: Space(space)}
	if n.Namespace == "" || n.Namespace == svgNamespace || n.Namespace == mathNamespace {
		n.DataAtom = atom.Lookup([]byte(n.Data))
	}
	seen := map[xml.Name]bool{}
	for _, a := range t.Attr {
		kept, name, failed := p.attribute(a)
		if failed != nil {
			return failed
		}
		if seen[name] {
			return p.fail("the attribute %s is written twice on %s", spelled(a.Name), spelled(t.Name))
		}
		seen[name] = true
		n.Attr = append(n.Attr, kept)
	}
	parent.AppendChild(n)
	p.open = append(p.open, xmlOpen{name: t.Name, node: n, outer: outer})
	return nil
}

// declare takes the namespaces an element declares into the scope of
// what it holds.
func (p *xmlParser) declare(t xml.StartElement) error {
	var prefixes map[string]string
	for _, a := range t.Attr {
		switch {
		case a.Name.Space == "" && a.Name.Local == xmlnsPrefix:
			p.scope.Default = a.Value
		case a.Name.Space == xmlnsPrefix:
			if a.Value == "" || a.Name.Local == xmlnsPrefix || (a.Name.Local == xmlPrefix) != (a.Value == xmlAddress) {
				return p.fail("the prefix %s cannot be declared so", a.Name.Local)
			}
			if prefixes == nil {
				prefixes = make(map[string]string, len(p.scope.Prefixes)+1)
				for prefix, address := range p.scope.Prefixes {
					prefixes[prefix] = address
				}
				p.scope.Prefixes = prefixes
			}
			prefixes[a.Name.Local] = a.Value
		}
	}
	return nil
}

var errUndeclared = errors.New("a prefix that is not declared")

// resolve is the namespace of a prefix: the default one for none on an
// element, none for none on an attribute.
func (p *xmlParser) resolve(prefix string, element bool) (string, error) {
	switch {
	case prefix == "" && element:
		return p.scope.Default, nil
	case prefix == "":
		return "", nil
	case prefix == xmlPrefix:
		return xmlAddress, nil
	case prefix == xmlnsPrefix:
		return xmlnsAddress, nil
	}
	address, declared := p.scope.Prefixes[prefix]
	if !declared {
		return "", errUndeclared
	}
	return address, nil
}

// attribute is an attribute as the tree keeps it, as HTML's parser
// keeps those of SVG: the ones of XML, XLink and the declarations of
// namespaces under those names, any other with a prefix under the name
// it was written with. Its name with its namespace tells it from the
// others.
func (p *xmlParser) attribute(a xml.Attr) (html.Attribute, xml.Name, error) {
	if strings.Contains(a.Name.Local, ":") {
		return html.Attribute{}, xml.Name{}, p.fail("%s is not a name: nothing stands before its colon, or after it",
			spelled(a.Name))
	}
	if a.Name.Space == "" && a.Name.Local == xmlnsPrefix {
		return html.Attribute{Key: xmlnsPrefix, Val: a.Value}, xml.Name{Space: xmlnsAddress, Local: xmlnsPrefix}, nil
	}
	space, err := p.resolve(a.Name.Space, false)
	if err != nil {
		return html.Attribute{}, xml.Name{}, p.fail("the prefix of the attribute %s is not declared", spelled(a.Name))
	}
	name := xml.Name{Space: space, Local: a.Name.Local}
	switch space {
	case "":
		return html.Attribute{Key: a.Name.Local, Val: a.Value}, name, nil
	case xmlAddress:
		return html.Attribute{Namespace: xmlPrefix, Key: a.Name.Local, Val: a.Value}, name, nil
	case xmlnsAddress:
		return html.Attribute{Namespace: xmlnsPrefix, Key: a.Name.Local, Val: a.Value}, name, nil
	case xlinkAddress:
		return html.Attribute{Namespace: xlinkPrefix, Key: a.Name.Local, Val: a.Value}, name, nil
	}
	return html.Attribute{Key: spelled(a.Name), Val: a.Value}, name, nil
}

func (p *xmlParser) end(t xml.EndElement) error {
	if len(p.open) == 0 {
		return p.fail("</%s> closes nothing", spelled(t.Name))
	}
	last := p.open[len(p.open)-1]
	if last.name != t.Name {
		return p.fail("</%s> closes the element %s", spelled(t.Name), spelled(last.name))
	}
	p.open, p.scope = p.open[:len(p.open)-1], last.outer
	return nil
}

var declaredEncoding = regexp.MustCompile(
	`^<\?xml[^>]*?\sencoding\s*=\s*["']([A-Za-z][A-Za-z0-9._-]*)["']`)

// DecodeXML is the text of an XML document: decoded by its byte order
// mark, else by the charset its Content-Type names, else by the
// encoding its XML declaration names, else as the UTF-8 it must be.
func DecodeXML(body []byte, contentType string) string {
	label := ""
	switch {
	case len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF:
		return string(body[3:])
	case len(body) >= 2 && body[0] == 0xFE && body[1] == 0xFF:
		label, body = "utf-16be", body[2:]
	case len(body) >= 2 && body[0] == 0xFF && body[1] == 0xFE:
		label, body = "utf-16le", body[2:]
	default:
		if _, params, err := mime.ParseMediaType(contentType); err == nil {
			label = params["charset"]
		}
		if found := declaredEncoding.FindSubmatch(body[:min(len(body), maxDeclaration)]); label == "" && found != nil {
			label = string(found[1])
		}
	}
	if e, name := charset.Lookup(label); e != nil && name != "utf-8" {
		if text, err := e.NewDecoder().Bytes(body); err == nil {
			return string(text)
		}
	}
	return string(body)
}
