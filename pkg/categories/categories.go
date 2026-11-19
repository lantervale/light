// Package categories is the categories a reader sorts pages into, in
// plain Go: an ordered list of their own, each a name, what belongs in
// it in their words, a mark and what light does with a page of it; what
// they chose for a site or a page, which wins over any judgment; the
// verdict for a page; and how many pages of each were read, by day.
//
// The first category that fits wins, and Default, everything else, is
// always the last. A verdict is a hint: nothing here says a page is
// safe.
package categories

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Action is what light does with a page of a category.
type Action string

const (
	// NoMark shows nothing.
	NoMark Action = "none"
	// Marked shows a label in the address bar and a line under the tab.
	Marked Action = "mark"
	// Warn shows the label, and a bar under the header before the page
	// is typed into.
	Warn Action = "warn"
	// Dim covers the page until its reader chooses to see it.
	Dim Action = "dim"
)

// Actions are the actions, the quietest first.
func Actions() []Action { return []Action{NoMark, Marked, Warn, Dim} }

func (a Action) String() string {
	switch a {
	case NoMark:
		return "No mark"
	case Marked:
		return "Mark"
	case Warn:
		return "Warn first"
	case Dim:
		return "Dim or hide"
	}
	return string(a)
}

func (a Action) valid() bool { return slices.Contains(Actions(), a) }

// Shape is the shape of a mark: a category is never told by its colour
// alone.
type Shape string

const (
	Dot      Shape = "dot"
	Square   Shape = "square"
	Frame    Shape = "frame"
	Ring     Shape = "ring"
	Diamond  Shape = "diamond"
	Triangle Shape = "triangle"
)

// Shapes are the shapes a mark has.
func Shapes() []Shape { return []Shape{Dot, Square, Frame, Ring, Diamond, Triangle} }

// Mark is what stands for a category: a colour and a shape.
type Mark struct {
	Colour string `json:"colour"`
	Shape  Shape  `json:"shape"`
}

// The colours of the marks.
const (
	purple = "#b48cf2"
	amber  = "#f0b354"
	green  = "#5ad18a"
	blue   = "#5aa9e6"
	pink   = "#f28cc8"
	teal   = "#4fd1c5"
	red    = "#ff6b6b"
	grey   = "#8f8c9c"
)

// Marks are the marks a category is given to choose from, none twice
// the same colour or the same shape as the one before it.
func Marks() []Mark {
	return []Mark{
		{purple, Dot}, {amber, Square}, {green, Frame}, {blue, Diamond}, {pink, Ring},
		{teal, Square}, {red, Triangle}, {amber, Triangle}, {blue, Dot}, {green, Diamond},
	}
}

func (m Mark) valid() bool {
	if len(m.Colour) != 7 || m.Colour[0] != '#' || !slices.Contains(Shapes(), m.Shape) {
		return false
	}
	return strings.Trim(strings.ToLower(m.Colour[1:]), "0123456789abcdef") == ""
}

// Category is one category: its name, what belongs in it as its reader
// says it, its mark and what light does with a page of it.
type Category struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mark
	Action Action `json:"action"`
}

const (
	// DefaultName names the category of everything else.
	DefaultName = "Default"
	// maxName and maxDescription are the longest a name and a
	// description are kept, in bytes; maxPages how many pages are kept
	// a choice for, and keptDays how many days of reading.
	maxName        = 40
	maxDescription = 240
	maxPages       = 500
	keptDays       = 60
	day            = "2006-01-02"
)

// Default is the category of everything else: it is always the last,
// and shows nothing.
func Default() Category {
	return Category{Name: DefaultName, Description: "everything else", Mark: Mark{grey, Ring}, Action: NoMark}
}

// shipped are the categories of a new list.
func shipped() []Category {
	return []Category{
		{"Danger", "asks for a password or card where it should not, or imitates another site",
			Mark{red, Triangle}, Warn},
		{"Politics", "elections, parties, governments and what they decide", Mark{purple, Dot}, Marked},
		{"Selling to me", "the page mainly wants me to buy or subscribe", Mark{amber, Square}, Marked},
		{"Work", "what I work on and with", Mark{green, Frame}, Marked},
	}
}

// By is who chose a page's category.
type By string

const (
	// ByModel is the model's judgment of the page's text.
	ByModel By = "model"
	// BySite is its reader's choice for every page of the site.
	BySite By = "site"
	// ByPage is its reader's choice for the page.
	ByPage By = "page"
)

// Verdict is the category of a page, why, and who chose it.
type Verdict struct {
	Category
	Reason string
	By     By
}

// Choice is a category its reader chose for a site, or for a page.
type Choice struct {
	Site     string `json:"site,omitempty"`
	Address  string `json:"address,omitempty"`
	Category string `json:"category"`
}

// Count is how many pages of a category were read.
type Count struct {
	Category
	Pages int
}

// List is a reader's categories in their order, what they chose for
// sites and pages, and what they read.
type List struct {
	own   []Category
	sites []Choice
	pages []Choice
	read  map[string]map[string]int
	// asked counts the changes of what a model is asked, and version
	// every change.
	asked, version uint64
}

// New is the list light ships: Danger, Politics, Selling to me, Work,
// then Default.
func New() *List { return &List{own: shipped(), read: map[string]map[string]int{}} }

// All are the categories in order, Default the last.
func (l *List) All() []Category { return append(slices.Clone(l.own), Default()) }

// Names are the names of the categories, in order.
func (l *List) Names() []string {
	all := l.All()
	names := make([]string, len(all))
	for i, c := range all {
		names[i] = c.Name
	}
	return names
}

// Asked changes when what a model is asked of a page did: the names,
// the descriptions or their order.
func (l *List) Asked() uint64 { return l.asked }

// Version changes with every change of the list.
func (l *List) Version() uint64 { return l.version }

func (l *List) changed(asked bool) {
	l.version++
	if asked {
		l.asked++
	}
}

func (l *List) find(name string) int {
	return slices.IndexFunc(l.own, func(c Category) bool { return strings.EqualFold(c.Name, name) })
}

// Named is the category of that name, whatever its case.
func (l *List) Named(name string) (Category, bool) {
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, DefaultName) {
		return Default(), true
	}
	if i := l.find(name); i >= 0 {
		return l.own[i], true
	}
	return Category{}, false
}

// line is text on one line, its white space single, at most size bytes
// of it.
func line(text string, size int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= size {
		return text
	}
	for size > 0 && !utf8.RuneStart(text[size]) {
		size--
	}
	return strings.TrimSpace(text[:size])
}

// named is a name for a category described so: the words before a
// colon, else its first words up to one that only joins.
func named(description string) (string, string) {
	if name, rest, ok := strings.Cut(description, ":"); ok && strings.TrimSpace(name) != "" &&
		strings.TrimSpace(rest) != "" && len(name) <= maxName {
		return strings.TrimSpace(name), strings.TrimSpace(rest)
	}
	var words []string
	for _, word := range strings.Fields(description) {
		word = strings.TrimFunc(word, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if joins := slices.Contains([]string{"a", "an", "the", "of", "to", "and", "or", "in", "on", "for", "with",
			"without", "that", "which", "about", "from"}, strings.ToLower(word)); word == "" || joins && len(words) > 0 {
			break
		}
		if words = append(words, word); len(words) == 3 {
			break
		}
	}
	name := strings.Join(words, " ")
	first, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(first)) + name[size:], description
}

// free is name, numbered until no category but the one at but has it.
func (l *List) free(name string, but int) string {
	taken := func(name string) bool {
		i := l.find(name)
		return strings.EqualFold(name, DefaultName) || i >= 0 && i != but
	}
	if !taken(name) {
		return name
	}
	for n := 2; ; n++ {
		if numbered := fmt.Sprintf("%s %d", name, n); !taken(numbered) {
			return numbered
		}
	}
}

// spare is the first mark no category has, the first of all when each
// is taken.
func (l *List) spare() Mark {
	for _, m := range Marks() {
		if !slices.ContainsFunc(l.own, func(c Category) bool { return c.Mark == m }) {
			return m
		}
	}
	return Marks()[0]
}

// Add adds a category for the pages described, the last before Default:
// named by the words before a colon, else by the first words of the
// description, with a mark no other has, and marked. It reports false
// for a description that says nothing.
func (l *List) Add(description string) (Category, bool) {
	name, description := named(line(description, maxDescription))
	if name = line(name, maxName); name == "" || description == "" {
		return Category{}, false
	}
	c := Category{Name: l.free(name, -1), Description: description, Mark: l.spare(), Action: Marked}
	l.own = append(l.own, c)
	l.changed(true)
	return c, true
}

// Edit changes the category name to what to says: a name that is empty
// or Default's keeps the one it had, one another category has is
// numbered, and a mark or an action light does not know keeps the one
// it had. What was chosen and read under the old name follows the new.
func (l *List) Edit(name string, to Category) bool {
	i := l.find(name)
	if i < 0 {
		return false
	}
	was := l.own[i]
	to.Name, to.Description = line(to.Name, maxName), line(to.Description, maxDescription)
	if to.Name == "" || strings.EqualFold(to.Name, DefaultName) {
		to.Name = was.Name
	}
	to.Name = l.free(to.Name, i)
	if to.Description == "" {
		to.Description = was.Description
	}
	if !to.valid() {
		to.Mark = was.Mark
	}
	if !to.Action.valid() {
		to.Action = was.Action
	}
	if to == was {
		return false
	}
	l.own[i] = to
	if to.Name != was.Name {
		l.rename(was.Name, to.Name)
	}
	l.changed(to.Name != was.Name || to.Description != was.Description)
	return true
}

func (l *List) rename(from, to string) {
	for _, choices := range [][]Choice{l.sites, l.pages} {
		for i := range choices {
			if choices[i].Category == from {
				choices[i].Category = to
			}
		}
	}
	for _, pages := range l.read {
		if n, ok := pages[from]; ok {
			delete(pages, from)
			pages[to] += n
		}
	}
}

// Remove takes the category name off the list, with what was chosen
// for it. Default stays.
func (l *List) Remove(name string) bool {
	i := l.find(name)
	if i < 0 {
		return false
	}
	gone := l.own[i].Name
	l.own = slices.Delete(l.own, i, i+1)
	chose := func(c Choice) bool { return c.Category == gone }
	l.sites, l.pages = slices.DeleteFunc(l.sites, chose), slices.DeleteFunc(l.pages, chose)
	l.changed(true)
	return true
}

// Move moves the category name by places in the order, as far as the
// first place and the last before Default.
func (l *List) Move(name string, by int) bool {
	i := l.find(name)
	if i < 0 {
		return false
	}
	to := min(max(i+by, 0), len(l.own)-1)
	if to == i {
		return false
	}
	c := l.own[i]
	l.own = slices.Insert(slices.Delete(l.own, i, i+1), to, c)
	l.changed(true)
	return true
}

// SiteOf names the site of an address: its host without a leading www.
func SiteOf(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// pageOf is an address as a page is kept by: without its fragment.
func pageOf(address string) string {
	page, _, _ := strings.Cut(address, "#")
	return page
}

// ChooseSite has every page of the site of address be of the category
// name, whatever a model makes of it.
func (l *List) ChooseSite(address, name string) bool {
	c, known := l.Named(name)
	site := SiteOf(address)
	if !known || site == "" {
		return false
	}
	l.sites = append(slices.DeleteFunc(l.sites, func(c Choice) bool { return c.Site == site }),
		Choice{Site: site, Category: c.Name})
	l.changed(false)
	return true
}

// ChoosePage has the page at address be of the category name: a
// correction, which wins over the choice for its site.
func (l *List) ChoosePage(address, name string) bool {
	c, known := l.Named(name)
	page := pageOf(address)
	if !known || page == "" {
		return false
	}
	l.pages = append(slices.DeleteFunc(l.pages, func(c Choice) bool { return c.Address == page }),
		Choice{Address: page, Category: c.Name})
	if len(l.pages) > maxPages {
		l.pages = slices.Delete(l.pages, 0, len(l.pages)-maxPages)
	}
	l.changed(false)
	return true
}

// Forget drops a choice made for a site or for a page.
func (l *List) Forget(choice Choice) bool {
	sites, pages := len(l.sites), len(l.pages)
	same := func(c Choice) bool { return c.Site == choice.Site && c.Address == choice.Address }
	l.sites, l.pages = slices.DeleteFunc(l.sites, same), slices.DeleteFunc(l.pages, same)
	if len(l.sites) == sites && len(l.pages) == pages {
		return false
	}
	l.changed(false)
	return true
}

// ForgetSite drops the choice made for the site of address.
func (l *List) ForgetSite(address string) bool { return l.Forget(Choice{Site: SiteOf(address)}) }

// Choices are the choices made, for sites then for pages, the oldest
// first.
func (l *List) Choices() []Choice { return append(slices.Clone(l.sites), l.pages...) }

// Site is the category chosen for every page of the site of address.
func (l *List) Site(address string) (Category, bool) {
	site := SiteOf(address)
	if i := slices.IndexFunc(l.sites, func(c Choice) bool { return c.Site == site }); i >= 0 && site != "" {
		return l.Named(l.sites[i].Category)
	}
	return Category{}, false
}

// Chosen is what its reader chose for the page at address: for the
// page itself, else for its site. A model is not asked about such a
// page.
func (l *List) Chosen(address string) (Verdict, bool) {
	page := pageOf(address)
	if i := slices.IndexFunc(l.pages, func(c Choice) bool { return c.Address == page }); i >= 0 {
		if c, ok := l.Named(l.pages[i].Category); ok {
			return Verdict{Category: c, Reason: "You chose it for this page.", By: ByPage}, true
		}
	}
	if c, ok := l.Site(address); ok {
		return Verdict{Category: c, Reason: "You chose it for every page of " + SiteOf(address) + ".", By: BySite}, true
	}
	return Verdict{}, false
}

// Verdict is the category of the page at address: what its reader
// chose, else the one a model answered and the reason it gave. An
// answer that names no category is Default, and no answer no verdict.
func (l *List) Verdict(address, answer, reason string) (Verdict, bool) {
	if chosen, ok := l.Chosen(address); ok {
		return chosen, true
	}
	if strings.TrimSpace(answer) == "" {
		return Verdict{}, false
	}
	c, known := l.Named(answer)
	if !known {
		c = Default()
	}
	if c.Name == DefaultName {
		reason = "It matched none of your other categories."
	}
	return Verdict{Category: c, Reason: Sentence(reason), By: ByModel}, true
}

// Sentence is what a model gave as a reason, as it is shown: its first
// line, on one line, no longer than a sentence is.
func Sentence(reason string) string {
	const longest = 220
	first, _, _ := strings.Cut(strings.TrimSpace(reason), "\n")
	first = line(first, longest)
	if first != "" && !strings.ContainsAny(first[len(first)-1:], ".!?") {
		first += "."
	}
	return first
}

// Question is what a model is asked of a page: the categories in
// order, each with what belongs in it, and to name the first that fits.
func (l *List) Question() string {
	var b strings.Builder
	b.WriteString("You sort a web page into one of its reader's own categories. The categories, in the reader's " +
		"order, each with what the reader says belongs in it:\n")
	for _, c := range l.All() {
		b.WriteString("- " + c.Name + ": " + c.Description + "\n")
	}
	b.WriteString("Take the first category of the list that fits the page. Answer with the name of that one " +
		"category and nothing else: " + strings.Join(l.Names(), ", ") + ".")
	return b.String()
}

// Why is what a model is asked for the reason a page is of c.
func Why(c Category) string {
	return "A web page was sorted into its reader's category \"" + c.Name + "\" (" + c.Description + "). In one " +
		"short sentence that begins with \"Because\", say what in the page puts it there."
}

// Read notes a page of the category name read at at.
func (l *List) Read(name string, at time.Time) {
	c, known := l.Named(name)
	if !known {
		return
	}
	on := at.Format(day)
	if l.read[on] == nil {
		l.read[on] = map[string]int{}
	}
	l.read[on][c.Name]++
	oldest := at.AddDate(0, 0, -keptDays).Format(day)
	for d := range l.read {
		if d < oldest {
			delete(l.read, d)
		}
	}
	l.changed(false)
}

// Unread takes back a page noted read at at under the category name.
func (l *List) Unread(name string, at time.Time) {
	pages := l.read[at.Format(day)]
	if pages[name] == 0 {
		return
	}
	if pages[name]--; pages[name] == 0 {
		delete(pages, name)
	}
	l.changed(false)
}

// ForgetRead forgets what was read.
func (l *List) ForgetRead() {
	if len(l.read) == 0 {
		return
	}
	l.read = map[string]map[string]int{}
	l.changed(false)
}

// Week is how many pages of each category but Default were read in the
// week of now, from its Monday, in the list's order and without those
// none was read of, and how many pages were read in all.
func (l *List) Week(now time.Time) ([]Count, int) {
	monday := now.AddDate(0, 0, -(int(now.Weekday())+6)%7).Format(day)
	today := now.Format(day)
	pages := map[string]int{}
	total := 0
	for d, read := range l.read {
		if d < monday || d > today {
			continue
		}
		for name, n := range read {
			pages[name] += n
			total += n
		}
	}
	var counts []Count
	for _, c := range l.own {
		if pages[c.Name] > 0 {
			counts = append(counts, Count{c, pages[c.Name]})
		}
	}
	return counts, total
}
