package pixelmatch

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"io/ioutil"
	"math"
	"testing"
)

func TestUpstreamV8(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/pixelmatch-v8.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Version string
		Cases   []struct {
			Name          string
			Width, Height int
			A, B, Output  []byte
			Diff          int
			Options       struct {
				Threshold, Alpha, WindowSize     *float64
				Checkerboard                     *bool
				IncludeAA, DiffMask              bool
				AAColor, DiffColor, DiffColorAlt []byte
				IgnoreMask                       []byte
			}
		}
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if fixtures.Version != "8.0.0" || len(fixtures.Cases) == 0 {
		t.Fatal("missing pixelmatch v8.0.0 reference cases")
	}
	for _, tc := range fixtures.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			opts := []MatchOption{IgnoreMask(tc.Options.IgnoreMask)}
			if v := tc.Options.Threshold; v != nil {
				opts = append(opts, Threshold(*v))
			}
			if v := tc.Options.Alpha; v != nil {
				opts = append(opts, Alpha(*v))
			}
			if v := tc.Options.WindowSize; v != nil {
				opts = append(opts, WindowSize(*v))
			}
			if v := tc.Options.Checkerboard; v != nil {
				opts = append(opts, Checkerboard(*v))
			}
			if tc.Options.IncludeAA {
				opts = append(opts, IncludeAntiAlias)
			}
			if tc.Options.DiffMask {
				opts = append(opts, EnableDiffMask)
			}
			for i, c := range [][]byte{tc.Options.AAColor, tc.Options.DiffColor, tc.Options.DiffColorAlt} {
				if c != nil {
					marker := color.NRGBA{R: c[0], G: c[1], B: c[2], A: 255}
					opts = append(opts, []func(color.Color) MatchOption{AntiAliasedColor, DiffColor, DiffColorAlt}[i](marker))
				}
			}
			// Different origins and parent strides must not affect checkerboard
			// hashes, sliding windows, mask indices, or AA neighbor coordinates.
			makeImage := func(data []byte, origin image.Point, cropped bool) image.Image {
				rect := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(tc.Width, tc.Height))}
				img := image.NewNRGBA(rect)
				if cropped {
					img = image.NewNRGBA(rect.Inset(-2)).SubImage(rect).(*image.NRGBA)
				}
				for y := 0; y < tc.Height; y++ {
					copy(img.Pix[img.PixOffset(rect.Min.X, rect.Min.Y+y):], data[y*tc.Width*4:(y+1)*tc.Width*4])
				}
				return img
			}
			for _, cropped := range []bool{false, true} {
				for _, wrapped := range []bool{false, true} {
					a := makeImage(tc.A, image.Pt(-3, 2), cropped)
					b := makeImage(tc.B, image.Pt(4, -5), cropped)
					if wrapped {
						a, b = wrappedImage{a}, wrappedImage{b}
					}
					var out image.Image
					withOutput := append(append([]MatchOption(nil), opts...), WriteTo(&out))
					if diff, err := MatchPixel(a, b, withOutput...); err != nil || diff != tc.Diff {
						t.Fatalf("cropped=%v wrapped=%v: got %d, %v; want %d", cropped, wrapped, diff, err, tc.Diff)
					}
					if out == nil || out.Bounds() != a.Bounds() {
						t.Fatal("incorrect output bounds")
					}
					if got := out.(*image.RGBA).Pix; !bytes.Equal(got, tc.Output) {
						t.Fatalf("cropped=%v wrapped=%v: diff image differs from upstream", cropped, wrapped)
					}
					if diff, err := MatchPixel(a, b, opts...); err != nil || diff != tc.Diff {
						t.Fatalf("without output: got %d, %v; want %d", diff, err, tc.Diff)
					}
				}
			}
		})
	}
}

func TestIgnoreMaskSize(t *testing.T) {
	a := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for _, mask := range [][]byte{{}, {1}, {0, 0, 0, 0, 0}} {
		if _, err := MatchPixel(a, a, IgnoreMask(mask)); err != ErrIgnoreMaskSizeNotMatch {
			t.Fatalf("mask length %d: got %v", len(mask), err)
		}
	}
	for _, mask := range [][]byte{nil, {0, 0, 0, 0}} {
		if diff, err := MatchPixel(a, a, IgnoreMask(mask)); err != nil || diff != 0 {
			t.Fatalf("valid mask: got %d, %v", diff, err)
		}
	}
}

func TestNonFiniteWindowSize(t *testing.T) {
	a, b := image.NewNRGBA(image.Rect(0, 0, 4, 4)), image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range b.Pix {
		b.Pix[i] = 255
	}
	for _, size := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if diff, err := MatchPixel(a, b, WindowSize(size), IncludeAntiAlias); err != nil || diff != 16 {
			t.Fatalf("window %v: got %d, %v; want 16", size, diff, err)
		}
	}
}
