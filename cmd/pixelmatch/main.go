package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/orisano/pixelmatch"
)

type colorValue color.RGBA

func (c *colorValue) String() string {
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

func (c *colorValue) Set(s string) error {
	if (len(s) != 7 && len(s) != 9) || s[0] != '#' {
		return fmt.Errorf("color must be #RRGGBB or #RRGGBBAA")
	}
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
	a := uint64(255)
	if len(s) == 9 {
		a, err = strconv.ParseUint(s[7:9], 16, 8)
		if err != nil {
			return err
		}
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

	code, err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		log.Print(err)
	}
	os.Exit(code)
}

func run(argv []string, stdout, stderr io.Writer) (int, error) {
	flags := flag.NewFlagSet("pixelmatch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: pixelmatch [flags] image1.png image2.png [diff.png] [threshold] [includeAA]")
		flags.PrintDefaults()
	}
	threshold := flags.Float64("threshold", 0.1, "maximum OKLab HyAB color distance")
	dest := flags.String("dest", "", "destination path (- writes the image to stdout)")
	aa := flags.Bool("aa", false, "count anti-aliased pixels as differences")
	alpha := flags.Float64("alpha", 0.1, "opacity of unchanged pixels in diff output")
	checkerboard := flags.Bool("checkerboard", true, "compare semi-transparent pixels over a checkerboard")
	windowSize := flags.Float64("windowsize", math.Inf(1), "return the highest diff count in an N by N window")
	diffMask := flags.Bool("diffmask", false, "draw differences over a transparent background")
	ignorePath := flags.String("ignoremask", "", "raw ignore mask file with one byte per image pixel")
	antiAliased := colorValue(color.RGBA{R: 255, G: 255})
	flags.Var(&antiAliased, "aacolor", "anti-aliased pixel color (#RRGGBB)")
	diffColor := colorValue(color.RGBA{R: 255})
	flags.Var(&diffColor, "diffcolor", "diff pixel color (#RRGGBB)")
	var diffColorAlt colorValue
	flags.Var(&diffColorAlt, "diffcoloralt", "color for pixels darker in the second image (#RRGGBB)")
	if err := flags.Parse(argv); err != nil {
		if err == flag.ErrHelp {
			return 0, nil
		}
		return 64, err
	}
	args := flags.Args()
	if len(args) < 2 || len(args) > 5 {
		flags.Usage()
		return 64, nil
	}
	if len(args) >= 3 {
		*dest = args[2]
	}
	if len(args) >= 4 {
		v, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			return 64, fmt.Errorf("invalid threshold: %w", err)
		}
		*threshold = v
	}
	if len(args) >= 5 {
		*aa = args[4] != "false"
	}
	img1, err := openImage(args[0])
	if err != nil {
		return 1, fmt.Errorf("open image(path=%v): %w", args[0], err)
	}
	img2, err := openImage(args[1])
	if err != nil {
		return 1, fmt.Errorf("open image(path=%v): %w", args[1], err)
	}
	if img1.Bounds().Size() != img2.Bounds().Size() {
		fmt.Fprintf(stdout, "Image dimensions do not match: %dx%d vs %dx%d\n", img1.Bounds().Dx(), img1.Bounds().Dy(), img2.Bounds().Dx(), img2.Bounds().Dy())
		return 65, nil
	}

	var out image.Image
	opts := []pixelmatch.MatchOption{
		pixelmatch.Threshold(*threshold),
		pixelmatch.Alpha(*alpha),
		pixelmatch.AntiAliasedColor(color.RGBA(antiAliased)),
		pixelmatch.DiffColor(color.RGBA(diffColor)),
		pixelmatch.Checkerboard(*checkerboard),
		pixelmatch.WindowSize(*windowSize),
	}
	if *dest != "" {
		opts = append(opts, pixelmatch.WriteTo(&out))
	}
	if *diffMask {
		opts = append(opts, pixelmatch.EnableDiffMask)
	}
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "diffcoloralt" {
			opts = append(opts, pixelmatch.DiffColorAlt(color.RGBA(diffColorAlt)))
		}
	})
	if *ignorePath != "" {
		mask, err := ioutil.ReadFile(*ignorePath)
		if err != nil {
			return 1, fmt.Errorf("read ignore mask: %w", err)
		}
		opts = append(opts, pixelmatch.IgnoreMask(mask))
	}
	if *aa {
		opts = append(opts, pixelmatch.IncludeAntiAlias)
	}

	diff, err := pixelmatch.MatchPixel(img1, img2, opts...)
	if err != nil {
		return 1, fmt.Errorf("match pixel: %w", err)
	}
	report := stdout
	if *dest == "-" {
		report = stderr
	}
	fmt.Fprintf(report, "different pixels: %d\n", diff)
	fmt.Fprintf(report, "error: %g%%\n", math.Round(10000*float64(diff)/float64(img1.Bounds().Dx()*img1.Bounds().Dy()))/100)
	code := 0
	if diff != 0 {
		code = 66
	}
	if *dest == "" {
		return code, nil
	}

	format := "png"
	var w io.Writer
	if *dest == "-" {
		w = stdout
	} else {
		switch ext := filepath.Ext(*dest); ext {
		case ".png":
			format = "png"
		case ".jpeg", ".jpg":
			format = "jpeg"
		default:
			return 1, fmt.Errorf("unsupported format(%v)", ext)
		}
		f, err := os.Create(*dest)
		if err != nil {
			return 1, fmt.Errorf("create destination image: %w", err)
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
		return 1, fmt.Errorf("encode: %w", encErr)
	}
	return code, nil
}

func openImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}
