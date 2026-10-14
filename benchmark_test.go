package pixelmatch

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkMatchPixel(b *testing.B) {
	const width, height = 512, 512
	rect := image.Rect(0, 0, width, height)
	a := image.NewNRGBA(rect)
	identical := image.NewNRGBA(rect)
	sparse := image.NewNRGBA(rect)
	dense := image.NewNRGBA(rect)
	transparentA, transparentB := image.NewNRGBA(rect), image.NewNRGBA(rect)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255}
			a.SetNRGBA(x, y, c)
			dense.SetNRGBA(x, y, color.NRGBA{R: 255 - c.R, G: 255 - c.G, B: 255 - c.B, A: 255})
			c.A = 128
			transparentA.SetNRGBA(x, y, c)
			transparentB.SetNRGBA(x, y, color.NRGBA{R: 255 - c.R, G: 255 - c.G, B: 255 - c.B, A: 192})
		}
	}
	copy(identical.Pix, a.Pix)
	copy(sparse.Pix, a.Pix)
	for y := 0; y < height; y += 32 {
		for x := 0; x < width; x += 32 {
			sparse.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	for _, tc := range []struct {
		name string
		a, b image.Image
		opts []MatchOption
	}{
		{"identical", a, identical, nil},
		{"sparse", a, sparse, nil},
		{"dense", a, dense, nil},
		{"dense/includeAA", a, dense, []MatchOption{IncludeAntiAlias}},
		{"transparent", transparentA, transparentB, nil},
		{"sparse/window", a, sparse, []MatchOption{WindowSize(32)}},
		{"dense/window", a, dense, []MatchOption{WindowSize(32)}},
		{"cropped", a.SubImage(image.Rect(3, 2, width-1, height)), sparse.SubImage(image.Rect(3, 2, width-1, height)), nil},
	} {
		b.Run(tc.name, func(b *testing.B) {
			benchmarkMatchPixel(b, tc.a, tc.b, tc.opts)
		})
	}
}

func BenchmarkMatchPixelPNG(b *testing.B) {
	for _, name := range []string{"01_matching", "02_nonmatching", "03_nonmatching_strideskip"} {
		b.Run(name, func(b *testing.B) {
			read := func(suffix string) image.Image {
				f, err := os.Open(filepath.Join("testdata", name+suffix+".png"))
				if err != nil {
					b.Fatal(err)
				}
				defer f.Close()
				img, err := png.Decode(f)
				if err != nil {
					b.Fatal(err)
				}
				return img
			}
			benchmarkMatchPixel(b, read("_a"), read("_b"), nil)
		})
	}
}

func benchmarkMatchPixel(b *testing.B, a, c image.Image, opts []MatchOption) {
	for _, output := range []bool{false, true} {
		name := "count"
		if output {
			name = "output"
		}
		b.Run(name, func(b *testing.B) {
			var out image.Image
			opts := append([]MatchOption(nil), opts...)
			if output {
				opts = append(opts, WriteTo(&out))
			}
			want, err := MatchPixel(a, c, opts...)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(a.Bounds().Dx() * a.Bounds().Dy() * 8))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got, err := MatchPixel(a, c, opts...); err != nil || got != want {
					b.Fatalf("MatchPixel = %d, %v; want %d", got, err, want)
				}
			}
		})
	}
}
