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
	R, G, B, A uint32
}

func rgbaFromColor(c *rgba) (r, g, b, a float64) {
	const x = 1.0 / 256.0
	r = float64(c.R) * x
	g = float64(c.G) * x
	b = float64(c.B) * x
	a = float64(c.A) * x
	return
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
	aa := options.alpha / 255
	if isIdentical(a, b) { // fast path if identical
		if out != nil && !options.diffMask {
			rect := a.Bounds()
			aLine := make([]rgba, rect.Dx())
			for y := rect.Min.Y; y < rect.Max.Y; y++ {
				readLine(aLine, a, y)
				for i := range aLine {
					x := rect.Min.X + i
					r, g, b, a := rgbaFromColor(&aLine[i])
					v := uint8(blend(rgbaToY(r, g, b), a*aa))
					out.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
				}
			}
		}
		return 0, nil
	}

	maxDelta := 35215 * options.threshold * options.threshold
	diff := 0

	rect := a.Bounds()
	var outLine []uint8
	if out != nil {
		outLine = make([]uint8, rect.Dx()*4)
		for i := range outLine {
			outLine[i] = 0xff
		}
	}

	y := rect.Min.Y
	ar := newImageLineReader(a, y)
	br := newImageLineReader(b, y)
	for ; ar.Next() && br.Next(); y++ {
		aLine := ar.Line()
		bLine := br.Line()

		for i := range aLine {
			delta := colorDelta(&aLine[i], &bLine[i], false)
			x := rect.Min.X + i
			if math.Abs(delta) > maxDelta {
				if !options.includeAA && (isAntiAliased(ar, br, x, y) || isAntiAliased(br, ar, x, y)) {
					if out != nil && !options.diffMask {
						c := options.antiAliasedColor
						d := outLine[i*4 : i*4+4 : i*4+4]
						d[0] = c.R
						d[1] = c.G
						d[2] = c.B
					}
				} else {
					if out != nil {
						if delta < 0 && options.diffColorAlt != nil {
							c := *options.diffColorAlt
							d := outLine[i*4 : i*4+4 : i*4+4]
							d[0] = c.R
							d[1] = c.G
							d[2] = c.B
						} else {
							c := options.diffColor
							d := outLine[i*4 : i*4+4 : i*4+4]
							d[0] = c.R
							d[1] = c.G
							d[2] = c.B
						}
					}
					diff++
				}
			} else {
				if out != nil && !options.diffMask {
					r, g, b, a := rgbaFromColor(&aLine[i])
					v := uint8(blend(rgbaToY(r, g, b), aa*a))
					d := outLine[i*4 : i*4+4 : i*4+4]
					d[0] = v
					d[1] = v
					d[2] = v
				}
			}
		}
		if out != nil {
			copy(out.Pix[out.PixOffset(rect.Min.X, y):], outLine)
		}
	}

	if options.writeTo != nil {
		*options.writeTo = out
	}

	return diff, nil
}

func colorDelta(a, b *rgba, yOnly bool) float64 {
	if a.A == b.A && a.R == b.R && a.G == b.G && a.B == b.B {
		return 0
	}
	ar, ag, ab, aa := rgbaFromColor(a)
	if aa < 255 {
		ar, ag, ab = blendRGBA(ar, ag, ab, aa)
	}

	br, bg, bb, ba := rgbaFromColor(b)
	if ba < 255 {
		br, bg, bb = blendRGBA(br, bg, bb, ba)
	}

	ya := rgbaToY(ar, ag, ab)
	yb := rgbaToY(br, bg, bb)
	y := ya - yb
	if yOnly {
		return y
	}
	i := rgbaToI(ar, ag, ab) - rgbaToI(br, bg, bb)
	q := rgbaToQ(ar, ag, ab) - rgbaToQ(br, bg, bb)
	delta := 0.5053*y*y + 0.299*i*i + 0.1957*q*q
	if ya > yb {
		return -delta
	} else {
		return delta
	}
}

func blendRGBA(r, g, b, a float64) (float64, float64, float64) {
	a /= 255
	return blend(r, a), blend(g, a), blend(b, a)
}

func blend(c float64, a float64) float64 {
	return 255 + (c-255)*a
}

func rgbaToY(r, g, b float64) float64 {
	return r*0.29889531 + g*0.58662247 + b*0.11448223
}

func rgbaToI(r, g, b float64) float64 {
	return r*0.59597799 - g*0.27417610 - b*0.32180189
}

func rgbaToQ(r, g, b float64) float64 {
	return r*0.21147017 - g*0.52261711 + b*0.31114694
}

func isAntiAliased(a, b *imageLineReader, x1, y1 int) bool {
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
			if delta == 0 {
				zeroes++
				if zeroes > 2 {
					return false
				}
			} else if delta < min {
				min = delta
				minX = x
				minY = y
			} else if delta > max {
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

func hasManySiblings(img *imageLineReader, x1, y1 int) bool {
	rect := img.Bounds()
	x0 := maxInt(x1-1, rect.Min.X)
	y0 := maxInt(y1-1, rect.Min.Y)
	x2 := minInt(x1+1, rect.Max.X-1)
	y2 := minInt(y1+1, rect.Max.Y-1)
	zeroes := 0
	if x1 == x0 || x1 == x2 || y1 == y0 || y1 == y2 {
		zeroes = 1
	}

	a := img.At(x1, y1)
	for x := x0; x <= x2; x++ {
		for y := y0; y <= y2; y++ {
			if x == x1 && y == y1 {
				continue
			}

			b := img.At(x, y)
			if a.R == b.R && a.G == b.G && a.B == b.B && a.A == b.A {
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
	case *image.Gray16:
		y, ok := b.(*image.Gray16)
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
		if !bytes.Equal(pixA[y*strideA:y*strideA+strideA], pixB[y*strideB:y*strideB+strideB]) {
			return false
		}
	}
	return true
}

func readLine(dst []rgba, img image.Image, y int) {
	rect := img.Bounds()
	switch v := img.(type) {
	case *image.RGBA:
		lineOffset := v.PixOffset(rect.Min.X, y)
		for i := range dst {
			offset := lineOffset + i*4
			s := v.Pix[offset : offset+4 : offset+4]
			r := uint32(s[0])
			g := uint32(s[1])
			b := uint32(s[2])
			a := uint32(s[3])
			dst[i] = rgba{r<<8 | r, g<<8 | g, b<<8 | b, a<<8 | a}
		}
	case *image.RGBA64:
		lineOffset := v.PixOffset(rect.Min.X, y)
		for i := range dst {
			offset := lineOffset + i*8
			s := v.Pix[offset : offset+8 : offset+8]
			r := uint32(s[0])<<8 | uint32(s[1])
			g := uint32(s[2])<<8 | uint32(s[3])
			b := uint32(s[4])<<8 | uint32(s[5])
			a := uint32(s[6])<<8 | uint32(s[7])
			dst[i] = rgba{r, g, b, a}
		}
	case *image.NRGBA:
		lineOffset := v.PixOffset(rect.Min.X, y)
		for i := range dst {
			offset := lineOffset + i*4
			s := v.Pix[offset : offset+4 : offset+4]
			r := uint32(s[0])
			g := uint32(s[1])
			b := uint32(s[2])
			a := uint32(s[3])
			if a == 0xff {
				dst[i] = rgba{r<<8 | r, g<<8 | g, b<<8 | b, 0xffff}
			} else {
				dst[i] = rgba{(r<<8 | r) * a / 0xff, (g<<8 | g) * a / 0xff, (b<<8 | b) * a / 0xff, a<<8 | a}
			}
		}
	case *image.NRGBA64:
		lineOffset := v.PixOffset(rect.Min.X, y)
		for i := range dst {
			offset := lineOffset + i*8
			s := v.Pix[offset : offset+8 : offset+8]
			r := uint32(s[0])<<8 | uint32(s[1])
			g := uint32(s[2])<<8 | uint32(s[3])
			b := uint32(s[4])<<8 | uint32(s[5])
			a := uint32(s[6])<<8 | uint32(s[7])
			dst[i] = rgba{r * a / 0xffff, g * a / 0xffff, b * a / 0xffff, a}
		}
	case *image.Gray:
		lineOffset := v.PixOffset(rect.Min.X, y)
		for i := range dst {
			y := uint32(v.Pix[lineOffset+i])
			y |= y << 8
			dst[i] = rgba{y, y, y, 0xffff}
		}
	case *image.Gray16:
		lineOffset := v.PixOffset(rect.Min.X, y)
		for i := range dst {
			offset := lineOffset + i*2
			s := v.Pix[offset : offset+2 : offset+2]
			y := uint32(s[0])<<8 | uint32(s[1])
			dst[i] = rgba{y, y, y, 0xffff}
		}
	default:
		for i := range dst {
			r, g, b, a := v.At(rect.Min.X+i, y).RGBA()
			dst[i] = rgba{r, g, b, a}
		}
	}
}

type imageLineReader struct {
	image image.Image

	rect  image.Rectangle
	width int

	y     int
	lines [5][]rgba
}

func newImageLineReader(img image.Image, y int) *imageLineReader {
	rect := img.Bounds()
	width := rect.Dx()
	return &imageLineReader{
		image: img,
		rect:  rect,
		width: width,
		y:     y,
	}
}

func (r *imageLineReader) Next() bool {
	if r.y == r.rect.Max.Y {
		return false
	}
	if r.lines[2] == nil {
		for i := range r.lines {
			y := r.y + i - 2
			if r.rect.Min.Y <= y && y < r.rect.Max.Y {
				line := make([]rgba, r.width)
				readLine(line, r.image, y)
				r.lines[i] = line
			}
		}
	} else {
		old := r.lines[0]
		r.lines[0] = r.lines[1]
		r.lines[1] = r.lines[2]
		r.lines[2] = r.lines[3]
		r.lines[3] = r.lines[4]
		r.lines[4] = old
		if r.rect.Min.Y <= r.y+2 && r.y+2 < r.rect.Max.Y {
			if r.lines[4] == nil {
				r.lines[4] = make([]rgba, r.width)
			}
			readLine(r.lines[4], r.image, r.y+2)
		} else {
			r.lines[4] = nil
		}
	}
	r.y++
	return true
}

func (r *imageLineReader) Line() []rgba {
	return r.lines[2]
}

func (r *imageLineReader) Y() int {
	return r.y - 1
}

func (r *imageLineReader) At(x, y int) *rgba {
	return &r.lines[y-r.Y()+2][x]
}

func (r *imageLineReader) Bounds() image.Rectangle {
	return r.rect
}
