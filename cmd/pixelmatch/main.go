package main

import (
	"flag"
	"fmt"
	"github.com/orisano/pixelmatch"
	"github.com/pkg/errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
)

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
	_, err = pixelmatch.MatchPixel(img1, img2, pixelmatch.Threshold(*threshold), pixelmatch.WriteTo(&out))
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
