package config

import (
	"net/url"
	"slices"
	"strings"
)

// validRecall is c with the sites never remembered each as its host,
// in lower case, once: what is no host is dropped.
func (c Config) validRecall() Config {
	var sites []string
	for _, typed := range strings.FieldsFunc(c.RecallExcluded, func(r rune) bool { return r == ',' || r == ' ' }) {
		if site := hostOf(typed); site != "" && !slices.Contains(sites, site) {
			sites = append(sites, site)
		}
	}
	c.RecallExcluded = strings.Join(sites, ", ")
	return c
}

// hostOf is the host what was typed names, an address or a host, without
// its port or a leading www.; "" for what names none.
func hostOf(typed string) string {
	typed = strings.ToLower(typed)
	if !strings.Contains(typed, "://") {
		typed = "//" + typed
	}
	u, err := url.Parse(typed)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")
	if host == "" || strings.ContainsFunc(host, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '-' && r != ':'
	}) {
		return ""
	}
	return host
}

// Excludes reports whether what the pages of a site say is never
// remembered: the site, by its host, is one of RecallExcluded or under
// one.
func (c Config) Excludes(site string) bool {
	for excluded := range strings.SplitSeq(c.RecallExcluded, ", ") {
		if excluded != "" && (site == excluded || strings.HasSuffix(site, "."+excluded)) {
			return true
		}
	}
	return false
}

// Excluding is c with a site added to those never remembered.
func (c Config) Excluding(site string) Config {
	c.RecallExcluded += ", " + site
	return c.validRecall()
}
