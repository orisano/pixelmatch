package pixelmatch

import "math"

// These lookup tables use pixelmatch v8.0.0's interpolation, including the
// exact cube root near black, so the same threshold produces the same result.
var linearRGB = func() [257]float64 {
	var table [257]float64
	for i := 0; i < 256; i++ {
		c := float64(i) / 255
		if c <= 0.04045 {
			table[i] = c / 12.92
		} else {
			table[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	table[256] = table[255]
	return table
}()

var cubeRoots = func() [4098]float64 {
	var table [4098]float64
	for i := range table {
		table[i] = math.Cbrt(float64(i) / 4096)
	}
	return table
}()

func linearChannel(c float64) float64 {
	i := int(c)
	return linearRGB[i] + (linearRGB[i+1]-linearRGB[i])*(c-float64(i))
}

func cubeRoot(c float64) float64 {
	t := c * 4096
	i := int(t)
	if i < 8 {
		return math.Cbrt(c)
	}
	return cubeRoots[i] + (cubeRoots[i+1]-cubeRoots[i])*(t-float64(i))
}

func colorLightness(l, m, s float64) float64 {
	const k1, k2 = 0.206, 0.03
	const k3 = (1 + k1) / (1 + k2)
	L := 0.2104542553*l + 0.7936177850*m - 0.0040720468*s
	x := k3*L - k1
	return 0.5 * (x + math.Sqrt(x*x+4*k2*k3*L))
}

func colorLMS(r, g, b float64) (l, m, s float64) {
	r, g, b = linearChannel(r), linearChannel(g), linearChannel(b)
	l = cubeRoot(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m = cubeRoot(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s = cubeRoot(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return
}

// colorDelta returns 0 when the OKLab HyAB distance is within the threshold,
// -1 when the second pixel is darker, and +1 for other mismatches.
func colorDelta(a, b *rgba, index int, checkerboard bool, threshold float64) int {
	if *a == *b {
		return 0
	}
	ar, ag, ab, aa := rgbaFromColor(a)
	br, bg, bb, ba := rgbaFromColor(b)
	if aa < 255 || ba < 255 {
		r, g, blue := 255.0, 255.0, 255.0
		if checkerboard {
			h := uint32(index) * 0x9e3779b1
			r = float64(48 + 159*(h>>31))
			g = float64(48 + 159*((h>>30)&1))
			blue = float64(48 + 159*((h>>29)&1))
		}
		ar = (ar*aa + r*(255-aa)) / 255
		ag = (ag*aa + g*(255-aa)) / 255
		ab = (ab*aa + blue*(255-aa)) / 255
		br = (br*ba + r*(255-ba)) / 255
		bg = (bg*ba + g*(255-ba)) / 255
		bb = (bb*ba + blue*(255-ba)) / 255
	}
	al, am, as := colorLMS(ar, ag, ab)
	bl, bm, bs := colorLMS(br, bg, bb)
	dL := colorLightness(al, am, as) - colorLightness(bl, bm, bs)
	rest := threshold - math.Abs(dL)
	if rest >= 0 {
		dl, dm, ds := al-bl, am-bm, as-bs
		da := 1.9779984951*dl - 2.4285922050*dm + 0.4505937099*ds
		db := 0.0259040371*dl + 0.7827717662*dm - 0.8086757660*ds
		if da*da+db*db <= rest*rest {
			return 0
		}
	}
	if dL > 0 {
		return -1
	}
	return 1
}

// AA detection uses Rec.601 luma over white, independently of checkerboard.
// Alpha breaks ties when the composited brightness is exactly equal.
func brightnessDelta(a, b *rgba) float64 {
	if *a == *b {
		return 0
	}
	ar, ag, ab, aa := rgbaFromColor(a)
	br, bg, bb, ba := rgbaFromColor(b)
	dr, dg, db, da := ar-br, ag-bg, ab-bb, aa-ba
	if aa < 255 || ba < 255 {
		dr = (ar*aa - br*ba - 255*da) / 255
		dg = (ag*aa - bg*ba - 255*da) / 255
		db = (ab*aa - bb*ba - 255*da) / 255
		d := rgbaToY(dr, dg, db)
		if d == 0 && da != 0 {
			return da / 2
		}
		return d
	}
	return rgbaToY(dr, dg, db)
}
