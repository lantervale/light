package devtools

import (
	"fmt"
	"strings"
	"time"

	"github.com/lantervale/ember/ui"

	"github.com/lantervale/light/pkg/cookies"
)

const storageTab = "storage"

// Stored is what the site of a tab's page keeps in the browser.
type Stored struct {
	// Site is the site they are of, "" for a page that has none.
	Site string
	// Cookies are those a request of the page to itself carries.
	Cookies []cookies.Cookie
	// Local is the site's localStorage, a key and its value each.
	Local [][2]string
}

//nolint:gochecknoinits // the registration of the Storage tab
func init() {
	Register(Tab{ID: storageTab, Label: "Storage", Order: 400, Build: newStorage,
		Icon: `<path d="M3 4.5v7c0 1 2.2 1.8 5 1.8s5-.8 5-1.8v-7M3 4.5c0 1 2.2 1.8 5 1.8s5-.8 5-1.8-2.2-1.8-5-1.8-5 ` +
			`.8-5 1.8M3 8c0 1 2.2 1.8 5 1.8s5-.8 5-1.8"/>`})
}

// newStorage is the Storage tab: the cookies and the localStorage of
// the page's site, as they stood when the panel was last shown.
func newStorage(p *Panel) Tool {
	list := ui.Div(ui.ID("dt-storage"))
	return Tool{Body: ui.Div(ui.Class("dt-body"), list),
		Show: func() {
			fill(list)
			var kept Stored
			if p.host.Storage != nil {
				kept = p.host.Storage()
			}
			if kept.Site == "" {
				list.Append(ui.Div(ui.Class("dt-none"), ui.Text("This tab's page is of no site: it keeps nothing.")))
				p.SetFoot("")
				return
			}
			list.Append(ui.H4(ui.Text("Cookies")))
			for _, c := range kept.Cookies {
				row := pair(c.Name, c.Value)
				row.Append(ui.Span(ui.Class("dt-about"), ui.Text(about(c))))
				list.Append(row)
			}
			if len(kept.Cookies) == 0 {
				list.Append(ui.Div(ui.Class("dt-none"), ui.Text("No cookie goes to this page.")))
			}
			list.Append(ui.H4(ui.Text("Local storage")))
			for _, entry := range kept.Local {
				list.Append(pair(entry[0], entry[1]))
			}
			if len(kept.Local) == 0 {
				list.Append(ui.Div(ui.Class("dt-none"), ui.Text("The site stores nothing.")))
			}
			p.SetFoot(fmt.Sprintf("%s keeps %d cookies for this page and %d keys", kept.Site, len(kept.Cookies),
				len(kept.Local)))
		}}
}

// about is a cookie's attributes in a line.
func about(c cookies.Cookie) string {
	parts := []string{c.Domain}
	if !c.HostOnly {
		parts[0] = "." + c.Domain
	}
	parts = append(parts, c.Path)
	if c.Secure {
		parts = append(parts, "Secure")
	}
	if c.HTTPOnly {
		parts = append(parts, "HttpOnly")
	}
	parts = append(parts, "SameSite="+string(c.SameSite))
	if c.Persistent() {
		parts = append(parts, "until "+c.Expires.Local().Format(time.DateTime))
	} else {
		parts = append(parts, "until the run ends")
	}
	return strings.Join(parts, " · ")
}
