package app

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lantervale/light/pkg/cookies"
	"github.com/lantervale/light/pkg/webdriver"
)

// cookied is the address of the window's page, whose cookies the
// session reads and sets: a page of the web.
func (d *Driver) cookied() (*url.URL, error) {
	s, err := d.window()
	if err != nil {
		return nil, err
	}
	address := shownBy(s.browser.Tab().Current())
	at, err := url.Parse(address)
	if err != nil || !onTheWeb(address) {
		return nil, webdriver.Errorf(webdriver.InvalidCookieDomain, address+" keeps no cookie: it is no http or "+
			"https page")
	}
	return at, nil
}

// Cookies are the cookies the profile's jar holds for the window's
// page, those kept from scripts among them.
func (d *Driver) Cookies(ctx context.Context) ([]webdriver.Cookie, error) {
	var all []webdriver.Cookie
	err := d.once(ctx, func() error {
		at, err := d.cookied()
		if err != nil {
			return err
		}
		for _, c := range d.p.jar.For(at) {
			told := webdriver.Cookie{Name: c.Name, Value: c.Value, Path: c.Path, Domain: c.Domain, Secure: c.Secure,
				HTTPOnly: c.HTTPOnly, SameSite: sameSite(c.SameSite)}
			if c.Persistent() {
				expiry := c.Expires.Unix()
				told.Expiry = &expiry
			}
			all = append(all, told)
		}
		return nil
	})
	return all, err
}

func sameSite(of cookies.SameSite) string {
	switch of {
	case cookies.Strict:
		return "Strict"
	case cookies.None:
		return "None"
	case cookies.Lax:
	}
	return "Lax"
}

// AddCookie gives the jar a cookie as an answer of the window's page
// would set it, by the jar's own rules.
func (d *Driver) AddCookie(ctx context.Context, cookie webdriver.Cookie) error {
	return d.once(ctx, func() error {
		at, err := d.cookied()
		if err != nil {
			return err
		}
		host, domain := strings.ToLower(at.Hostname()), strings.ToLower(strings.TrimPrefix(cookie.Domain, "."))
		if domain != "" && host != domain && !strings.HasSuffix(host, "."+domain) {
			return webdriver.Errorf(webdriver.InvalidCookieDomain, "the page at "+host+" sets no cookie of "+domain)
		}
		//nolint:gosec // the cookie is the client's: what it says of Secure, HttpOnly and SameSite is kept as said
		set := http.Cookie{Name: cookie.Name, Value: cookie.Value, Path: cookie.Path, Secure: cookie.Secure,
			HttpOnly: cookie.HTTPOnly}
		if domain != "" && domain != host {
			set.Domain = domain
		}
		if set.Path == "" {
			set.Path = "/"
		}
		if cookie.Expiry != nil {
			set.Expires = time.Unix(*cookie.Expiry, 0)
		}
		switch cookie.SameSite {
		case "Strict":
			set.SameSite = http.SameSiteStrictMode
		case "None":
			set.SameSite = http.SameSiteNoneMode
		case "Lax":
			set.SameSite = http.SameSiteLaxMode
		}
		line := set.String()
		if line != "" {
			d.p.jar.Store(cookies.Request{URL: at}, []string{line})
		}
		for _, kept := range d.p.jar.For(at) {
			if kept.Name == cookie.Name && kept.Value == cookie.Value {
				return nil
			}
		}
		return webdriver.Errorf(webdriver.UnableToSetCookie, "light's jar did not keep the cookie "+cookie.Name+
			": it is not one its rules allow the page, or cookies are off in the settings")
	})
}

// DeleteCookies ends the cookie of that name the window's page has,
// each of them for "".
func (d *Driver) DeleteCookies(ctx context.Context, name string) error {
	return d.once(ctx, func() error {
		at, err := d.cookied()
		if err != nil {
			return err
		}
		for _, kept := range d.p.jar.For(at) {
			if name != "" && kept.Name != name {
				continue
			}
			//nolint:gosec // a cookie that ends one of the jar's: nothing is kept of it
			ended := http.Cookie{Name: kept.Name, Path: kept.Path, MaxAge: -1, Secure: kept.Secure}
			if !kept.HostOnly {
				ended.Domain = kept.Domain
			}
			by := *at
			if kept.Secure {
				by.Scheme = "https"
			}
			d.p.jar.Store(cookies.Request{URL: &by}, []string{ended.String()})
		}
		return nil
	})
}
