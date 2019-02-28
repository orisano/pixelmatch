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
			delta := subColor(a.At(x, y), b.At(x, y))
			if delta > maxDelta {
				if out != nil {
					out.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
				}
				diff++
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

func subColor(a, b color.Color) float64 {
	y1, i1, q1 := blendColor(a)
	y2, i2, q2 := blendColor(b)
	return 0.5053*(y1-y2)*(y1-y2) + 0.299*(i1-i2)*(i1-i2) + 0.1957*(q1-q2)*(q1-q2)
}

func blendColor(c color.Color) (float64, float64, float64) {
	r, g, b, a := c.RGBA()
	r = r >> 8
	g = g >> 8
	b = b >> 8
	a = a >> 8
	if a < 255 {
		x := float64(a) / 255
		r = uint32(255 + float64(r-255)*x)
		g = uint32(255 + float64(g-255)*x)
		b = uint32(255 + float64(b-255)*x)
	}
	y := float64(r)*0.29889531 + float64(g)*0.58662247 + float64(b)*0.11448223
	i := float64(r)*0.59597799 + float64(g)*0.27417610 + float64(b)*0.32180189
	q := float64(r)*0.21147017 + float64(g)*0.52261711 + float64(b)*0.31114694
	return y, i, q
}
