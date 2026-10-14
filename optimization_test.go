package pixelmatch

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"math/rand"
	"testing"
)

func TestEqualRowsMatchFallback(t *testing.T) {
	for _, typ := range imageTypes {
		t.Run(typ.name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(1))
			aRect := image.Rect(-3, 2, 4, 21)
			bRect := image.Rect(4, -5, 11, 14)
			makeCrop := func(rect image.Rectangle) draw.Image {
				return typ.new(rect.Inset(-2)).(interface {
					SubImage(image.Rectangle) image.Image
				}).SubImage(rect).(draw.Image)
			}
			a, b := makeCrop(aRect), makeCrop(bRect)
			mask := make([]byte, aRect.Dx()*aRect.Dy())
			for y := 0; y < aRect.Dy(); y++ {
				for x := 0; x < aRect.Dx(); x++ {
					c := color.NRGBA64{R: uint16(rng.Intn(65536)), G: uint16(rng.Intn(65536)), B: uint16(rng.Intn(65536)), A: uint16(rng.Intn(65536))}
					a.Set(aRect.Min.X+x, aRect.Min.Y+y, c)
					b.Set(bRect.Min.X+x, bRect.Min.Y+y, c)
					mask[y*aRect.Dx()+x] = byte(rng.Intn(2))
				}
			}
			// Long equal stretches exercise deferred reads and buffer reuse; the
			// first and last rows exercise clipped neighborhoods.
			for _, y := range []int{0, 8, 9, 18} {
				b.Set(bRect.Min.X+2, bRect.Min.Y+y, color.White)
			}
			for _, opts := range [][]MatchOption{
				nil,
				{IncludeAntiAlias},
				{EnableDiffMask},
				{IgnoreMask(mask), WindowSize(3)},
				{Checkerboard(false), Threshold(0), Alpha(0.5)},
			} {
				var got, want image.Image
				gotOpts := append(append([]MatchOption(nil), opts...), WriteTo(&got))
				wantOpts := append(append([]MatchOption(nil), opts...), WriteTo(&want))
				count, err := MatchPixel(a, b, gotOpts...)
				wantCount, wantErr := MatchPixel(wrappedImage{a}, wrappedImage{b}, wantOpts...)
				if err != nil || wantErr != nil || count != wantCount || !bytes.Equal(got.(*image.RGBA).Pix, want.(*image.RGBA).Pix) {
					t.Fatalf("direct result differs from fallback: counts %d/%d, errors %v/%v", count, wantCount, err, wantErr)
				}
				if countOnly, err := MatchPixel(a, b, opts...); err != nil || countOnly != wantCount {
					t.Fatalf("without output: got %d, %v; want %d", countOnly, err, wantCount)
				}
			}
		})
	}
}

func TestSiblingCacheMatchesImage(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	palette := []color.NRGBA64{
		{A: 65535},
		{R: 65535, G: 65535, B: 65535, A: 65535},
		{R: 12345, G: 12345, B: 12345, A: 32768},
		{R: 65535},
	}
	for _, typ := range imageTypes {
		t.Run(typ.name, func(t *testing.T) {
			for _, size := range []image.Point{{1, 1}, {1, 13}, {7, 1}, {2, 3}, {7, 13}} {
				rect := image.Rectangle{Min: image.Pt(-3, 2), Max: image.Pt(-3, 2).Add(size)}
				img := typ.new(rect)
				for y := rect.Min.Y; y < rect.Max.Y; y++ {
					for x := rect.Min.X; x < rect.Max.X; x++ {
						c := palette[0]
						if y%4 < 2 {
							c = palette[rng.Intn(len(palette))]
						}
						img.Set(x, y, c)
					}
				}
				r := newImageLineReader(img, rect.Min.Y)
				for r.Next() {
					if r.Y()%4 == 0 {
						continue // Some rows never load their pixels or sibling cache.
					}
					r.readNeighborhood()
					for y := maxInt(rect.Min.Y, r.Y()-1); y <= minInt(rect.Max.Y-1, r.Y()+1); y++ {
						for x := rect.Min.X; x < rect.Max.X; x++ {
							want := referenceSiblings(img, x, y)
							for repeat := 0; repeat < 2; repeat++ {
								if got := hasManySiblings(&r, x, y); got != want {
									t.Fatalf("size=%v current row=%d pixel=(%d,%d): got %v, want %v", size, r.Y(), x, y, got, want)
								}
							}
						}
					}
				}
			}
		})
	}
}

// Count directly from the source image, independently of the row reader/cache.
func referenceSiblings(img image.Image, x, y int) bool {
	rect := img.Bounds()
	count := 0
	if x == rect.Min.X || x == rect.Max.X-1 || y == rect.Min.Y || y == rect.Max.Y-1 {
		count++
	}
	c := readColor(img.At(x, y))
	for ny := y - 1; ny <= y+1; ny++ {
		for nx := x - 1; nx <= x+1; nx++ {
			if (nx != x || ny != y) && image.Pt(nx, ny).In(rect) && readColor(img.At(nx, ny)) == c {
				count++
			}
		}
	}
	return count > 2
}
