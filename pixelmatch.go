package pixelmatch

import (
	"errors"
	"image"
	"image/color"
)

var ErrImageSizesNotMatch = errors.New("image sizes do not match")

type MatchOptions struct {
	threshold float64
	includeAA bool
	writeTo   *image.Image
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

func MatchPixel(a, b image.Image, opts ...MatchOption) (int, error) {
	options := MatchOptions{
		threshold: 0.1,
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

	maxDelta := 35215 * options.threshold * options.threshold
	diff := 0

	rect := a.Bounds()
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			delta := colorDelta(a.At(x, y), b.At(x, y), false)
			if delta > maxDelta {
				if !options.includeAA && (isAntiAliased(a, b, x, y) || isAntiAliased(b, a, x, y)) {
					if out != nil {
						out.SetRGBA(x, y, color.RGBA{R: 255, G: 255, A: 255})
					}
				} else {
					if out != nil {
						out.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
					}
					diff++
				}
			} else {
				if out != nil {
					c := color.GrayModel.Convert(a.At(x, y)).(color.Gray)
					c.Y = 255 - uint8(float64(255-c.Y)*0.1)
					out.Set(x, y, c)
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
	blendRGBA(&ca)
	blendRGBA(&cb)

	y := rgbaToY(&ca) - rgbaToY(&cb)
	if yOnly {
		return y
	}
	i := rgbaToI(&ca) - rgbaToI(&cb)
	q := rgbaToQ(&ca) - rgbaToQ(&cb)
	return 0.5053*y*y + 0.299*i*i + 0.1957*q*q
}

func blendRGBA(rgba *color.RGBA) {
	if rgba.A < 255 {
		a := float64(rgba.A) / 255
		rgba.R = blend(rgba.R, a)
		rgba.G = blend(rgba.G, a)
		rgba.B = blend(rgba.B, a)
	}
}

func blend(c uint8, a float64) uint8 {
	return uint8(255 + float64(c-255)*a)
}

func rgbaToY(rgba *color.RGBA) float64 {
	return float64(rgba.R)*0.29889531 + float64(rgba.G)*0.58662247 + float64(rgba.B)*0.11448223
}

func rgbaToI(rgba *color.RGBA) float64 {
	return float64(rgba.R)*0.59597799 - float64(rgba.G)*0.27417610 - float64(rgba.B)*0.32180189
}

func rgbaToQ(rgba *color.RGBA) float64 {
	return float64(rgba.R)*0.21147017 - float64(rgba.G)*0.52261711 + float64(rgba.B)*0.31114694
}

func isAntiAliased(a, b image.Image, x, y int) bool {
	r := a.Bounds()
	if onEdge(r, x, y) {
		return false
	}

	min := float64(0)
	minX, minY := -1, -1
	max := float64(0)
	maxX, maxY := -1, -1

	c := a.At(x, y)
	zeroes := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dy == 0 && dx == 0 {
				continue
			}
			nx := x + dx
			ny := y + dy
			delta := colorDelta(c, a.At(nx, ny), true)

			switch {
			case delta == 0:
				zeroes++
				if zeroes > 2 {
					return false
				}
			case delta < min:
				min = delta
				minX = nx
				minY = ny
			case max < delta:
				max = delta
				maxX = nx
				maxY = ny
			}
		}
	}

	if max == 0 || min == 0 {
		return false
	}

	return (hasManySiblings(a, minX, minY) && hasManySiblings(b, minX, minY)) || (hasManySiblings(a, maxX, maxY) && hasManySiblings(b, maxX, maxY))
}

func hasManySiblings(img image.Image, x, y int) bool {
	if r := img.Bounds(); onEdge(r, x, y) {
		return false
	}

	zeroes := 0
	r, g, b, a := img.At(x, y).RGBA()
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dy == 0 && dx == 0 {
				continue
			}

			nx := x + dx
			ny := y + dy
			nr, ng, nb, na := img.At(nx, ny).RGBA()
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

func onEdge(r image.Rectangle, x, y int) bool {
	return x == r.Min.X || x == r.Max.X-1 || y == r.Min.Y || y == r.Max.Y-1
}
