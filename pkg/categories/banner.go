package categories

import "slices"

// Banner is the kind of the bar a category that warns shows over its
// pages.
type Banner string

const (
	// Info tells its reader something of the page.
	Info Banner = "info"
	// Warning asks for care: the banner of a category that names none.
	Warning Banner = "warning"
	// Danger says the page may harm.
	Danger Banner = "danger"
)

// Banners are the banners, the quietest first.
func Banners() []Banner { return []Banner{Info, Warning, Danger} }

func (b Banner) String() string {
	switch b {
	case Info:
		return "Information"
	case Warning:
		return "Warning"
	case Danger:
		return "Danger"
	}
	return string(b)
}

func (b Banner) valid() bool { return slices.Contains(Banners(), b) }
