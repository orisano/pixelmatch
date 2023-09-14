package pixelmatch

import (
	"errors"
	"image"
	"image/png"
	"os"
	"path"
	"testing"
)

func Test_Pixelmatch(t *testing.T) {
	type testcase struct {
		fNamePrefix string
		shouldMatch bool
	}

	for _, tc := range []testcase{
		{"01_matching", true},
		{"02_nonmatching", false},
		{"03_nonmatching_strideskip", false},
	} {
		t.Run(tc.fNamePrefix, func(t *testing.T) {

			pathA := path.Join("testdata", tc.fNamePrefix+"_a.png")
			pathB := path.Join("testdata", tc.fNamePrefix+"_b.png")

			readpng := func(path string) image.Image {

				f, err := os.Open(path)
				if errors.Is(err, os.ErrNotExist) {
					t.Fatalf("%s does not exist, please create it for this test to run: %v", path, err)
				} else if err != nil {
					t.Fatalf(err.Error())
				}

				img, err := png.Decode(f)
				if err != nil {
					t.Fatalf(err.Error())
				}

				return img
			}

			imgA := readpng(pathA)
			imgB := readpng(pathB)

			res, err := MatchPixel(imgA, imgB)
			if err != nil {
				t.Fatalf(err.Error())
			}

			if tc.shouldMatch {
				if res != 0 {
					t.Errorf("should have been 0 diff, but was %d", res)
				}
			} else {
				if res == 0 {
					t.Errorf("should have been >0 diff, but was %d", res)
				}
			}

		})
	}
}
