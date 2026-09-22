//go:build ignore

// Regenerate the PNG icons in this directory. Run by hand when the mark changes:
//
//	go run internal/webui/assets/icons/generate.go internal/webui/assets/icons
//
// Rasterises the panel's ECK-Tunnel mark (a tunnel arch over a ground line) to
// PNG. The mark also exists as an SVG (internal/webui/handlers_app.go) and that
// is what desktop browsers use; iOS and Android's maskable icons want bitmaps,
// so the same geometry is drawn here as signed distance fields. The numbers
// below are the SVG's own path data.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const sw = 4.0 // half of the SVG's stroke-width:8

type arch struct{ x0, x1, top, bottom float64 } // legs at x0/x1, arc starts at top

var (
	arches = []arch{{30, 98, 64, 100}, {50, 78, 70, 100}}
	ground = [2][2]float64{{22, 100}, {106, 100}}
	gradA  = [3]float64{0xFF, 0x45, 0x3A}
	gradB  = [3]float64{0xA1, 0x22, 0x19}
)

func segment(px, py, ax, ay, bx, by float64) float64 {
	vx, vy, wx, wy := bx-ax, by-ay, px-ax, py-ay
	d := vx*vx + vy*vy
	t := 0.0
	if d != 0 {
		t = math.Max(0, math.Min(1, (wx*vx+wy*vy)/d))
	}
	return math.Hypot(wx-t*vx, wy-t*vy)
}

// archDistance is the distance to the centre line of one arch: two legs and
// the upper half-circle joining them.
func archDistance(px, py float64, a arch) float64 {
	cx, r := (a.x0+a.x1)/2, (a.x1-a.x0)/2
	d := math.Min(segment(px, py, a.x0, a.top, a.x0, a.bottom), segment(px, py, a.x1, a.top, a.x1, a.bottom))
	if py <= a.top {
		d = math.Min(d, math.Abs(math.Hypot(px-cx, py-a.top)-r))
	}
	return d
}

func glyphDistance(x, y float64) float64 {
	d := segment(x, y, ground[0][0], ground[0][1], ground[1][0], ground[1][1])
	for _, a := range arches {
		d = math.Min(d, archDistance(x, y, a))
	}
	return d - sw
}

func roundBox(px, py, r float64) float64 {
	qx, qy := math.Abs(px-64)-64+r, math.Abs(py-64)-64+r
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

func cover(d, pxPerUnit float64) float64 { return math.Max(0, math.Min(1, 0.5-d*pxPerUnit)) }

// render draws one icon. inset shrinks the artwork to leave a maskable safe zone.
func render(size int, inset float64, rounded bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale, art := float64(size)/128, 1-2*inset
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			x := ((float64(px)+0.5)/float64(size) - inset) / art * 128
			y := ((float64(py)+0.5)/float64(size) - inset) / art * 128
			bg := 1.0
			if rounded {
				bg = cover(roundBox(x, y, 30), scale*art)
			}
			if bg <= 0 {
				continue
			}
			t := math.Max(0, math.Min(1, (x+y)/256)) // the SVG's diagonal gradient
			ink := cover(glyphDistance(x, y), scale*art)
			var c [3]uint8
			for i := range c {
				v := gradA[i] + (gradB[i]-gradA[i])*t
				c[i] = uint8(v + (255-v)*ink + 0.5)
			}
			img.SetNRGBA(px, py, color.NRGBA{c[0], c[1], c[2], uint8(bg*255 + 0.5)})
		}
	}
	return img
}

func main() {
	out := os.Args[1]
	for _, ic := range []struct {
		name    string
		size    int
		inset   float64
		rounded bool
	}{
		{"icon-192.png", 192, 0, true},
		{"icon-512.png", 512, 0, true},
		// Maskable: Android may crop to a circle, so the mark is pulled inside
		// the 80% safe zone and the background runs edge to edge.
		{"icon-maskable-512.png", 512, 0.14, false},
		// iOS composites its own rounding and shows no transparency.
		{"apple-touch-icon.png", 180, 0, false},
	} {
		f, err := os.Create(filepath.Join(out, ic.name))
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, render(ic.size, ic.inset, ic.rounded)); err != nil {
			panic(err)
		}
		f.Close()
	}
}
