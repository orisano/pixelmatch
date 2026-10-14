package pixelmatch

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"math"
)

var ErrImageSizesNotMatch = errors.New("image sizes do not match")

var ErrIgnoreMaskSizeNotMatch = errors.New("ignore mask size does not match image width/height")

type MatchOptions struct {
	threshold        float64
	includeAA        bool
	alpha            float64
	antiAliasedColor color.RGBA
	diffColor        color.RGBA
	diffColorAlt     *color.RGBA
	diffMask         bool
	checkerboard     bool
	windowSize       float64
	ignoreMask       []byte
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
		o.antiAliasedColor = outputColor(c)
	}
}

func DiffColor(c color.Color) MatchOption {
	return func(o *MatchOptions) {
		o.diffColor = outputColor(c)
	}
}

func DiffColorAlt(c color.Color) MatchOption {
	return func(o *MatchOptions) {
		diffColorAlt := outputColor(c)
		o.diffColorAlt = &diffColorAlt
	}
}

func EnableDiffMask(o *MatchOptions) {
	o.diffMask = true
}

// Checkerboard controls the comparison background for semi-transparent pixels.
// It defaults to true; false composites pixels over white.
func Checkerboard(enabled bool) MatchOption {
	return func(o *MatchOptions) {
		o.checkerboard = enabled
	}
}

// WindowSize returns the largest diff count in any N by N sliding window.
// Finite sizes are floored and clamped to [1, min(width, height)]. Non-finite
// sizes, including the default +Inf, return the total diff count.
func WindowSize(size float64) MatchOption {
	return func(o *MatchOptions) {
		o.windowSize = size
	}
}

// IgnoreMask skips pixels whose mask byte is nonzero. The mask must contain
// width*height bytes in row order, independent of the images' bounds origins.
// A nil mask disables masking. Ignored pixels are drawn as unchanged.
func IgnoreMask(mask []byte) MatchOption {
	return func(o *MatchOptions) {
		o.ignoreMask = mask
	}
}

// Output colors are opaque RGB markers, regardless of the supplied alpha.
func outputColor(c color.Color) color.RGBA {
	switch c := c.(type) {
	case color.NRGBA:
		return color.RGBA{R: c.R, G: c.G, B: c.B, A: 255}
	case color.NRGBA64:
		return color.RGBA{R: uint8(c.R >> 8), G: uint8(c.G >> 8), B: uint8(c.B >> 8), A: 255}
	default:
		return color.RGBAModel.Convert(c).(color.RGBA)
	}
}

// rgba stores non-premultiplied channels in the range 0 to 65535.
type rgba struct {
	R, G, B, A uint32
}

func rgbaFromColor(c *rgba) (r, g, b, a float64) {
	r = float64(c.R) / 257
	g = float64(c.G) / 257
	b = float64(c.B) / 257
	a = float64(c.A) / 257
	return
}

func unpremultiply(r, g, b, a uint32) rgba {
	if a != 0 && a != 0xffff {
		r = r * 0xffff / a
		g = g * 0xffff / a
		b = b * 0xffff / a
	}
	return rgba{r, g, b, a}
}

func readColor(c color.Color) rgba {
	switch c := c.(type) {
	case color.NRGBA:
		r, g, b, a := uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
		return rgba{r<<8 | r, g<<8 | g, b<<8 | b, a<<8 | a}
	case color.NRGBA64:
		return rgba{uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)}
	default:
		r, g, b, a := c.RGBA()
		return unpremultiply(r, g, b, a)
	}
}

func MatchPixel(a, b image.Image, opts ...MatchOption) (int, error) {
	options := MatchOptions{
		threshold:        0.1,
		alpha:            0.1,
		antiAliasedColor: color.RGBA{R: 255, G: 255},
		diffColor:        color.RGBA{R: 255},
		checkerboard:     true,
		windowSize:       math.Inf(1),
	}
	for _, opt := range opts {
		opt(&options)
	}

	rect, bRect := a.Bounds(), b.Bounds()
	if rect.Size() != bRect.Size() {
		return 0, ErrImageSizesNotMatch
	}
	if options.ignoreMask != nil && len(options.ignoreMask) != rect.Dx()*rect.Dy() {
		return 0, ErrIgnoreMaskSizeNotMatch
	}

	var out *image.RGBA
	if options.writeTo != nil {
		out = image.NewRGBA(rect)
	}
	pixA, pixB, strideA, strideB, bytesPerPixel := matchingImagePixels(a, b)
	if bytesPerPixel != 0 && equals(pixA, pixB, strideA, strideB, rect, bytesPerPixel) {
		if out != nil && !options.diffMask {
			aLine := make([]rgba, rect.Dx())
			for y := rect.Min.Y; y < rect.Max.Y; y++ {
				readLine(aLine, a, y)
				offset := out.PixOffset(rect.Min.X, y)
				line := out.Pix[offset : offset+rect.Dx()*4]
				drawGrayLine(line, aLine, options.alpha)
			}
		}
		if options.writeTo != nil {
			*options.writeTo = out
		}
		return 0, nil
	}

	diff := 0
	var diffPixels []byte
	if !math.IsInf(options.windowSize, 0) && !math.IsNaN(options.windowSize) {
		diffPixels = make([]byte, rect.Dx()*rect.Dy())
	}

	var outLine []uint8
	width := rect.Dx()
	rowBytes := width * bytesPerPixel

	y := rect.Min.Y
	ar := newImageLineReader(a, y)
	br := newImageLineReader(b, bRect.Min.Y)
	for ; ar.Next() && br.Next(); y++ {
		row := y - rect.Min.Y
		if out != nil {
			offset := out.PixOffset(rect.Min.X, y)
			outLine = out.Pix[offset : offset+width*4 : offset+width*4]
		}
		// Equal rows need neither color comparison nor AA detection. Neighbor
		// rows remain available through the lazy line readers when a diff needs them.
		if bytesPerPixel != 0 && bytes.Equal(pixA[row*strideA:row*strideA+rowBytes], pixB[row*strideB:row*strideB+rowBytes]) {
			if out != nil && !options.diffMask {
				drawGrayLine(outLine, ar.Line(), options.alpha)
			}
			continue
		}
		aLine := ar.Line()
		bLine := br.Line()
		rowIndex := row * width
		if !options.includeAA {
			ar.readNeighborhood()
			br.readNeighborhood()
		}

		for i := range aLine {
			index := rowIndex + i
			delta := 0
			if options.ignoreMask == nil || options.ignoreMask[index] == 0 {
				delta = colorDelta(&aLine[i], &bLine[i], index, options.checkerboard, options.threshold)
			}
			x := rect.Min.X + i
			if delta != 0 {
				if !options.includeAA && (isAntiAliased(&ar, &br, x, y) || isAntiAliased(&br, &ar, bRect.Min.X+i, br.Y())) {
					if out != nil && !options.diffMask {
						c := options.antiAliasedColor
						d := outLine[i*4 : i*4+4 : i*4+4]
						d[0] = c.R
						d[1] = c.G
						d[2] = c.B
						d[3] = 255
					}
				} else {
					if out != nil {
						c := options.diffColor
						if delta < 0 && options.diffColorAlt != nil {
							c = *options.diffColorAlt
						}
						d := outLine[i*4 : i*4+4 : i*4+4]
						d[0] = c.R
						d[1] = c.G
						d[2] = c.B
						d[3] = 255
					}
					diff++
					if diffPixels != nil {
						diffPixels[index] = 1
					}
				}
			} else {
				if out != nil && !options.diffMask {
					v := grayPixel(&aLine[i], options.alpha)
					d := outLine[i*4 : i*4+4 : i*4+4]
					d[0] = v
					d[1] = v
					d[2] = v
					d[3] = 255
				}
			}
		}
	}

	if options.writeTo != nil {
		*options.writeTo = out
	}

	if diffPixels != nil && diff != 0 {
		return maxWindowDiff(diffPixels, rect.Dx(), rect.Dy(), options.windowSize), nil
	}
	return diff, nil
}

func grayPixel(c *rgba, alpha float64) uint8 {
	r, g, b, a := rgbaFromColor(c)
	return uint8(255 + (rgbaToY(r, g, b)-255)*alpha*a/255)
}

func drawGrayLine(dst []byte, src []rgba, alpha float64) {
	for i := range src {
		v := grayPixel(&src[i], alpha)
		d := dst[i*4 : i*4+4 : i*4+4]
		d[0], d[1], d[2], d[3] = v, v, v, 255
	}
}

func rgbaToY(r, g, b float64) float64 {
	return r*0.29889531 + g*0.58662247 + b*0.11448223
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
			delta := brightnessDelta(c, a.At(x, y))
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

	offset := b.Bounds().Min.Sub(r.Min)
	return (hasManySiblings(a, minX, minY) && hasManySiblings(b, minX+offset.X, minY+offset.Y)) ||
		(hasManySiblings(a, maxX, maxY) && hasManySiblings(b, maxX+offset.X, maxY+offset.Y))
}

func hasManySiblings(img *imageLineReader, x1, y1 int) bool {
	rect := img.Bounds()
	row := y1 - img.Y() + 2
	if img.siblings[row] == nil {
		if img.siblingBuffer == nil {
			img.siblingBuffer = make([]byte, img.width*minInt(5, rect.Dy()))
		}
		img.siblings[row] = img.siblingBuffer[:img.width:img.width]
		img.siblingBuffer = img.siblingBuffer[img.width:]
	}
	cache := img.siblings[row]
	index := x1 - rect.Min.X
	if cache[index] != 0 {
		return cache[index] == 2
	}
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
				cache[index] = 2
				return true
			}
		}
	}
	cache[index] = 1
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

// Raw pixels can only be compared directly when both images share a format.
func matchingImagePixels(a, b image.Image) (pixA, pixB []byte, strideA, strideB, bytesPerPixel int) {
	switch x := a.(type) {
	case *image.RGBA:
		y, ok := b.(*image.RGBA)
		if ok {
			return x.Pix, y.Pix, x.Stride, y.Stride, 4
		}
	case *image.RGBA64:
		y, ok := b.(*image.RGBA64)
		if ok {
			return x.Pix, y.Pix, x.Stride, y.Stride, 8
		}
	case *image.NRGBA:
		y, ok := b.(*image.NRGBA)
		if ok {
			return x.Pix, y.Pix, x.Stride, y.Stride, 4
		}
	case *image.NRGBA64:
		y, ok := b.(*image.NRGBA64)
		if ok {
			return x.Pix, y.Pix, x.Stride, y.Stride, 8
		}
	case *image.Gray:
		y, ok := b.(*image.Gray)
		if ok {
			return x.Pix, y.Pix, x.Stride, y.Stride, 1
		}
	case *image.Gray16:
		y, ok := b.(*image.Gray16)
		if ok {
			return x.Pix, y.Pix, x.Stride, y.Stride, 2
		}
	}
	return nil, nil, 0, 0, 0
}

func equals(pixA, pixB []uint8, strideA, strideB int, rect image.Rectangle, bytesPerPixel int) bool {
	w := rect.Dx() * bytesPerPixel
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
			dst[i] = unpremultiply(r<<8|r, g<<8|g, b<<8|b, a<<8|a)
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
			dst[i] = unpremultiply(r, g, b, a)
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
			dst[i] = rgba{r<<8 | r, g<<8 | g, b<<8 | b, a<<8 | a}
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
			dst[i] = rgba{r, g, b, a}
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
			dst[i] = readColor(v.At(rect.Min.X+i, y))
		}
	}
}

type imageLineReader struct {
	image image.Image

	rect  image.Rectangle
	width int

	y      int
	lines  [5][]rgba
	ready  [5]bool
	buffer []rgba
	// Sibling counts depend only on one image, so neighboring AA checks can
	// share them. Zero is unknown, one is false, and two is true.
	siblings      [5][]byte
	siblingBuffer []byte
}

func newImageLineReader(img image.Image, y int) imageLineReader {
	rect := img.Bounds()
	width := rect.Dx()
	return imageLineReader{
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
	if r.y != r.rect.Min.Y {
		old := r.lines[0]
		r.lines[0] = r.lines[1]
		r.lines[1] = r.lines[2]
		r.lines[2] = r.lines[3]
		r.lines[3] = r.lines[4]
		r.lines[4] = old
		r.ready[0], r.ready[1], r.ready[2], r.ready[3], r.ready[4] =
			r.ready[1], r.ready[2], r.ready[3], r.ready[4], false
		oldSiblings := r.siblings[0]
		r.siblings[0], r.siblings[1], r.siblings[2], r.siblings[3], r.siblings[4] =
			r.siblings[1], r.siblings[2], r.siblings[3], r.siblings[4], oldSiblings
		for i := range r.siblings[4] {
			r.siblings[4][i] = 0
		}
	}
	r.y++
	return true
}

func (r *imageLineReader) Line() []rgba {
	return r.line(2)
}

func (r *imageLineReader) line(i int) []rgba {
	if !r.ready[i] {
		if r.lines[i] == nil {
			if r.buffer == nil {
				r.buffer = make([]rgba, r.width*minInt(5, r.rect.Dy()))
			}
			r.lines[i] = r.buffer[:r.width:r.width]
			r.buffer = r.buffer[r.width:]
		}
		readLine(r.lines[i], r.image, r.Y()+i-2)
		r.ready[i] = true
	}
	return r.lines[i]
}

func (r *imageLineReader) Y() int {
	return r.y - 1
}

func (r *imageLineReader) readNeighborhood() {
	for i := range r.lines {
		y := r.Y() + i - 2
		if r.rect.Min.Y <= y && y < r.rect.Max.Y {
			r.line(i)
		}
	}
}

func (r *imageLineReader) At(x, y int) *rgba {
	// readNeighborhood prepares these rows before AA detection starts.
	return &r.lines[y-r.Y()+2][x-r.rect.Min.X]
}

func (r *imageLineReader) Bounds() image.Rectangle {
	return r.rect
}
