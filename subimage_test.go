package pixelmatch

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestCroppedImages(t *testing.T) {
	constructors := []struct {
		name string
		new  func(image.Rectangle) draw.Image
	}{
		{"RGBA", func(r image.Rectangle) draw.Image { return image.NewRGBA(r) }},
		{"RGBA64", func(r image.Rectangle) draw.Image { return image.NewRGBA64(r) }},
		{"NRGBA", func(r image.Rectangle) draw.Image { return image.NewNRGBA(r) }},
		{"NRGBA64", func(r image.Rectangle) draw.Image { return image.NewNRGBA64(r) }},
		{"Gray", func(r image.Rectangle) draw.Image { return image.NewGray(r) }},
		{"Gray16", func(r image.Rectangle) draw.Image { return image.NewGray16(r) }},
	}
	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			// The crop reaches the parent's bottom edge and starts in the middle of a row.
			rect := image.Rect(2, 1, 5, 4)
			parent := tc.new(image.Rect(0, 0, 6, 4))
			draw.Draw(parent, parent.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
			a := parent.(interface {
				SubImage(image.Rectangle) image.Image
			}).SubImage(rect)
			parentB := tc.new(parent.Bounds())
			draw.Draw(parentB, parentB.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
			b := parentB.(interface {
				SubImage(image.Rectangle) image.Image
			}).SubImage(rect).(draw.Image)
			count, err := MatchPixel(a, b)
			if err != nil || count != 0 {
				t.Fatalf("equal crop: %d, %v", count, err)
			}
			// Exercise the anti-alias reader at nonzero coordinates, too.
			b.Set(4, 3, color.White)
			count, err = MatchPixel(a, b)
			if err != nil || count != 1 {
				t.Fatalf("different crop: %d, %v", count, err)
			}
		})
	}
}
