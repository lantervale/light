package cookies

import (
	"strconv"
	"strings"
	"time"
)

// parse reads one cookie as a Set-Cookie header or document.cookie
// gives it. It is false for what is no cookie: one without a name, over
// MaxSize, or holding a control character.
func parse(line string, now time.Time) (*Cookie, bool) {
	if strings.ContainsFunc(line, func(r rune) bool { return (r < ' ' && r != '\t') || r == 0x7f }) {
		return nil, false
	}
	pair, attributes, _ := strings.Cut(line, ";")
	name, value, named := strings.Cut(pair, "=")
	name, value = strings.Trim(name, " \t"), strings.Trim(value, " \t")
	if !named || name == "" || len(name)+len(value) > MaxSize {
		return nil, false
	}
	c := &Cookie{Name: name, Value: value, SameSite: Lax, Created: now, Used: now}
	var expires, aged time.Time
	for attribute := range strings.SplitSeq(attributes, ";") {
		name, value, _ = strings.Cut(attribute, "=")
		name, value = strings.ToLower(strings.Trim(name, " \t")), strings.Trim(value, " \t")
		if len(value) > MaxAttribute {
			continue
		}
		switch name {
		case "expires":
			if at, ok := parseDate(value); ok {
				expires = at
			}
		case "max-age":
			if seconds, ok := parseAge(value); ok {
				aged = now.Add(seconds)
			}
		case "domain":
			c.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimPrefix(value, ".")), ".")
		case "path":
			if c.Path = value; !strings.HasPrefix(value, "/") {
				c.Path = ""
			}
		case "secure":
			c.Secure = true
		case "httponly":
			c.HTTPOnly = true
		case "samesite":
			switch strings.ToLower(value) {
			case string(Strict):
				c.SameSite = Strict
			case string(None):
				c.SameSite = None
			default:
				c.SameSite = Lax
			}
		}
	}
	if c.Expires = expires; !aged.IsZero() {
		c.Expires = aged
	}
	if latest := now.Add(MaxAge); c.Expires.After(latest) {
		c.Expires = latest
	}
	return c, true
}

// parseAge reads a Max-Age: whole seconds, none or less ending the
// cookie at once.
func parseAge(value string) (time.Duration, bool) {
	digits := strings.TrimPrefix(value, "-")
	if digits == "" || strings.ContainsFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0, false
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	switch {
	case strings.HasPrefix(value, "-"), seconds == 0 && err == nil:
		return -time.Second, true
	case err != nil, seconds > int64(MaxAge/time.Second):
		return MaxAge, true
	}
	return time.Duration(seconds) * time.Second, true
}

//nolint:gochecknoglobals // the months a cookie's date names
var months = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}

// parseDate reads an Expires as browsers do: a time, a day, a month and
// a year in any order, between any separators.
func parseDate(value string) (time.Time, bool) {
	var hour, minute, second, day, month, year int
	var timed, dayed, monthed, yeared bool
	for _, token := range strings.FieldsFunc(value, dateDelimiter) {
		if !timed {
			if h, m, s, ok := clock(token); ok {
				hour, minute, second, timed = h, m, s, true
				continue
			}
		}
		number, digits := leading(token)
		switch {
		case !dayed && digits >= 1 && digits <= 2 && digits == len(token):
			day, dayed = number, true
		case !monthed && len(token) >= 3 && monthOf(token) > 0:
			month, monthed = monthOf(token), true
		case !yeared && digits >= 2 && digits <= 4 && digits == len(token):
			year, yeared = number, true
		}
	}
	year = fullYear(year)
	if !timed || !dayed || !monthed || !yeared || day < 1 || day > 31 || year < 1601 || hour > 23 || minute > 59 ||
		second > 59 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC), true
}

// dateDelimiter reports whether r stands between the parts of a date.
func dateDelimiter(r rune) bool {
	return r == '\t' || (r >= ' ' && r <= '/') || (r >= ';' && r <= '@') || (r >= '[' && r <= '`') ||
		(r >= '{' && r <= '~')
}

// fullYear is a year of two digits in its century: 70 to 99 in the
// last, 0 to 69 in this one.
func fullYear(year int) int {
	switch {
	case year >= 70 && year <= 99:
		return year + 1900
	case year >= 0 && year <= 69:
		return year + 2000
	}
	return year
}

func monthOf(token string) int {
	for i, name := range months {
		if strings.EqualFold(token[:3], name) {
			return i + 1
		}
	}
	return 0
}

// leading is the number token starts with and how many digits it has.
func leading(token string) (int, int) {
	digits := 0
	for digits < len(token) && token[digits] >= '0' && token[digits] <= '9' {
		digits++
	}
	number, _ := strconv.Atoi(token[:min(digits, 9)])
	return number, digits
}

// clock reads a time of day, 10:20:30.
func clock(token string) (int, int, int, bool) {
	parts := strings.SplitN(token, ":", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var read [3]int
	for i, part := range parts {
		number, digits := leading(part)
		if digits < 1 || digits > 2 || (i < 2 && digits != len(part)) {
			return 0, 0, 0, false
		}
		read[i] = number
	}
	return read[0], read[1], read[2], true
}
