package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	dir, err := ioutil.TempDir("", "pixelmatch-cli-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	writeImage := func(name string, img image.Image) string {
		var data bytes.Buffer
		if err := png.Encode(&data, img); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name+".png")
		if err := ioutil.WriteFile(path, data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	black, white := image.NewNRGBA(image.Rect(0, 0, 4, 4)), image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			black.SetNRGBA(x, y, color.NRGBA{A: 255})
			white.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	blackPath, whitePath := writeImage("black", black), writeImage("white", white)
	small := writeImage("small", image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	alpha, gray := image.NewNRGBA(image.Rect(0, 0, 1, 1)), image.NewNRGBA(image.Rect(0, 0, 1, 1))
	alpha.SetNRGBA(0, 0, color.NRGBA{A: 128})
	gray.SetNRGBA(0, 0, color.NRGBA{R: 127, G: 127, B: 127, A: 255})
	alphaPath, grayPath := writeImage("alpha", alpha), writeImage("gray", gray)
	rampA, rampB := image.NewNRGBA(image.Rect(0, 0, 3, 3)), image.NewNRGBA(image.Rect(0, 0, 3, 3))
	for y := 0; y < 3; y++ {
		for x, v := range []uint8{0, 128, 255} {
			rampA.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
			if x == 1 {
				v = 192
			}
			rampB.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	rampAPath, rampBPath := writeImage("ramp-a", rampA), writeImage("ramp-b", rampB)
	ignorePath := filepath.Join(dir, "ignore.bin")
	if err := ioutil.WriteFile(ignorePath, bytes.Repeat([]byte{1}, 16), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		code int
		text string
	}{
		{"usage", nil, 64, ""},
		{"help", []string{"-h"}, 0, ""},
		{"identical", []string{blackPath, blackPath}, 0, "different pixels: 0\nerror: 0%\n"},
		{"different", []string{blackPath, whitePath}, 66, "different pixels: 16\nerror: 100%\n"},
		{"dimensions", []string{blackPath, small}, 65, "Image dimensions do not match: 4x4 vs 1x1\n"},
		{"threshold positional", []string{blackPath, whitePath, "", "1.1"}, 0, "different pixels: 0\n"},
		{"threshold flag", []string{"-threshold", "1.1", blackPath, whitePath}, 0, "different pixels: 0\n"},
		{"checkerboard default", []string{alphaPath, grayPath}, 66, "different pixels: 1\n"},
		{"white background", []string{"-checkerboard=false", alphaPath, grayPath}, 0, "different pixels: 0\n"},
		{"window", []string{"-windowsize", "2", blackPath, whitePath}, 66, "different pixels: 4\n"},
		{"ignore", []string{"-ignoremask", ignorePath, blackPath, whitePath}, 0, "different pixels: 0\n"},
		{"exclude aa positional", []string{rampAPath, rampBPath, "", "0.1", "false"}, 66, "different pixels: 2\n"},
		{"include aa positional", []string{rampAPath, rampBPath, "", "0.1", "true"}, 66, "different pixels: 3\n"},
		{"include aa flag", []string{"-aa", rampAPath, rampBPath}, 66, "different pixels: 3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code, err := run(tc.args, &stdout, &stderr)
			if err != nil || code != tc.code {
				t.Fatalf("run = %d, %v; want %d, nil", code, err, tc.code)
			}
			if !strings.Contains(stdout.String(), tc.text) {
				t.Fatalf("stdout = %q; want %q", stdout.String(), tc.text)
			}
			if tc.name == "usage" || tc.name == "help" {
				if !strings.Contains(stderr.String(), "[diff.png] [threshold] [includeAA]") {
					t.Fatal("missing positional usage")
				}
			}
		})
	}
	for _, args := range [][]string{{blackPath, whitePath, "", "invalid"}, {"-diffcolor", "#ff", blackPath, whitePath}} {
		var stdout, stderr bytes.Buffer
		if code, err := run(args, &stdout, &stderr); code != 64 || err == nil {
			t.Fatalf("invalid args: got %d, %v", code, err)
		}
	}
	t.Run("positional diff output", func(t *testing.T) {
		path := filepath.Join(dir, "diff.png")
		var stdout, stderr bytes.Buffer
		if code, err := run([]string{blackPath, whitePath, path}, &stdout, &stderr); code != 66 || err != nil {
			t.Fatalf("run = %d, %v", code, err)
		}
		img, err := openImage(path)
		if err != nil || img.Bounds() != black.Bounds() {
			t.Fatalf("diff image: %v", err)
		}
		if got := color.RGBAModel.Convert(img.At(0, 0)); got != (color.RGBA{R: 255, A: 255}) {
			t.Fatalf("diff pixel = %v", got)
		}
	})
	t.Run("stdout image", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code, err := run([]string{"-dest", "-", "-diffcolor", "#14283c", blackPath, whitePath}, &stdout, &stderr); code != 66 || err != nil {
			t.Fatalf("run = %d, %v", code, err)
		}
		img, err := png.Decode(&stdout)
		if err != nil {
			t.Fatal(err)
		}
		if got := color.RGBAModel.Convert(img.At(0, 0)); got != (color.RGBA{R: 20, G: 40, B: 60, A: 255}) {
			t.Fatalf("custom diff pixel = %v", got)
		}
		if !strings.Contains(stderr.String(), "different pixels: 16") {
			t.Fatal("report must go to stderr when stdout carries an image")
		}
	})
}

func TestColorValue(t *testing.T) {
	for _, s := range []string{"", "#", "#ff", "14283c", "x14283c", "#zz283c", "#14283czzz"} {
		var c colorValue
		if err := c.Set(s); err == nil {
			t.Errorf("Set(%q) must return an error", s)
		}
	}
	for _, s := range []string{"#14283c", "#14283cff"} {
		var c colorValue
		if err := c.Set(s); err != nil || c.String() != "#14283cff" {
			t.Errorf("Set(%q) = %v, %v", s, c, err)
		}
	}
}
