//go:build v8 && cgo && (darwin || linux) && (amd64 || arm64)

package app //nolint:testpackage // the driver is checked against the screens it drives: their tabs, their windows

import (
	"encoding/json"
	"testing"

	"github.com/lantervale/light/pkg/webdriver"
)

func TestElementsAreFoundByXPath(t *testing.T) {
	d, _ := drivenAt(t, "/")
	locator := func(value string) string {
		return string(mustJSON(t, map[string]string{"using": "xpath", "value": value}))
	}
	one := func(path, value string) string {
		var found map[string]string
		_ = json.Unmarshal([]byte(d.ok("POST", path, locator(value))), &found)
		return found[webdriver.ElementKey]
	}
	for value, want := range map[string]int{
		`//button`:                                         2,
		`//BUTTON[@disabled]`:                              1,
		`//input[@type = "checkbox"]`:                      1,
		`//a[text() = "the next page"]`:                    1,
		`//p[contains(@class, "shown")]`:                   1,
		`(//button | //input)[position() > 1]`:             3,
		`//article`:                                        0,
		`/html/body/*[self::a or @data-kind][not(@style)]`: 2,
	} {
		var found []map[string]string
		_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/elements", locator(value))), &found)
		if len(found) != want {
			t.Errorf("%s finds %d elements, want %d", value, len(found), want)
		}
	}
	var ordered []map[string]string
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/elements", locator(`//input | //button[1] | //p[1]`))), &ordered)
	want := []string{d.find("#count"), d.find("#more"), d.find("#field"), d.find("#box")}
	if len(ordered) != len(want) {
		t.Fatalf("a union finds %d elements, want %d", len(ordered), len(want))
	}
	for i, found := range ordered {
		if found[webdriver.ElementKey] != want[i] {
			t.Errorf("the element %d of a union is not the document's", i)
		}
	}
	if got := one("/{s}/element", `//p[@data-kind = "counter"]`); got != d.find("#count") {
		t.Errorf("an element found by XPath is %q, by CSS %q", got, d.find("#count"))
	}
	if got := one("/{s}/element", `//button`); got != d.find("#more") {
		t.Errorf("the first of the elements an XPath selects is %q, want %q", got, d.find("#more"))
	}
	body := d.find("body")
	if got := one("/{s}/element/"+body+"/element", `a[@id = "next"]`); got != d.find("#next") {
		t.Errorf("an element found from another by a relative path is %q", got)
	}
	if got := one("/{s}/element/"+d.find("#count")+"/element", `following-sibling::input[2]`); got != d.find("#box") {
		t.Errorf("an element found along an axis of another is %q", got)
	}
	if got := one("/{s}/element/"+d.find("#count")+"/element", `..`); got != body {
		t.Errorf("the parent of an element is %q, want %q", got, body)
	}
	var under []map[string]string
	_ = json.Unmarshal([]byte(d.ok("POST", "/{s}/element/"+d.find("#count")+"/elements", locator(`.//button`))), &under)
	if len(under) != 0 {
		t.Errorf("a paragraph has %d buttons under it", len(under))
	}
	d.fails(webdriver.NoSuchElement, "POST", "/{s}/element", locator(`//article`))
	d.fails(webdriver.NoSuchElement, "POST", "/{s}/element/"+d.find("#count")+"/element", locator(`button`))
	for _, value := range []string{
		`//p[`, ``, `//p[$n]`, `nope()`, `//x:p`, `//p/@id`, `//p/text()`, `/`, `count(//p)`, `string(//p)`, `//p = 1`,
	} {
		d.fails(webdriver.InvalidSelector, "POST", "/{s}/element", locator(value))
		d.fails(webdriver.InvalidSelector, "POST", "/{s}/elements", locator(value))
		d.fails(webdriver.InvalidSelector, "POST", "/{s}/element/"+body+"/elements", locator(value))
	}
	d.ok("POST", "/{s}/timeouts", `{"implicit":2000}`)
	if got := one("/{s}/element", `//p[. = "late"]`); got != d.find("#late") {
		t.Errorf("an element that comes late is %q", got)
	}
}
