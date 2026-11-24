package triage

import (
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Kind is the family of a cause.
type Kind string

// The kinds of causes.
const (
	MissingAPI   Kind = "missing-api"
	Exception    Kind = "exception"
	Assertion    Kind = "assertion"
	CSSComputed  Kind = "css-computed"
	CSSParsing   Kind = "css-parsing"
	HarnessError Kind = "harness-error"
	Timeout      Kind = "timeout"
	Crash        Kind = "crash"
	Reftest      Kind = "reftest"
	Other        Kind = "other"
)

// Cause is why a test or a subtest failed. Scope is the WPT directory a cause is local to: an assertion, a timeout or
// a reference mismatch says something of its directory alone, while a missing API is missing everywhere.
type Cause struct {
	Kind  Kind
	Key   string
	Scope string
}

// Signature is the cause as one line, its scope in brackets.
func (c Cause) Signature() string {
	if c.Scope == "" {
		return c.Key
	}
	return c.Key + " [" + c.Scope + "]"
}

const maxKey = 100

var (
	notFunction    = regexp.MustCompile(`(\S+) is not a function`)
	notDefined     = regexp.MustCompile(`(\S+) is not defined`)
	notConstructor = regexp.MustCompile(`(\S+) is not a constructor`)
	notIterable    = regexp.MustCompile(`(\S+) is not iterable`)
	readingOf      = regexp.MustCompile(`Cannot (read|set) properties of (undefined|null) ` +
		`\((?:reading|setting) '([^']*)'\)`)
	rejection     = regexp.MustCompile(`Unhandled rejection with value: object "(.*)"$`)
	errorName     = regexp.MustCompile(`^(?:Uncaught )?(?:[A-Z]\w*Error|DOMException|Error)(?: \[\w+\])?: `)
	assertPrefix  = regexp.MustCompile(`^(assert_\w+|promise_rejects_\w+): ?`)
	assertOutcome = regexp.MustCompile(`(?:^|\s)(?:expected |got disallowed|lengths differ|property \d|` +
		`Reached unreachable|value .* not in array|threw |function "|did not throw)`)
	threwObject   = regexp.MustCompile(`threw object "([^"]*)"`)
	ownProperty   = regexp.MustCompile(`property "([^"]+)" (?:missing|not found)`)
	idlMember     = regexp.MustCompile(`^(\w+) interface: (?:operation |attribute |constant |member )?(\w+)`)
	idlMissing    = regexp.MustCompile(`must have|not found|missing|does not have`)
	cssComputed   = regexp.MustCompile(`^Property (\S+) value '`)
	cssSet        = regexp.MustCompile(`^e\.style\['([^']+)'\] = .* should (not )?set the property value`)
	number        = regexp.MustCompile(`\d+(?:\.\d+)?`)
	space         = regexp.MustCompile(`\s+`)
	gotUndefined  = regexp.MustCompile(`got undefined$`)
	gotEmptyValue = regexp.MustCompile(`but got ""$`)
)

// ClassifySubtest names the cause of a subtest that did not pass, in the test at path test.
func ClassifySubtest(test string, s Subtest) Cause {
	scope := Scope(test)
	switch s.Status {
	case "TIMEOUT":
		return Cause{Kind: Timeout, Key: "subtest timeout", Scope: scope}
	case "NOTRUN":
		return Cause{Kind: Other, Key: "not run", Scope: scope}
	}
	if s.Message == "" {
		return Cause{Kind: Other, Key: "failed without a message", Scope: scope}
	}
	return classifyMessage(test, s.Name, s.Message)
}

// ClassifyTest names the cause carried by a test's own status, and false when the test's status is no failure.
func ClassifyTest(r *TestResult) (Cause, bool) {
	scope := Scope(r.Test)
	switch r.Status {
	case "TIMEOUT":
		return Cause{Kind: Timeout, Key: "test timeout", Scope: scope}, true
	case "CRASH":
		return Cause{Kind: Crash, Key: "crash", Scope: scope}, true
	case "FAIL":
		if len(r.Subtests) == 0 {
			return Cause{Kind: Reftest, Key: "reference mismatch", Scope: scope}, true
		}
		return Cause{Kind: Other, Key: "test failed", Scope: scope}, true
	case "ERROR", "PRECONDITION_FAILED", "INTERNAL-ERROR", "EXTERNAL-TIMEOUT":
		if c, ok := exception(r.Message); ok {
			return c, true
		}
		key := "harness error"
		if m := normalise(r.Message); m != "" {
			key += ": " + m
		}
		return Cause{Kind: HarnessError, Key: clip(key), Scope: scope}, true
	}
	return Cause{}, false
}

func classifyMessage(test, name, message string) Cause {
	if c, ok := exception(message); ok {
		return c
	}
	if c, ok := cssCause(test, name, message); ok {
		return c
	}
	if c, ok := missingProperty(name, message); ok {
		return c
	}
	m := assertPrefix.FindStringSubmatch(message)
	if m == nil {
		return Cause{Kind: Exception, Key: clip(normalise(unwrap(message))), Scope: Scope(test)}
	}
	return assertion(test, m[1], message[len(m[0]):])
}

func unwrap(message string) string {
	if m := rejection.FindStringSubmatch(message); m != nil {
		message = m[1]
	}
	return errorName.ReplaceAllString(message, "")
}

// exception recognises the messages V8 gives the exceptions a missing API throws, alone or wrapped in a rejection or
// an assertion that caught them.
func exception(message string) (Cause, bool) {
	if m := threwObject.FindStringSubmatch(message); m != nil && assertPrefix.MatchString(message) {
		message = m[1]
	}
	message = unwrap(message)
	if assertPrefix.MatchString(message) {
		return Cause{}, false
	}
	if m := notFunction.FindStringSubmatch(message); m != nil {
		return Cause{Kind: MissingAPI, Key: "not a function: " + last(m[1])}, true
	}
	if m := notDefined.FindStringSubmatch(message); m != nil {
		return Cause{Kind: MissingAPI, Key: "not defined: " + last(m[1])}, true
	}
	if m := notConstructor.FindStringSubmatch(message); m != nil {
		return Cause{Kind: MissingAPI, Key: "not a constructor: " + last(m[1])}, true
	}
	if m := notIterable.FindStringSubmatch(message); m != nil {
		return Cause{Kind: Exception, Key: "not iterable: " + last(m[1])}, true
	}
	if m := readingOf.FindStringSubmatch(message); m != nil {
		verb := map[string]string{"read": "reading", "set": "setting"}[m[1]]
		return Cause{Kind: Exception, Key: verb + " '" + m[3] + "' of " + m[2]}, true
	}
	return Cause{}, false
}

func last(expr string) string {
	expr = strings.TrimRight(expr, "()")
	if i := strings.LastIndex(expr, "."); i >= 0 && i < len(expr)-1 {
		expr = expr[i+1:]
	}
	return expr
}

func cssCause(test, name, message string) (Cause, bool) {
	if !strings.HasPrefix(test, "/css/") {
		return Cause{}, false
	}
	scope := Scope(test)
	if m := cssComputed.FindStringSubmatch(name); m != nil {
		if gotEmptyValue.MatchString(message) {
			return Cause{Kind: CSSComputed, Key: "computed value unsupported: " + m[1], Scope: scope}, true
		}
		return Cause{Kind: CSSComputed, Key: "computed value: " + m[1], Scope: scope}, true
	}
	if m := cssSet.FindStringSubmatch(name); m != nil {
		if m[2] != "" {
			return Cause{Kind: CSSParsing, Key: "invalid value accepted: " + m[1], Scope: scope}, true
		}
		return Cause{Kind: CSSParsing, Key: "value not parsed: " + m[1], Scope: scope}, true
	}
	return Cause{}, false
}

func missingProperty(name, message string) (Cause, bool) {
	if m := idlMember.FindStringSubmatch(name); m != nil &&
		strings.Contains(message, `"`+m[2]+`"`) && idlMissing.MatchString(message) {
		return Cause{Kind: MissingAPI, Key: "missing IDL member " + m[1] + "." + m[2]}, true
	}
	if m := ownProperty.FindStringSubmatch(message); m != nil {
		return Cause{Kind: MissingAPI, Key: "missing property " + m[1]}, true
	}
	return Cause{}, false
}

func assertion(test, name, rest string) Cause {
	scope := Scope(test)
	description, outcome := rest, ""
	if loc := assertOutcome.FindStringIndex(rest); loc != nil {
		description, outcome = rest[:loc[0]], rest[loc[0]:]
	}
	key := name
	if d := normalise(description); d != "" {
		key += ": " + d
	}
	switch {
	case strings.Contains(outcome, "did not throw"):
		key += " → did not throw"
	case gotUndefined.MatchString(outcome):
		if !strings.Contains(key, ":") {
			key += ":"
		}
		key += " → undefined"
	}
	return Cause{Kind: Assertion, Key: clip(key), Scope: scope}
}

func normalise(s string) string {
	s = elideQuotes(s)
	s = number.ReplaceAllString(s, "N")
	return strings.TrimSpace(space.ReplaceAllString(s, " "))
}

func clip(s string) string {
	if utf8.RuneCountInString(s) <= maxKey {
		return s
	}
	return string([]rune(s)[:maxKey-1]) + "…"
}

// Scope is the directory a test is counted under: its first two folders, three under html (/html/semantics/forms).
func Scope(test string) string {
	dir := path.Dir(strings.SplitN(test, "?", 2)[0])
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	depth := 2
	if parts[0] == "html" {
		depth = 3
	}
	if len(parts) > depth {
		parts = parts[:depth]
	}
	return "/" + strings.Join(parts, "/")
}

// elideQuotes replaces what quotes hold with an ellipsis. A quote closes where the same quote mark is followed by the
// end, a space or punctuation, so a quoted value holding the other mark, or its own, stays whole.
func elideQuotes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		q := s[i]
		if q != '"' && q != '\'' {
			b.WriteByte(q)
			continue
		}
		end := -1
		if i == 0 || !isWord(s[i-1]) {
			end = closing(s, i)
		}
		if end < 0 {
			b.WriteByte(q)
			continue
		}
		b.WriteByte(q)
		b.WriteString("…")
		b.WriteByte(q)
		i = end
	}
	return b.String()
}

func closing(s string, open int) int {
	q := s[open]
	for j := open + 1; j < len(s); j++ {
		if s[j] == q && (j+1 == len(s) || strings.IndexByte(" \t\n:,;.)]}", s[j+1]) >= 0) {
			return j
		}
	}
	return -1
}

func isWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
