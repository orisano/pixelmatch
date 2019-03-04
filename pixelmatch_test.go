package pixelmatch

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkMatchPixel(b *testing.B) {
	img1 := readImage(b, "1a.png")
	img2 := readImage(b, "1b.png")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		MatchPixel(img1, img2)
	}
}

func readImage(b *testing.B, name string) image.Image {
	b.Helper()
	f, err := os.Open(filepath.Join("fixtures", name))
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		b.Fatal(err)
	}
	return img
}
