package pixelmatch

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestIdenticalOutput(t *testing.T) {
	for _, mask := range []bool{false, true} {
		name := "background"
		if mask {
			name = "mask"
		}
		t.Run(name, func(t *testing.T) {
			a := image.NewRGBA(image.Rect(0, 0, 2, 2))
			var out image.Image
			opts := []MatchOption{WriteTo(&out)}
			if mask {
				opts = append(opts, EnableDiffMask)
			}
			count, err := MatchPixel(a, a, opts...)
			if err != nil || count != 0 {
				t.Fatalf("MatchPixel = %d, %v", count, err)
			}
			if out == nil {
				t.Fatal("WriteTo output is nil for identical images")
			}
			if out.Bounds() != a.Bounds() {
				t.Fatalf("output bounds = %v, want %v", out.Bounds(), a.Bounds())
			}
			want := color.RGBA{255, 255, 255, 255}
			if mask {
				want = color.RGBA{}
			}
			for y := 0; y < 2; y++ {
				for x := 0; x < 2; x++ {
					if got := out.At(x, y); got != want {
						t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, want)
					}
				}
			}
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, out); err != nil {
				t.Fatal(err)
			}
		})
	}
}
