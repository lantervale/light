package webdriver

import (
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"
)

// The types of input sources, and of their actions.
const (
	SourceNone    = "none"
	SourceKey     = "key"
	SourcePointer = "pointer"
	SourceWheel   = "wheel"

	ActionPause         = "pause"
	ActionKeyDown       = "keyDown"
	ActionKeyUp         = "keyUp"
	ActionPointerDown   = "pointerDown"
	ActionPointerUp     = "pointerUp"
	ActionPointerMove   = "pointerMove"
	ActionPointerCancel = "pointerCancel"
	ActionScroll        = "scroll"

	OriginViewport = "viewport"
	OriginPointer  = "pointer"
)

// Source is one input device of Perform Actions and what it does, an
// action a tick.
type Source struct {
	ID, Type string
	// Pointer is the kind of a pointer: "mouse", "pen" or "touch".
	Pointer string
	Actions []Action
}

// Action is what a source does in one tick.
type Action struct {
	Type     string
	Duration time.Duration
	// Value is the key of a keyDown or a keyUp, one character.
	Value string
	// Button is the button of a pointerDown or a pointerUp.
	Button int
	// X and Y are where a pointer moves or a wheel scrolls, from Origin:
	// OriginViewport, OriginPointer, or the middle of the element
	// Element names.
	X, Y    float64
	Origin  string
	Element string
	// DeltaX and DeltaY are how far a wheel scrolls.
	DeltaX, DeltaY float64
}

// Ticks are the actions of sources tick by tick: in each, the action
// of every source that has one left, by its place in sources.
func Ticks(sources []Source) [][]Step {
	var ticks [][]Step
	for i := 0; ; i++ {
		var tick []Step
		for _, source := range sources {
			if i < len(source.Actions) {
				tick = append(tick, Step{Source: source, Action: source.Actions[i]})
			}
		}
		if tick == nil {
			return ticks
		}
		ticks = append(ticks, tick)
	}
}

// Step is one source's action in a tick.
type Step struct {
	Source Source
	Action Action
}

// sources reads the actions Perform Actions was sent.
func sources(raw json.RawMessage) ([]Source, *Error) {
	var list []json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &list) != nil {
		return nil, Errorf(InvalidArgument, "actions must be a list")
	}
	var out []Source
	for _, each := range list {
		source, err := sourceOf(each)
		if err != nil {
			return nil, err
		}
		out = append(out, source)
	}
	return out, nil
}

func sourceOf(raw json.RawMessage) (Source, *Error) {
	fields, ok := object(raw)
	if !ok {
		return Source{}, Errorf(InvalidArgument, "an action sequence must be an object")
	}
	source := Source{Pointer: "mouse"}
	if source.ID, ok = text(fields["id"]); !ok {
		return source, Errorf(InvalidArgument, "an action sequence must have an id that is a string")
	}
	source.Type, _ = text(fields["type"])
	switch source.Type {
	case SourceNone, SourceKey, SourceWheel:
	case SourcePointer:
		if given, set := fields["parameters"]; set && !isNull(given) {
			parameters, isObject := object(given)
			if !isObject {
				return source, Errorf(InvalidArgument, "the parameters of a pointer must be an object")
			}
			if kind, named := parameters["pointerType"]; named && !isNull(kind) {
				source.Pointer, _ = text(kind)
			}
		}
		if source.Pointer != "mouse" && source.Pointer != "pen" && source.Pointer != "touch" {
			return source, Errorf(InvalidArgument, `a pointerType must be "mouse", "pen" or "touch"`)
		}
	default:
		return source, Errorf(InvalidArgument, `the type of an action sequence must be "none", "key", "pointer" `+
			`or "wheel"`)
	}
	var items []json.RawMessage
	if isNull(fields["actions"]) || json.Unmarshal(fields["actions"], &items) != nil {
		return source, Errorf(InvalidArgument, "the actions of an action sequence must be a list")
	}
	for _, item := range items {
		action, err := actionOf(source.Type, item)
		if err != nil {
			return source, err
		}
		source.Actions = append(source.Actions, action)
	}
	return source, nil
}

// actionOf reads one action of a source of that type.
func actionOf(source string, raw json.RawMessage) (Action, *Error) {
	fields, ok := object(raw)
	if !ok {
		return Action{}, Errorf(InvalidArgument, "an action must be an object")
	}
	action := Action{Origin: OriginViewport}
	action.Type, _ = text(fields["type"])
	if value, given := fields["duration"]; given && !isNull(value) {
		ms, whole := integer(value)
		if !whole {
			return action, Errorf(InvalidArgument, "a duration must be a whole number of milliseconds")
		}
		action.Duration = milliseconds(ms)
	}
	var err *Error
	switch {
	case action.Type == ActionPause:
	case source == SourceKey && (action.Type == ActionKeyDown || action.Type == ActionKeyUp):
		if action.Value, ok = text(fields["value"]); !ok || utf8.RuneCountInString(action.Value) == 0 {
			err = Errorf(InvalidArgument, "the value of a key action must be a string of one character")
		}
	case source == SourcePointer && (action.Type == ActionPointerDown || action.Type == ActionPointerUp):
		button, whole := integer(fields["button"])
		if !whole {
			err = Errorf(InvalidArgument, "the button of a pointer action must be a whole number from 0")
		}
		action.Button = int(button)
	case source == SourcePointer && action.Type == ActionPointerCancel:
	case source == SourcePointer && action.Type == ActionPointerMove:
		err = action.place(fields, true)
	case source == SourceWheel && action.Type == ActionScroll:
		if err = action.place(fields, false); err == nil {
			var okX, okY bool
			action.DeltaX, okX = number(fields["deltaX"])
			action.DeltaY, okY = number(fields["deltaY"])
			if !okX || !okY {
				err = Errorf(InvalidArgument, "the deltaX and deltaY of a scroll must be numbers")
			}
		}
	default:
		err = Errorf(InvalidArgument, fmt.Sprintf("%q is no action of a %s source", action.Type, source))
	}
	return action, err
}

// place reads where an action is: its x and y, and what they are from.
func (a *Action) place(fields map[string]json.RawMessage, pointer bool) *Error {
	var okX, okY bool
	a.X, okX = number(fields["x"])
	a.Y, okY = number(fields["y"])
	if !okX || !okY {
		return Errorf(InvalidArgument, "the x and y of an action must be numbers")
	}
	origin, given := fields["origin"]
	if !given || isNull(origin) {
		return nil
	}
	if word, isText := text(origin); isText {
		if word != OriginViewport && (word != OriginPointer || !pointer) {
			return Errorf(InvalidArgument, fmt.Sprintf("%q is no origin of this action", word))
		}
		a.Origin = word
		return nil
	}
	element, ok := reference(origin, ElementKey)
	if !ok {
		return Errorf(InvalidArgument, `an origin must be "viewport", "pointer" or an element`)
	}
	a.Origin, a.Element = "", element
	return nil
}

// reference is the id raw refers to under key, false when raw is no
// such reference.
func reference(raw json.RawMessage, key string) (string, bool) {
	fields, ok := object(raw)
	if !ok {
		return "", false
	}
	return text(fields[key])
}
