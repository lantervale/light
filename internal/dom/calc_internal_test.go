package dom

import "testing"

func TestAComputedValueHasItsMathFunctionsWorkedOut(t *testing.T) {
	for _, c := range []struct{ name, value, want string }{
		{"opacity", "exp(0)", "1"},
		{"opacity", "calc(log(1) + 0.5)", "0.5"},
		{"opacity", "calc(exp(0) + log(1) + log(1))", "1"},
		{"opacity", "calc(log(0))", "0"},
		{"opacity", "calc(infinity)", "1"},
		{"opacity", "calc(1 / 3)", "0.333333"},
		{"opacity", "calc(pow(1,1) - 0.5)", "0.5"},
		{"opacity", "calc(hypot(1) * 0.5)", "0.5"},
		{"opacity", "round(1.1,1)", "1"},
		{"opacity", "round(down, 1.9, 1)", "1"},
		{"opacity", "mod(-1, 3)", "1"},
		{"opacity", "calc(rem(-1, 3) + 1)", "0"},
		{"opacity", "progress(1, 0, 2)", "0.5"},
		{"opacity", "progress(50px, 0px, 100px)", "0.5"},
		{"opacity", "sign(-3)", "0"},
		{"opacity", "min(0.2, exp(0))", "0.2"},
		{"opacity", "clamp(0.1, 0.05, 1)", "0.1"},
		{"opacity", "calc(sin(90deg) * cos(0))", "1"},
		{"z-index", "calc(2.4)", "2"},
		{"z-index", "round(up, 2.1)", "3"},
		{"flex-grow", "sqrt(4)", "2"},
		{"line-height", "pow(2, 2)", "4"},
		{"margin-left", "calc(1px * pow(2, 3))", "8px"},
		{"margin-left", "calc(10px / 4px * 1px)", "2.5px"},
		{"rotate", "calc(1turn / 4)", "90deg"},
		{"rotate", "atan2(1, 1)", "45deg"},
		{"transition-duration", "calc(500ms + 1s)", "1.5s"},
		{"transform", "scale(exp(0)) translate(calc(1px + 2px), 0px)", "scale(1) translate(3px, 0px)"},
		{"margin-left", "calc(1em + 1px)", "33px"},
		{"margin-left", "calc(50% + 1px)", "calc(50% + 1px)"},
		{"letter-spacing", "hypot(2px, 40%)", "hypot(2px, 40%)"},
		{"grid-template-columns", "minmax(10px, 1fr) repeat(2, 1fr)", "minmax(10px, 1fr) repeat(2, 1fr)"},
		{"width", "fit-content(min(10px, 20px))", "fit-content(10px)"},
		{"margin-left", "calc(1px / 0)", "calc(1px / 0)"},
		{"opacity", "progress(4em, 0px, 16rem)", "0.5"},
		{"margin-left", "calc(1em + 1ex)", "calc(1em + 1ex)"},
	} {
		if got := worked(c.name, c.value, fontSizes{em: 32, rem: 16}); got != c.want {
			t.Errorf("%s: %s is %q, want %q", c.name, c.value, got, c.want)
		}
	}
}
