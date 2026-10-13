package pixelmatch

import "math"

func maxWindowDiff(mask []byte, width, height int, size float64) int {
	n := int(math.Min(math.Max(math.Floor(size), 1), float64(minInt(width, height))))
	columns := make([]int, width)
	maxCount, bandTotal := 0, 0
	for y := 0; y < height; y++ {
		for x := range columns {
			d := int(mask[y*width+x])
			if y >= n {
				d -= int(mask[(y-n)*width+x])
			}
			columns[x] += d
			bandTotal += d
		}
		if y < n-1 || bandTotal <= maxCount {
			continue
		}
		count := 0
		for x, col := range columns {
			count += col
			if x >= n {
				count -= columns[x-n]
			}
			if x >= n-1 && count > maxCount {
				maxCount = count
			}
		}
	}
	return maxCount
}
