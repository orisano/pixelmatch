package pixelmatch

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// These counts and output colors were checked against mapbox/pixelmatch v8.0.0.
func TestMatchPixelColorCompatibility(t *testing.T) {
	tests := []struct {
		name string
		a, b color.NRGBA
		opts []MatchOption
		diff int
		want color.RGBA
	}{
		{"black white boundary", color.NRGBA{A: 255}, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, []MatchOption{Threshold(1)}, 1, color.RGBA{R: 255, A: 255}},
		{"threshold boundary", color.NRGBA{A: 255}, color.NRGBA{R: 26, G: 26, B: 26, A: 255}, []MatchOption{Threshold(0.0986)}, 1, color.RGBA{R: 255, A: 255}},
		{"gray rounding", color.NRGBA{R: 49, G: 49, B: 49, A: 255}, color.NRGBA{R: 49, G: 49, B: 49, A: 255}, []MatchOption{Alpha(0.5)}, 0, color.RGBA{R: 152, G: 152, B: 152, A: 255}},
		{"transparent white checkerboard", color.NRGBA{R: 255, G: 255, B: 255, A: 85}, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, nil, 1, color.RGBA{R: 255, A: 255}},
		{"transparent white", color.NRGBA{R: 255, G: 255, B: 255, A: 85}, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, []MatchOption{Checkerboard(false)}, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{"red over white", color.NRGBA{R: 255, A: 85}, color.NRGBA{R: 255, G: 170, B: 170, A: 255}, []MatchOption{Checkerboard(false)}, 0, color.RGBA{R: 249, G: 249, B: 249, A: 255}},
		{"transparent gray output", color.NRGBA{R: 30, G: 60, B: 90, A: 85}, color.NRGBA{R: 30, G: 60, B: 90, A: 85}, []MatchOption{Alpha(0.5)}, 0, color.RGBA{R: 221, G: 221, B: 221, A: 255}},
		{"lightens", color.NRGBA{A: 255}, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, []MatchOption{DiffColorAlt(color.RGBA{G: 255})}, 1, color.RGBA{R: 255, A: 255}},
		{"darkens", color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{A: 255}, []MatchOption{DiffColorAlt(color.RGBA{G: 255})}, 1, color.RGBA{G: 255, A: 255}},
	}
	for _, tc := range tests {
		types := imageTypes[:4]
		if tc.a.A == 255 && tc.b.A == 255 && tc.a.R == tc.a.G && tc.a.G == tc.a.B && tc.b.R == tc.b.G && tc.b.G == tc.b.B {
			types = imageTypes
		}
		for _, typ := range types {
			t.Run(tc.name+"/"+typ.name, func(t *testing.T) {
				rect := image.Rect(0, 0, 1, 1)
				a, b := typ.new(rect), typ.new(rect)
				a.Set(0, 0, tc.a)
				b.Set(0, 0, tc.b)
				// Also exercise the fallback for other implementations of image.Image.
				for _, wrapped := range []bool{false, true} {
					var imgA, imgB image.Image = a, b
					if wrapped {
						imgA, imgB = wrappedImage{a}, wrappedImage{b}
					}
					var out image.Image
					opts := append([]MatchOption{WriteTo(&out)}, tc.opts...)
					diff, err := MatchPixel(imgA, imgB, opts...)
					if err != nil || diff != tc.diff {
						t.Errorf("wrapped=%v: MatchPixel() = %d, %v; want %d, nil", wrapped, diff, err, tc.diff)
					}
					if out == nil {
						t.Fatal("output image is nil")
					}
					if got := color.RGBAModel.Convert(out.At(0, 0)); got != tc.want {
						t.Errorf("wrapped=%v: output = %v; want %v", wrapped, got, tc.want)
					}
					if diff, err := MatchPixel(imgA, imgB, tc.opts...); err != nil || diff != tc.diff {
						t.Errorf("wrapped=%v: without output = %d, %v; want %d, nil", wrapped, diff, err, tc.diff)
					}
				}
			})
		}
	}
}

type wrappedImage struct{ image.Image }

func TestMatchPixelMaskCompatibility(t *testing.T) {
	// The last middle pixel is excluded as AA; the preceding rows count as diffs.
	// This also catches stale diff pixels when the output row buffer is reused.
	a, b := image.NewNRGBA(image.Rect(-3, 2, 0, 5)), image.NewNRGBA(image.Rect(4, -5, 7, -2))
	for y := 0; y < 3; y++ {
		for x, v := range []uint8{0, 128, 255} {
			a.SetNRGBA(x+a.Rect.Min.X, y+a.Rect.Min.Y, color.NRGBA{R: v, G: v, B: v, A: 255})
			if x == 1 {
				v = 192
			}
			b.SetNRGBA(x+b.Rect.Min.X, y+b.Rect.Min.Y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	for _, mask := range []bool{false, true} {
		for _, includeAA := range []bool{false, true} {
			var out image.Image
			opts := []MatchOption{WriteTo(&out), AntiAliasedColor(color.NRGBA{R: 255, G: 255, A: 85})}
			if mask {
				opts = append(opts, EnableDiffMask)
			}
			wantDiff := 2
			if includeAA {
				opts = append(opts, IncludeAntiAlias)
				wantDiff = 3
			}
			diff, err := MatchPixel(a, b, opts...)
			if err != nil || diff != wantDiff {
				t.Fatalf("mask=%v, includeAA=%v: MatchPixel() = %d, %v; want %d, nil", mask, includeAA, diff, err, wantDiff)
			}
			for y := 0; y < 3; y++ {
				for x := 0; x < 3; x++ {
					var want color.RGBA
					switch {
					case x == 1 && (y < 2 || includeAA):
						want = color.RGBA{R: 255, A: 255}
					case mask:
					case x == 0:
						want = color.RGBA{R: 229, G: 229, B: 229, A: 255}
					case x == 1:
						want = color.RGBA{R: 255, G: 255, A: 255}
					default:
						want = color.RGBA{R: 255, G: 255, B: 255, A: 255}
					}
					if got := color.RGBAModel.Convert(out.At(x+a.Rect.Min.X, y+a.Rect.Min.Y)); got != want {
						t.Errorf("mask=%v, includeAA=%v: pixel (%d, %d) = %v; want %v", mask, includeAA, x, y, got, want)
					}
				}
			}
		}
	}
}

func TestMatchPixelDifferentOrigins(t *testing.T) {
	for _, typ := range imageTypes {
		t.Run(typ.name, func(t *testing.T) {
			aRect, bRect := image.Rect(-3, 2, 0, 5), image.Rect(4, -5, 7, -2)
			a, b := typ.new(aRect), typ.new(bRect)
			draw.Draw(a, aRect, image.Black, image.Point{}, draw.Src)
			draw.Draw(b, bRect, image.Black, image.Point{}, draw.Src)
			for _, different := range []bool{false, true} {
				want := 0
				if different {
					b.Set(bRect.Min.X+1, bRect.Min.Y+1, color.White)
					want = 1
				}
				var out image.Image
				diff, err := MatchPixel(a, b, WriteTo(&out))
				if err != nil || diff != want {
					t.Fatalf("different=%v: MatchPixel() = %d, %v; want %d, nil", different, diff, err, want)
				}
				if out == nil || out.Bounds() != aRect {
					t.Fatal("output must have the first image's bounds")
				}
			}
		})
	}
}

func TestMatchPixelDifferentSizes(t *testing.T) {
	a, b := image.NewNRGBA(image.Rect(0, 0, 2, 2)), image.NewNRGBA(image.Rect(0, 0, 4, 1))
	if _, err := MatchPixel(a, b); err != ErrImageSizesNotMatch {
		t.Fatalf("MatchPixel() error = %v; want %v", err, ErrImageSizesNotMatch)
	}
}

func TestMatchPixelOutputColors(t *testing.T) {
	a, b := image.NewGray(image.Rect(0, 0, 2, 1)), image.NewGray(image.Rect(0, 0, 2, 1))
	a.SetGray(1, 0, color.Gray{Y: 255})
	b.SetGray(0, 0, color.Gray{Y: 255})
	var out image.Image
	diff, err := MatchPixel(a, b, WriteTo(&out),
		DiffColor(color.NRGBA{R: 20, G: 40, B: 60, A: 85}),
		DiffColorAlt(color.NRGBA64{R: 0x1234, G: 0x5678, B: 0x9abc}))
	if err != nil || diff != 2 {
		t.Fatalf("MatchPixel() = %d, %v; want 2, nil", diff, err)
	}
	for x, want := range []color.RGBA{{R: 20, G: 40, B: 60, A: 255}, {R: 0x12, G: 0x56, B: 0x9a, A: 255}} {
		if got := color.RGBAModel.Convert(out.At(x, 0)); got != want {
			t.Errorf("pixel (%d, 0) = %v; want %v", x, got, want)
		}
	}
}

func TestMatchPixelTransparentSiblings(t *testing.T) {
	// Transparent neighbors have distinct RGB values in the JS input. Dropping
	// those values creates equal siblings and incorrectly excludes the center as AA.
	pixels := []color.NRGBA{
		{A: 255}, {R: 255}, {R: 1, G: 1, B: 1, A: 255},
		{G: 255}, {R: 128, G: 128, B: 128, A: 255}, {B: 255},
		{R: 2, G: 2, B: 2, A: 255}, {R: 255, G: 255, B: 255}, {R: 3, G: 3, B: 3, A: 255},
	}
	rect := image.Rect(0, 0, 3, 3)
	a, b := image.NewNRGBA(rect), image.NewNRGBA(rect)
	palette := make(color.Palette, len(pixels)+1)
	for i, c := range pixels {
		a.SetNRGBA(i%3, i/3, c)
		b.SetNRGBA(i%3, i/3, c)
		palette[i] = c
	}
	b.SetNRGBA(1, 1, color.NRGBA{R: 200, G: 200, B: 200, A: 255})
	palette[len(pixels)] = b.At(1, 1)
	pa, pb := image.NewPaletted(rect, palette), image.NewPaletted(rect, palette)
	for i := range pixels {
		pa.SetColorIndex(i%3, i/3, uint8(i))
		pb.SetColorIndex(i%3, i/3, uint8(i))
	}
	pb.SetColorIndex(1, 1, uint8(len(pixels)))
	for _, pair := range [][2]image.Image{{a, b}, {wrappedImage{a}, wrappedImage{b}}, {pa, pb}} {
		var out image.Image
		diff, err := MatchPixel(pair[0], pair[1], WriteTo(&out))
		if err != nil || diff != 1 {
			t.Errorf("%T: MatchPixel() = %d, %v; want 1, nil", pair[0], diff, err)
		}
		if got := color.RGBAModel.Convert(out.At(1, 1)); got != (color.RGBA{R: 255, A: 255}) {
			t.Errorf("%T: center = %v; want opaque red", pair[0], got)
		}
	}
}

func TestMatchPixel16BitPrecision(t *testing.T) {
	for _, typ := range imageTypes {
		if typ.name != "RGBA64" && typ.name != "NRGBA64" && typ.name != "Gray16" {
			continue
		}
		t.Run(typ.name, func(t *testing.T) {
			rect := image.Rect(0, 0, 1, 1)
			a, b := typ.new(rect), typ.new(rect)
			a.Set(0, 0, color.Gray16{Y: 0xfffe})
			b.Set(0, 0, color.Gray16{Y: 0xffff})
			if diff, err := MatchPixel(a, b, Threshold(0)); err != nil || diff != 1 {
				t.Fatalf("MatchPixel() = %d, %v; want 1, nil", diff, err)
			}
		})
	}
}
