package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/input"

	"github.com/lantervale/light/pkg/webdriver"
)

// wheelNotch is how many pixels of a scroll action are one notch of
// the wheel light's window is given.
const wheelNotch = 40

// devices are the keys and buttons the driver holds down in the
// window, and where each pointer is in the session's document, in CSS
// pixels: what Release Actions lets go of.
type devices struct {
	keys     map[input.Key]bool
	buttons  map[input.MouseButton]bool
	pointers map[string]geom.Vec2
}

// special are the keys WebDriver names by a private character.
//
//nolint:gochecknoglobals // a constant table
var special = map[rune]input.Key{
	'': input.KeyBackspace, '': input.KeyTab, '': input.KeyEnter, '': input.KeyEnter,
	'': input.KeyLeftShift, '': input.KeyLeftControl, '': input.KeyLeftAlt,
	'': input.KeyPause, '': input.KeyEscape, '': input.KeyPageUp, '': input.KeyPageDown,
	'': input.KeyEnd, '': input.KeyHome, '': input.KeyLeft, '': input.KeyUp,
	'': input.KeyRight, '': input.KeyDown, '': input.KeyInsert, '': input.KeyDelete,
	'': input.KeyPad0, '': input.KeyPad1, '': input.KeyPad2, '': input.KeyPad3,
	'': input.KeyPad4, '': input.KeyPad5, '': input.KeyPad6, '': input.KeyPad7,
	'': input.KeyPad8, '': input.KeyPad9, '': input.KeyPadMultiply, '': input.KeyPadAdd,
	'': input.KeyPadSubtract, '': input.KeyPadDecimal, '': input.KeyPadDivide,
	'': input.KeyF1, '': input.KeyF2, '': input.KeyF3, '': input.KeyF4,
	'': input.KeyF5, '': input.KeyF6, '': input.KeyF7, '': input.KeyF8,
	'': input.KeyF9, '': input.KeyF10, '': input.KeyF11, '': input.KeyF12,
	'': input.KeyLeftSuper, '': input.KeyRightShift, '': input.KeyRightControl,
	'': input.KeyRightAlt, '': input.KeyRightSuper,
}

// typedBy are the characters the special keys that type one type.
//
//nolint:gochecknoglobals // a constant table
var typedBy = map[rune]rune{
	'': ' ', '': ';', '': '=', '': '0', '': '1', '': '2', '': '3',
	'': '4', '': '5', '': '6', '': '7', '': '8', '': '9', '': '*',
	'': '+', '': ',', '': '-', '': '.', '': '/',
}

// modifier reports whether a key is held with others and types
// nothing.
func modifier(k input.Key) bool {
	return k >= input.KeyLeftShift && k <= input.KeyRightSuper
}

// keyOf is the key of the keyboard that types r, false for a
// character no key of a US keyboard types alone.
func keyOf(r rune) (input.Key, bool) {
	if k, named := special[r]; named {
		return k, true
	}
	upper := unicode.ToUpper(r)
	if upper == ' ' || (upper >= '0' && upper <= '9') || (upper >= 'A' && upper <= 'Z') {
		return input.Key(upper), true
	}
	return 0, false
}

func (d *Driver) hold(in *input.State, k input.Key, down bool) {
	if d.keys.keys == nil {
		d.keys.keys = make(map[input.Key]bool)
	}
	in.SetKey(k, down)
	if down {
		d.keys.keys[k] = true
		return
	}
	delete(d.keys.keys, k)
}

func (d *Driver) press(in *input.State, b input.MouseButton, down bool) {
	if d.keys.buttons == nil {
		d.keys.buttons = make(map[input.MouseButton]bool)
	}
	in.SetMouseButton(b, down)
	if down {
		d.keys.buttons[b] = true
		return
	}
	delete(d.keys.buttons, b)
}

// release lets go of every key and button the driver holds, in the
// window of the session.
func (d *Driver) release() {
	if s, _ := d.screenOf(d.tab); s != nil {
		in := s.win.Input()
		for k := range d.keys.keys {
			in.SetKey(k, false)
		}
		for b := range d.keys.buttons {
			in.SetMouseButton(b, false)
		}
	}
	d.keys = devices{}
}

// listening has the scripts of the session's document hear the keys,
// as after a press in it.
func (d *Driver) listening(s *Screen, live *running) {
	set := s.session().frames[s.shownWeb()]
	for _, other := range s.scripted(set) {
		other.keyboard = false
	}
	live.keyboard = true
}

// pressed is where an element is pressed: the middle of its box, in
// the window.
type pressed struct {
	Rect      webdriver.Rect `json:"rect"`
	Displayed bool           `json:"displayed"`
	Enabled   bool           `json:"enabled"`
}

// aim scrolls an element into view, then finds where to press it; at
// is set by the step that ends the wait.
func (d *Driver) aim(f *flow, element string, at *geom.Vec2) {
	d.reading(f, func(_ *Screen, live *running) error {
		_, err := d.ask(live.rt, "aim", element)
		return err
	})
	f.frames(2)
	d.reading(f, func(s *Screen, live *running) error {
		value, err := d.ask(live.rt, "target", element)
		if err != nil {
			return err
		}
		var target pressed
		if err = json.Unmarshal(value, &target); err != nil {
			return err
		}
		if !target.Displayed || target.Rect.Width <= 0 || target.Rect.Height <= 0 {
			return webdriver.Errorf(webdriver.ElementNotInteractable, "the element is not shown")
		}
		middle := geom.V2(float32(target.Rect.X+target.Rect.Width/2), float32(target.Rect.Y+target.Rect.Height/2))
		if *at, err = d.inWindow(s, middle); err != nil {
			return webdriver.Errorf(webdriver.ElementNotInteractable, "the element is out of the page's view")
		}
		return nil
	})
}

// inWindow is a point of the session's document, in CSS pixels, as a
// point of its window; an error for one out of the page's view.
func (d *Driver) inWindow(s *Screen, at geom.Vec2) (geom.Vec2, error) {
	origin, zoom, err := d.origin(s)
	if err != nil {
		return geom.Vec2{}, err
	}
	point := origin.Add(at.Scale(zoom))
	if view, _ := s.pageView(); !view.Contains(point) {
		return point, webdriver.Errorf(webdriver.MoveTargetOutOfBounds, fmt.Sprintf("%v, %v is out of the page's view",
			at.X, at.Y))
	}
	return point, nil
}

// click adds a press and a release of the left button at a point of
// the window, each on a frame of its own as the window hears them.
func (d *Driver) click(f *flow, at *geom.Vec2) {
	with := func(do func(in *input.State)) func() error {
		return func() error {
			s, err := d.window()
			if err == nil {
				do(s.win.Input())
			}
			return err
		}
	}
	f.do(with(func(in *input.State) { in.SetMouse(*at) })).frames(1).
		do(with(func(in *input.State) { d.press(in, input.MouseLeft, true) })).frames(1).
		do(with(func(in *input.State) { d.press(in, input.MouseLeft, false) })).frames(2)
}

// ElementClick scrolls the element into view, clicks its middle
// through the window's input, and waits for the page the click led to.
func (d *Driver) ElementClick(ctx context.Context, element string, wait webdriver.Load) error {
	return d.flowing(ctx, func(f *flow) {
		var at geom.Vec2
		d.aim(f, element, &at)
		d.click(f, &at)
		f.do(func() error {
			if s, err := d.window(); err == nil {
				f.then(d.landing(s, d.tab, wait))
			}
			return nil
		})
	})
}

// ElementSendKeys gives the element the keyboard, the caret at its
// end, then types: text as the window's typed characters, the keys
// WebDriver names as presses, a modifier held until the null key or
// the end.
func (d *Driver) ElementSendKeys(ctx context.Context, element, text string) error {
	return d.flowing(ctx, func(f *flow) {
		var at geom.Vec2
		d.aim(f, element, &at)
		d.reading(f, func(_ *Screen, live *running) error {
			value, err := d.ask(live.rt, "focus", element)
			if err != nil {
				return err
			}
			var state struct {
				Focused bool `json:"focused"`
				Enabled bool `json:"enabled"`
			}
			if err = json.Unmarshal(value, &state); err != nil {
				return err
			}
			if !state.Enabled {
				return webdriver.Errorf(webdriver.ElementNotInteractable, "the element is disabled")
			}
			if !state.Focused {
				d.click(f, &at)
			}
			f.frames(1)
			d.reading(f, func(s *Screen, live *running) error {
				d.listening(s, live)
				d.typing(f, text)
				return nil
			})
			return nil
		})
	})
}

// selectAll is what Send Keys takes to select a field's whole text:
// the platform's command key held, A, and the keys let go.
func selectAll(system string) string {
	if command(system) == input.KeySuper {
		return "\uE03Da\uE000"
	}
	return "\uE009a\uE000"
}

// ElementClear empties a field as its user does, selecting its text
// and deleting it, so the page hears it change: a field holding the
// keyboard keeps its text from a script. What is editable and no field
// is emptied by the page's script.
func (d *Driver) ElementClear(ctx context.Context, element string) error {
	var filled bool
	if err := d.asking(ctx, &filled, "clear", element); err != nil || !filled {
		return err
	}
	return d.ElementSendKeys(ctx, element, selectAll(d.p.system)+"\uE003")
}

// typing adds the frames that type text.
func (d *Driver) typing(f *flow, text string) {
	in := func() *input.State {
		if s, _ := d.screenOf(d.tab); s != nil {
			return s.win.Input()
		}
		return &input.State{}
	}
	var held []input.Key
	chord := false
	tap := func(k input.Key) {
		f.do(func() error { d.hold(in(), k, true); return nil }).frames(1).
			do(func() error { d.hold(in(), k, false); return nil }).frames(1)
	}
	lift := func() {
		for _, k := range held {
			d.hold(in(), k, false)
		}
		held = nil
	}
	var plain []rune
	flush := func() {
		if len(plain) == 0 {
			return
		}
		typed := plain
		plain = nil
		f.do(func() error {
			for _, r := range typed {
				in().AddText(r)
			}
			return nil
		}).frames(1)
	}
	for _, r := range text {
		k, named := special[r]
		switch {
		case r == '':
			flush()
			chord = false
			f.do(func() error { lift(); return nil }).frames(1)
		case named && modifier(k):
			flush()
			chord = chord || (k != input.KeyLeftShift && k != input.KeyRightShift)
			f.do(func() error {
				d.hold(in(), k, true)
				held = append(held, k)
				return nil
			}).frames(1)
		case named:
			flush()
			tap(k)
		default:
			if typed, known := typedBy[r]; known {
				r = typed
			}
			if key, known := keyOf(r); chord && known {
				tap(key)
				continue
			}
			plain = append(plain, r)
		}
	}
	flush()
	f.do(func() error { lift(); return nil }).frames(2)
}

// PerformActions plays the sources tick by tick through the window's
// input: every action of a tick in one frame, then the frames of the
// tick's longest duration, one at least, so a press and its release
// are heard apart.
func (d *Driver) PerformActions(ctx context.Context, sources []webdriver.Source) error {
	return d.flowing(ctx, func(f *flow) {
		for _, tick := range webdriver.Ticks(sources) {
			var longest time.Duration
			for _, each := range tick {
				longest = max(longest, each.Action.Duration)
				d.acting(f, each)
			}
			var began time.Time
			f.do(func() error {
				began = time.Now()
				return nil
			}).frames(1).then(func() (bool, error) { return time.Since(began) >= longest, nil })
		}
		f.frames(1)
	})
}

// acting adds what one action of a tick does.
func (d *Driver) acting(f *flow, each webdriver.Step) {
	action := each.Action
	switch action.Type {
	case webdriver.ActionKeyDown, webdriver.ActionKeyUp:
		f.do(func() error { return d.key(action) })
	case webdriver.ActionPointerDown, webdriver.ActionPointerUp:
		f.do(func() error {
			s, err := d.window()
			if err != nil {
				return err
			}
			buttons := []input.MouseButton{input.MouseLeft, input.MouseMiddle, input.MouseRight}
			if action.Button < 0 || action.Button >= len(buttons) {
				return webdriver.Errorf(webdriver.UnsupportedOperation, "light's pointer has the buttons 0, 1 and 2")
			}
			d.press(s.win.Input(), buttons[action.Button], action.Type == webdriver.ActionPointerDown)
			return nil
		})
	case webdriver.ActionPointerMove, webdriver.ActionScroll:
		d.pointing(f, each)
	}
}

// key presses or lets go of the key an action names: a key of the
// keyboard when it is one, and the character it types on its way down.
func (d *Driver) key(action webdriver.Action) error {
	s, err := d.window()
	if err != nil {
		return err
	}
	in, down := s.win.Input(), action.Type == webdriver.ActionKeyDown
	r := []rune(action.Value)[0]
	if k, known := keyOf(r); known {
		d.hold(in, k, down)
	}
	if typed, known := typedBy[r]; known {
		r = typed
	}
	if _, named := special[r]; down && !named && unicode.IsPrint(r) {
		if live, failed := d.document(s); failed == nil {
			d.listening(s, live)
		}
		in.AddText(r)
	}
	return nil
}

// pointing adds a pointer's move, or a wheel's scroll at a point.
func (d *Driver) pointing(f *flow, each webdriver.Step) {
	action := each.Action
	move := func(s *Screen, middle geom.Vec2) error {
		if d.keys.pointers == nil {
			d.keys.pointers = map[string]geom.Vec2{}
		}
		at := geom.V2(float32(action.X), float32(action.Y)).Add(middle)
		if action.Origin == webdriver.OriginPointer {
			at = at.Add(d.keys.pointers[each.Source.ID])
		}
		point, err := d.inWindow(s, at)
		if err != nil {
			return err
		}
		in := s.win.Input()
		in.SetMouse(point)
		if action.Type == webdriver.ActionScroll {
			in.AddWheel(geom.V2(float32(-action.DeltaX/wheelNotch), float32(-action.DeltaY/wheelNotch)))
			return nil
		}
		d.keys.pointers[each.Source.ID] = at
		return nil
	}
	if action.Element == "" {
		f.do(func() error {
			s, err := d.window()
			if err != nil {
				return err
			}
			return move(s, geom.Vec2{})
		})
		return
	}
	d.reading(f, func(s *Screen, live *running) error {
		value, err := d.ask(live.rt, "rect", action.Element)
		if err != nil {
			return err
		}
		var box webdriver.Rect
		if err = json.Unmarshal(value, &box); err != nil {
			return err
		}
		return move(s, geom.V2(float32(box.X+box.Width/2), float32(box.Y+box.Height/2)))
	})
}

// ReleaseActions lets go of what Perform Actions left held.
func (d *Driver) ReleaseActions(ctx context.Context) error {
	return d.flowing(ctx, func(f *flow) {
		f.do(func() error {
			d.release()
			return nil
		}).frames(1)
	})
}
