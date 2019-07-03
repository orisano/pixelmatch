package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"

	"github.com/orisano/pixelmatch"
)

type colorValue color.RGBA

func (c *colorValue) String() string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func (c *colorValue) Set(s string) error {
	r, err := strconv.ParseUint(s[1:3], 16, 8)
	if err != nil {
		return err
	}
	g, err := strconv.ParseUint(s[3:5], 16, 8)
	if err != nil {
		return err
	}
	b, err := strconv.ParseUint(s[5:7], 16, 8)
	if err != nil {
		return err
	}
	a, err := strconv.ParseUint(s[7:9], 16, 8)
	if err != nil {
		return err
	}
	*c = colorValue(color.RGBA{
		R: uint8(r),
		G: uint8(g),
		B: uint8(b),
		A: uint8(a),
	})
	return nil
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("pixelmatch: ")

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	threshold := flag.Float64("threshold", 0.1, "threshold")
	dest := flag.String("dest", "-", "destination path")
	aa := flag.Bool("aa", false, "ignore anti alias pixel")
	alpha := flag.Float64("alpha", 0.1, "alpha")
	antiAliased := colorValue(color.RGBA{R: 255, G: 255})
	flag.Var(&antiAliased, "aacolor", "anti aliased color")
	diffColor := colorValue(color.RGBA{R: 255})
	flag.Var(&diffColor, "diffcolor", "diff color")

	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		fmt.Println("Usage of pixelmatch [flags] image1 image2 :")
		flag.PrintDefaults()
		os.Exit(2)
	}
	img1, err := openImage(args[0])
	if err != nil {
		return errors.Wrapf(err, "failed to open image(path=%v)", args[0])
	}
	img2, err := openImage(args[1])
	if err != nil {
		return errors.Wrapf(err, "failed to open image(path=%v)", args[1])
	}

	var out image.Image
	opts := []pixelmatch.MatchOption{
		pixelmatch.Threshold(*threshold),
		pixelmatch.Alpha(*alpha),
		pixelmatch.AntiAliasedColor(color.RGBA(antiAliased)),
		pixelmatch.DiffColor(color.RGBA(diffColor)),
		pixelmatch.WriteTo(&out),
	}
	if *aa {
		opts = append(opts, pixelmatch.IncludeAntiAlias)
	}

	_, err = pixelmatch.MatchPixel(img1, img2, opts...)
	if err != nil {
		return errors.Wrap(err, "failed to match pixel")
	}

	format := "png"
	var w io.Writer
	if *dest == "-" {
		w = os.Stdout
	} else {
		switch ext := filepath.Ext(*dest); ext {
		case ".png":
			format = "png"
		case ".jpeg", ".jpg":
			format = "jpeg"
		default:
			return errors.Errorf("unsupported format: %v", ext)
		}
		f, err := os.Create(*dest)
		if err != nil {
			return errors.Wrap(err, "failed to create destination image")
		}
		defer f.Close()
		w = f
	}

	var encErr error
	switch format {
	case "png":
		encErr = png.Encode(w, out)
	case "jpeg":
		encErr = jpeg.Encode(w, out, nil)
	}
	if encErr != nil {
		return errors.Wrap(encErr, "failed to encode")
	}
	return nil
}

func openImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open")
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode image")
	}
	return img, nil
}
