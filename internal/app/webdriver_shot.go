package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/lantervale/ember/geom"
	"github.com/lantervale/ember/platform"

	"github.com/lantervale/light/pkg/webdriver"
)

// captureWait is how long a window is given to draw the frame a
// screenshot is of.
const captureWait = 5 * time.Second

// shot is what a window showed: its pixels, and how many of them a
// point of the window is across.
type shot struct {
	picture image.Image
	scale   float64
}

func noCapture(why string) error {
	return webdriver.Errorf(webdriver.UnableToCaptureScreen, why)
}

// captured asks the engine for the next frame win presents, as a
// capture of a hidden run is taken (platform.Window's Capture), and
// reads the picture it saved once it is there. A window nothing draws,
// as in a World that is only stepped, has no picture to give.
func (d *Driver) captured(win *platform.Window, done func(shot, error)) step {
	var (
		dir, path string
		began     time.Time
	)
	finish := func(taken shot, err error) (bool, error) {
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
		done(taken, err)
		return true, err
	}
	return func() (bool, error) {
		if !win.Open() {
			return finish(shot{}, noCapture("light's window is drawn by nothing in this run: there is no picture "+
				"of it to take"))
		}
		if path == "" {
			var err error
			if dir, err = os.MkdirTemp("", "light-webdriver-"); err != nil {
				return finish(shot{}, noCapture(err.Error()))
			}
			path, began = filepath.Join(dir, "shot.png"), time.Now()
			win.Capture(path)
			return false, nil
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) || (err == nil && len(data) == 0) {
			if time.Since(began) > captureWait {
				return finish(shot{}, noCapture("the window drew no frame to take a picture of"))
			}
			return false, nil
		}
		if err != nil {
			return finish(shot{}, noCapture(err.Error()))
		}
		picture, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return finish(shot{}, noCapture(err.Error()))
		}
		scale := 1.0
		if size := win.Size(); size.X > 0 {
			scale = float64(picture.Bounds().Dx()) / float64(size.X)
		}
		return finish(shot{picture: picture, scale: scale}, nil)
	}
}

// cut is the part of a shot that area, in points of the window, covers,
// as a picture of width by height pixels: each pixel the mean of the
// window's pixels it stands for, so a page is pictured a pixel a CSS
// pixel whatever the display's scale and the page's zoom.
func cut(taken shot, area geom.Rect, width, height int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	bounds := taken.picture.Bounds()
	across, down := float64(area.W)*taken.scale/float64(width), float64(area.H)*taken.scale/float64(height)
	left, top := float64(area.X)*taken.scale, float64(area.Y)*taken.scale
	for y := range height {
		y0 := int(math.Round(top + float64(y)*down))
		y1 := max(int(math.Round(top+float64(y+1)*down)), y0+1)
		for x := range width {
			x0 := int(math.Round(left + float64(x)*across))
			x1 := max(int(math.Round(left+float64(x+1)*across)), x0+1)
			out.SetNRGBA(x, y, mean(taken.picture, image.Rect(x0, y0, x1, y1).Add(bounds.Min).Intersect(bounds)))
		}
	}
	return out
}

// within is the part of r that is in bounds.
func within(r, bounds geom.Rect) geom.Rect {
	left, top := max(r.X, bounds.X), max(r.Y, bounds.Y)
	right, bottom := min(r.X+r.W, bounds.X+bounds.W), min(r.Y+r.H, bounds.Y+bounds.H)
	return geom.Rect{X: left, Y: top, W: max(right-left, 0), H: max(bottom-top, 0)}
}

// mean is the mean colour of the pixels of area, opaque.
func mean(picture image.Image, area image.Rectangle) color.NRGBA {
	var r, g, b, n uint32
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			pr, pg, pb, _ := picture.At(x, y).RGBA()
			r, g, b, n = r+pr>>8, g+pg>>8, b+pb>>8, n+1
		}
	}
	if n == 0 {
		return color.NRGBA{A: math.MaxUint8}
	}
	part := func(sum uint32) uint8 { return uint8(min((sum+n/2)/n, math.MaxUint8)) }
	return color.NRGBA{R: part(r), G: part(g), B: part(b), A: math.MaxUint8}
}

// picturing takes a picture of the part of the page's view that area
// gives, in CSS pixels of the page, once the steps before it are done.
func (d *Driver) picturing(f *flow, into *[]byte, area func(s *Screen) (geom.Rect, error)) {
	f.do(func() error {
		s, err := d.window()
		if err != nil {
			return err
		}
		f.then(d.capture(s.win, func(taken shot, failed error) {
			if failed != nil {
				return
			}
			f.do(func() error {
				part, bad := area(s)
				if bad != nil {
					return bad
				}
				view, _ := s.pageView()
				zoom := float32(1)
				if page := s.shownWeb(); page != nil && page.Doc.Zoom() > 0 {
					zoom = page.Doc.Zoom()
				}
				part = within(part, view)
				width, height := int(math.Round(float64(part.W/zoom))), int(math.Round(float64(part.H/zoom)))
				if width <= 0 || height <= 0 {
					return noCapture("there is nothing of it in the page's view")
				}
				var encoded bytes.Buffer
				if bad = png.Encode(&encoded, cut(taken, part, width, height)); bad != nil {
					return noCapture(bad.Error())
				}
				*into = encoded.Bytes()
				return nil
			})
		}))
		return nil
	})
}

// Screenshot is the page's view as a PNG: what light draws under its
// tabs and its navigation row, a pixel a CSS pixel.
func (d *Driver) Screenshot(ctx context.Context) ([]byte, error) {
	var picture []byte
	err := d.flowing(ctx, func(f *flow) {
		d.picturing(f, &picture, func(s *Screen) (geom.Rect, error) {
			view, shown := s.pageView()
			if !shown {
				return view, noCapture(fmt.Sprintf("%s is no page of the web: light pictures the view of one",
					s.browser.Tab().Current()))
			}
			return view, nil
		})
	})
	return picture, err
}

// ElementScreenshot is the part of an element that is in the page's
// view, once scrolled to.
func (d *Driver) ElementScreenshot(ctx context.Context, element string) ([]byte, error) {
	var picture []byte
	err := d.flowing(ctx, func(f *flow) {
		var box webdriver.Rect
		d.reading(f, func(_ *Screen, live *running) error {
			_, err := d.ask(live.rt, "aim", element)
			return err
		})
		f.frames(2)
		d.reading(f, func(_ *Screen, live *running) error {
			value, err := d.ask(live.rt, "rect", element)
			if err != nil {
				return err
			}
			return json.Unmarshal(value, &box)
		})
		d.picturing(f, &picture, func(s *Screen) (geom.Rect, error) {
			origin, zoom, err := d.origin(s)
			return geom.Rect{X: origin.X + float32(box.X)*zoom, Y: origin.Y + float32(box.Y)*zoom,
				W: float32(box.Width) * zoom, H: float32(box.Height) * zoom}, err
		})
	})
	return picture, err
}
