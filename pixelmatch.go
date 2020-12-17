package pixelmatch

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"math"
)

var ErrImageSizesNotMatch = errors.New("image sizes do not match")

type MatchOptions struct {
	threshold        float64
	includeAA        bool
	alpha            float64
	antiAliasedColor color.RGBA
	diffColor        color.RGBA
	diffColorAlt     *color.RGBA
	diffMask         bool
	writeTo          *image.Image
}

type MatchOption func(*MatchOptions)

func Threshold(threshold float64) MatchOption {
	return func(o *MatchOptions) {
		o.threshold = threshold
	}
}

func WriteTo(img *image.Image) MatchOption {
	return func(o *MatchOptions) {
		o.writeTo = img
	}
}

func IncludeAntiAlias(o *MatchOptions) {
	o.includeAA = true
}

func Alpha(alpha float64) MatchOption {
	return func(o *MatchOptions) {
		o.alpha = alpha
	}
}

func AntiAliasedColor(c color.Color) MatchOption {
	return func(o *MatchOptions) {
		o.antiAliasedColor = color.RGBAModel.Convert(c).(color.RGBA)
	}
}

func DiffColor(c color.Color) MatchOption {
	return func(o *MatchOptions) {
		o.diffColor = color.RGBAModel.Convert(c).(color.RGBA)
	}
}

func DiffColorAlt(c color.Color) MatchOption {
	return func(o *MatchOptions) {
		diffColorAlt := color.RGBAModel.Convert(c).(color.RGBA)
		o.diffColorAlt = &diffColorAlt
	}
}

func EnableDiffMask(o *MatchOptions) {
	o.diffMask = true
}

type rgba struct {
	R float64
	G float64
	B float64
	A float64
}

func rgbaFromColor(c color.Color) *rgba {
	const x = 1.0 / 256.0
	r, g, b, a := c.RGBA()
	return &rgba{
		R: float64(r) * x,
		G: float64(g) * x,
		B: float64(b) * x,
		A: float64(a) * x,
	}
}

func MatchPixel(a, b image.Image, opts ...MatchOption) (int, error) {
	options := MatchOptions{
		threshold:        0.1,
		alpha:            0.1,
		antiAliasedColor: color.RGBA{R: 255, G: 255},
		diffColor:        color.RGBA{R: 255},
	}
	for _, opt := range opts {
		opt(&options)
	}

	if !a.Bounds().Eq(b.Bounds()) {
		return 0, ErrImageSizesNotMatch
	}

	var out *image.RGBA
	if options.writeTo != nil {
		out = image.NewRGBA(a.Bounds())
	}

	if isIdentical(a, b) { // fast path if identical
		if out != nil && !options.diffMask {
			rect := a.Bounds()
			for y := rect.Min.Y; y < rect.Max.Y; y++ {
				for x := rect.Min.X; x < rect.Max.X; x++ {
					c := rgbaFromColor(a.At(x, y))
					v := uint8(blend(rgbaToY(c), options.alpha*c.A/255))
					out.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
				}
			}
		}
		return 0, nil
	}

	maxDelta := 35215 * options.threshold * options.threshold
	diff := 0

	rect := a.Bounds()
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			delta := colorDelta(a.At(x, y), b.At(x, y), false)
			if math.Abs(delta) > maxDelta {
				if !options.includeAA && (isAntiAliased(a, b, x, y) || isAntiAliased(b, a, x, y)) {
					if out != nil && !options.diffMask {
						c := options.antiAliasedColor
						c.A = 255
						out.SetRGBA(x, y, c)
					}
				} else {
					if out != nil {
						if delta < 0 && options.diffColorAlt != nil {
							c := *options.diffColorAlt
							c.A = 255
							out.SetRGBA(x, y, c)
						} else {
							c := options.diffColor
							c.A = 255
							out.SetRGBA(x, y, c)
						}
					}
					diff++
				}
			} else {
				if out != nil && !options.diffMask {
					c := rgbaFromColor(a.At(x, y))
					v := uint8(blend(rgbaToY(c), options.alpha*c.A/255))
					out.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
				}
			}
		}
	}

	if options.writeTo != nil {
		*options.writeTo = out
	}

	return diff, nil
}

func colorDelta(a, b color.Color, yOnly bool) float64 {
	ca := color.RGBAModel.Convert(a).(color.RGBA)
	cb := color.RGBAModel.Convert(b).(color.RGBA)
	if ca.A == cb.A && ca.R == cb.R && ca.G == cb.G && ca.B == cb.B {
		return 0
	}
	fa := rgbaFromColor(ca)
	fb := rgbaFromColor(cb)

	blendRGBA(fa)
	blendRGBA(fb)

	ya := rgbaToY(fa)
	yb := rgbaToY(fb)
	y := ya - yb
	if yOnly {
		return y
	}
	i := rgbaToI(fa) - rgbaToI(fb)
	q := rgbaToQ(fa) - rgbaToQ(fb)
	delta := 0.5053*y*y + 0.299*i*i + 0.1957*q*q
	if ya > yb {
		return -delta
	} else {
		return delta
	}
}

func blendRGBA(c *rgba) {
	if c.A < 255 {
		a := c.A / 255
		c.R = blend(c.R, a)
		c.G = blend(c.G, a)
		c.B = blend(c.B, a)
	}
}

func blend(c float64, a float64) float64 {
	return 255 + (c-255)*a
}

func rgbaToY(c *rgba) float64 {
	return c.R*0.29889531 + c.G*0.58662247 + c.B*0.11448223
}

func rgbaToI(c *rgba) float64 {
	return c.R*0.59597799 - c.G*0.27417610 - c.B*0.32180189
}

func rgbaToQ(c *rgba) float64 {
	return c.R*0.21147017 - c.G*0.52261711 + c.B*0.31114694
}

func isAntiAliased(a, b image.Image, x1, y1 int) bool {
	r := a.Bounds()
	x0 := maxInt(x1-1, r.Min.X)
	y0 := maxInt(y1-1, r.Min.Y)
	x2 := minInt(x1+1, r.Max.X-1)
	y2 := minInt(y1+1, r.Max.Y-1)
	zeroes := 0
	if x1 == x0 || x1 == x2 || y1 == y0 || y1 == y2 {
		zeroes = 1
	}

	min := 0.0
	max := 0.0
	var minX, minY, maxX, maxY int
	c := a.At(x1, y1)
	for x := x0; x <= x2; x++ {
		for y := y0; y <= y2; y++ {
			if x == x1 && y == y1 {
				continue
			}
			delta := colorDelta(c, a.At(x, y), true)

			switch {
			case delta == 0:
				zeroes++
				if zeroes > 2 {
					return false
				}
			case delta < min:
				min = delta
				minX = x
				minY = y
			case max < delta:
				max = delta
				maxX = x
				maxY = y
			}
		}
	}

	if max == 0 || min == 0 {
		return false
	}

	return (hasManySiblings(a, minX, minY) && hasManySiblings(b, minX, minY)) || (hasManySiblings(a, maxX, maxY) && hasManySiblings(b, maxX, maxY))
}

func hasManySiblings(img image.Image, x1, y1 int) bool {
	rect := img.Bounds()
	x0 := maxInt(x1-1, rect.Min.X)
	y0 := maxInt(y1-1, rect.Min.Y)
	x2 := minInt(x1+1, rect.Max.X-1)
	y2 := minInt(y1+1, rect.Max.Y-1)
	zeroes := 0
	if x1 == x0 || x1 == x2 || y1 == y0 || y1 == y2 {
		zeroes = 1
	}

	r, g, b, a := img.At(x1, y1).RGBA()
	for x := x0; x <= x2; x++ {
		for y := y0; y <= y2; y++ {
			if x == x1 && y == y1 {
				continue
			}

			nr, ng, nb, na := img.At(x, y).RGBA()
			if r == nr && g == ng && b == nb && a == na {
				zeroes++
			}
			if zeroes > 2 {
				return true
			}
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a < b {
		return b
	} else {
		return a
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func isIdentical(a, b image.Image) bool {
	switch x := a.(type) {
	case *image.RGBA:
		y, ok := b.(*image.RGBA)
		if ok && equals(x.Pix, y.Pix, x.Stride, y.Stride, x.Rect) {
			return true
		}
	case *image.RGBA64:
		y, ok := b.(*image.RGBA64)
		if ok && equals(x.Pix, y.Pix, x.Stride, y.Stride, x.Rect) {
			return true
		}
	case *image.NRGBA:
		y, ok := b.(*image.NRGBA)
		if ok && equals(x.Pix, y.Pix, x.Stride, y.Stride, x.Rect) {
			return true
		}
	case *image.NRGBA64:
		y, ok := b.(*image.NRGBA64)
		if ok && equals(x.Pix, y.Pix, x.Stride, y.Stride, x.Rect) {
			return true
		}
	case *image.Gray:
		y, ok := b.(*image.Gray)
		if ok && equals(x.Pix, y.Pix, x.Stride, y.Stride, x.Rect) {
			return true
		}
	}
	return false
}

func equals(pixA, pixB []uint8, strideA, strideB int, rect image.Rectangle) bool {
	w := rect.Dx()
	h := rect.Dy()
	if w*h == len(pixA) && w*h == len(pixB) { // both is not sub-image
		return bytes.Equal(pixA, pixB)
	}
	for y := 0; y < h; y++ {
		if !bytes.Equal(pixA[y*strideA:y*strideA+w], pixB[y*strideB:y*strideB+w]) {
			return false
		}
	}
	return true
}
