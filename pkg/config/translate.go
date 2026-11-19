package config

import (
	"slices"
	"strings"
)

// PageRule is what light does with the pages written in a language its
// reader does not read, while the model translates.
type PageRule int

const (
	// AskToTranslate offers the translation, AlwaysTranslate makes it
	// as the page shows and NeverTranslate offers nothing.
	AskToTranslate PageRule = iota
	AlwaysTranslate
	NeverTranslate
)

// Language is the language a tag names, its first part in lower case:
// "fr" for "fr-CA"; "" for what names none.
func Language(tag string) string {
	base, _, _ := strings.Cut(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(tag)), "_", "-"), "-")
	if len(base) < 2 || len(base) > 3 || strings.ContainsFunc(base, func(r rune) bool { return r < 'a' || r > 'z' }) {
		return ""
	}
	return base
}

// languages are the languages a setting lists, each once.
func languages(listed string) []string {
	var out []string
	for _, tag := range strings.FieldsFunc(listed, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		if l := Language(tag); l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// validTranslate is c with the languages always and never translated
// each once, by its language: one that is in both is never translated.
func (c Config) validTranslate() Config {
	never := languages(c.TranslateNever)
	always := slices.DeleteFunc(languages(c.TranslateAlways), func(l string) bool { return slices.Contains(never, l) })
	c.TranslateAlways, c.TranslateNever = strings.Join(always, ", "), strings.Join(never, ", ")
	return c
}

// PageRule is the rule for the pages written in the language of tag.
func (c Config) PageRule(tag string) PageRule {
	switch l := Language(tag); {
	case l == "":
	case slices.Contains(languages(c.TranslateNever), l):
		return NeverTranslate
	case slices.Contains(languages(c.TranslateAlways), l):
		return AlwaysTranslate
	}
	return AskToTranslate
}

// WithPageRule is c with rule for the pages written in the language of
// tag.
func (c Config) WithPageRule(tag string, rule PageRule) Config {
	l := Language(tag)
	if l == "" {
		return c
	}
	without := func(listed string) []string {
		return slices.DeleteFunc(languages(listed), func(kept string) bool { return kept == l })
	}
	always, never := without(c.TranslateAlways), without(c.TranslateNever)
	switch rule {
	case AlwaysTranslate:
		always = append(always, l)
	case NeverTranslate:
		never = append(never, l)
	case AskToTranslate:
	}
	c.TranslateAlways, c.TranslateNever = strings.Join(always, ", "), strings.Join(never, ", ")
	return c
}
