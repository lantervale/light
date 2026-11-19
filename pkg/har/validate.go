package har

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// The kinds of value a field of the format holds.
const (
	text = iota
	number
	whole
	boolean
	moment
	object
	list
)

// field is one field of an object of the format: its kind, whether the
// format requires it, and for an object or a list of them, its own.
type field struct {
	name     string
	kind     int
	required bool
	of       []field
}

//nolint:gochecknoglobals // the format, as its schema writes it
var (
	note    = field{"comment", text, false, nil}
	name    = field{"name", text, true, nil}
	value   = field{"value", text, true, nil}
	creator = []field{name, {"version", text, true, nil}, note}
	pair    = []field{name, value, note}
	cookie  = []field{
		name, value, {"path", text, false, nil},
		{"domain", text, false, nil}, {"expires", moment, false, nil}, {"httpOnly", boolean, false, nil},
		{"secure", boolean, false, nil}, note,
	}
	schema = []field{{"log", object, true, []field{
		{"version", text, true, nil},
		{"creator", object, true, creator},
		{"browser", object, false, creator},
		note,
		{"pages", list, false, []field{
			{"startedDateTime", moment, true, nil}, {"id", text, true, nil}, {"title", text, true, nil},
			{"pageTimings", object, true, []field{
				{"onContentLoad", number, false, nil}, {"onLoad", number, false, nil}, note,
			}},
			note,
		}},
		{"entries", list, true, []field{
			{"pageref", text, false, nil}, {"startedDateTime", moment, true, nil}, {"time", number, true, nil},
			{"request", object, true, []field{
				{"method", text, true, nil}, {"url", text, true, nil}, {"httpVersion", text, true, nil},
				{"cookies", list, true, cookie}, {"headers", list, true, pair}, {"queryString", list, true, pair},
				{"postData", object, false, []field{
					{"mimeType", text, true, nil}, {"text", text, false, nil}, note,
					{"params", list, false, []field{
						name, {"value", text, false, nil}, {"fileName", text, false, nil},
						{"contentType", text, false, nil}, note,
					}},
				}},
				{"headersSize", whole, true, nil}, {"bodySize", whole, true, nil}, note,
			}},
			{"response", object, true, []field{
				{"status", whole, true, nil}, {"statusText", text, true, nil}, {"httpVersion", text, true, nil},
				{"cookies", list, true, cookie}, {"headers", list, true, pair},
				{"content", object, true, []field{
					{"size", whole, true, nil}, {"compression", whole, false, nil}, {"mimeType", text, true, nil},
					{"text", text, false, nil}, {"encoding", text, false, nil}, note,
				}},
				{"redirectURL", text, true, nil}, {"headersSize", whole, true, nil}, {"bodySize", whole, true, nil},
				note,
			}},
			{"cache", object, true, nil},
			{"timings", object, true, []field{
				{"blocked", number, false, nil}, {"dns", number, false, nil}, {"connect", number, false, nil},
				{"send", number, true, nil}, {"wait", number, true, nil}, {"receive", number, true, nil},
				{"ssl", number, false, nil}, note,
			}},
			{"serverIPAddress", text, false, nil}, {"connection", text, false, nil}, note,
		}},
	}}}
)

// Validate reports what in data is not a HAR 1.2 file as its schema
// has it: a required field that is missing, a value of another kind, a
// date that is not ISO 8601, a field that is not the format's and does
// not begin with "_". Beyond the schema it checks what importers rely
// on: the version, a page for each pageref, steps that are -1 or more
// and add up to an entry's time, and base64 that decodes.
func Validate(data []byte) error {
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("not JSON: %w", err)
	}
	var faults []error
	check("", file, schema, &faults)
	log, _ := file["log"].(map[string]any)
	if version, _ := log["version"].(string); version != Version {
		faults = append(faults, fmt.Errorf("log.version is %q, want %q", version, Version))
	}
	pages := map[string]bool{}
	for _, p := range objects(log["pages"]) {
		id, _ := p["id"].(string)
		pages[id] = true
	}
	for i, e := range objects(log["entries"]) {
		at := fmt.Sprintf("log.entries[%d]", i)
		if ref, named := e["pageref"].(string); named && !pages[ref] {
			faults = append(faults, fmt.Errorf("%s.pageref names no page: %q", at, ref))
		}
		timings, _ := e["timings"].(map[string]any)
		sum := 0.0
		for name, value := range timings {
			step, isNumber := value.(float64)
			if !isNumber || name[0] == '_' {
				continue
			}
			if step < -1 || (step < 0 && (name == "send" || name == "wait" || name == "receive")) {
				faults = append(faults, fmt.Errorf("%s.timings.%s is %v", at, name, step))
			}
			if name != "ssl" {
				sum += max(0, step)
			}
		}
		if took, _ := e["time"].(float64); math.Abs(took-sum) > 0.01 || took < 0 {
			faults = append(faults, fmt.Errorf("%s.time is %v, and its steps add up to %v", at, took, sum))
		}
		response, _ := e["response"].(map[string]any)
		content, _ := response["content"].(map[string]any)
		switch encoding, _ := content["encoding"].(string); encoding {
		case "":
		case inBase64:
			body, _ := content["text"].(string)
			if _, err := base64.StdEncoding.DecodeString(body); err != nil {
				faults = append(faults, fmt.Errorf("%s.response.content.text: %w", at, err))
			}
		default:
			faults = append(faults, fmt.Errorf("%s.response.content.encoding is %q", at, encoding))
		}
	}
	return errors.Join(faults...)
}

func objects(value any) []map[string]any {
	var out []map[string]any
	items, _ := value.([]any)
	for _, item := range items {
		if o, ok := item.(map[string]any); ok {
			out = append(out, o)
		}
	}
	return out
}

// check lists in faults what the object at path has that fields do not
// allow. An object given no fields may hold anything.
func check(path string, o map[string]any, fields []field, faults *[]error) {
	known := map[string]bool{}
	for _, f := range fields {
		known[f.name] = true
		at := path + f.name
		value, has := o[f.name]
		if !has {
			if f.required {
				*faults = append(*faults, fmt.Errorf("%s is missing", at))
			}
			continue
		}
		if fault := kindOf(at, value, f, faults); fault != "" {
			*faults = append(*faults, fmt.Errorf("%s is %v, want %s", at, value, fault))
		}
	}
	for name := range o {
		if fields != nil && !known[name] && name[0] != '_' {
			*faults = append(*faults, fmt.Errorf("%s%s is not a field of the format", path, name))
		}
	}
}

// kindOf is "" when value is of f's kind, else what it should be.
func kindOf(at string, value any, f field, faults *[]error) string {
	switch f.kind {
	case text:
		if _, ok := value.(string); !ok {
			return "a string"
		}
	case number, whole:
		n, ok := value.(float64)
		if !ok {
			return "a number"
		}
		if f.kind == whole && n != math.Trunc(n) {
			return "a whole number"
		}
	case boolean:
		if _, ok := value.(bool); !ok {
			return "a boolean"
		}
	case moment:
		s, _ := value.(string)
		if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			return "an ISO 8601 date"
		}
	case object:
		inner, ok := value.(map[string]any)
		if !ok {
			return "an object"
		}
		check(at+".", inner, f.of, faults)
	case list:
		items, ok := value.([]any)
		if !ok {
			return "an array"
		}
		for i, item := range items {
			inner, isObject := item.(map[string]any)
			if !isObject {
				return "an array of objects"
			}
			check(fmt.Sprintf("%s[%d].", at, i), inner, f.of, faults)
		}
	}
	return ""
}
