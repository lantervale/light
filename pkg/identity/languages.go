package identity

import (
	"strconv"
	"strings"
)

// Languages is the languages a person prefers as browsers list them,
// in navigator.languages and in Accept-Language: each tag in its
// order, and the language alone after the last tag of it, unless it is
// listed. A list that names none is American English.
func Languages(preferred []string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, tag := range preferred {
		if tag = canonical(tag); tag != "" && !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return []string{"en-US", "en"}
	}
	var out []string
	for i, tag := range tags {
		out = append(out, tag)
		base, _, regional := strings.Cut(tag, "-")
		if !regional || seen[base] {
			continue
		}
		last := true
		for _, later := range tags[i+1:] {
			if strings.HasPrefix(later, base+"-") {
				last = false
			}
		}
		if last {
			out = append(out, base)
		}
	}
	return out
}

// canonical is a language tag as it is written, "" for what is none:
// fr-FR for fr_fr, zh-Hans-CN for zh-hans-cn.
func canonical(tag string) string {
	parts := strings.Split(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"), "-")
	for i, part := range parts {
		if part == "" || len(part) > 8 || strings.ContainsFunc(part, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
		}) {
			return ""
		}
		switch {
		case i == 0:
			parts[i] = strings.ToLower(part)
		case len(part) == 2:
			parts[i] = strings.ToUpper(part)
		case len(part) == 4:
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		default:
			parts[i] = strings.ToLower(part)
		}
	}
	return strings.Join(parts, "-")
}

// AcceptLanguage is an Accept-Language header of the languages listed:
// the first as it is, each after it a tenth less wanted, down to a
// tenth.
func AcceptLanguage(list []string) string {
	var b strings.Builder
	for i, tag := range list {
		if i == 0 {
			b.WriteString(tag)
			continue
		}
		b.WriteString("," + tag + ";q=0." + strconv.Itoa(max(10-i, 1)))
	}
	return b.String()
}

// Tags are the language tags a text lists, in its order: a list as a
// person types it or as an Accept-Language header has it ("fr-CH,
// fr;q=0.9"), and the one macOS prints of a user's languages.
func Tags(text string) []string {
	var tags []string
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '\n' || r == '(' || r == ')' || r == ' ' || r == '\t'
	}) {
		field, _, _ = strings.Cut(strings.Trim(field, `"'`), ";")
		if tag := canonical(field); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// Environment is the languages a POSIX environment names, by getenv:
// those of LANGUAGE when a locale is set, else the locale of LC_ALL,
// LC_MESSAGES or LANG. The C locale names none.
func Environment(getenv func(string) string) []string {
	locale := ""
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale = getenv(name); locale != "" {
			break
		}
	}
	tag := localeTag(locale)
	if tag == "" {
		return nil
	}
	var tags []string
	for _, entry := range strings.Split(getenv("LANGUAGE"), ":") {
		if listed := localeTag(entry); listed != "" {
			tags = append(tags, listed)
		}
	}
	if len(tags) == 0 {
		return []string{tag}
	}
	return tags
}

// localeTag is the language tag of a POSIX locale, "" for C and POSIX:
// fr-FR for fr_FR.UTF-8@euro.
func localeTag(locale string) string {
	locale, _, _ = strings.Cut(locale, ".")
	locale, _, _ = strings.Cut(locale, "@")
	if locale == "C" || locale == "POSIX" {
		return ""
	}
	return canonical(locale)
}
