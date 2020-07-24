package asap

import (
	"math"
)

// Smooth performs ASAP smoothing
func Smooth(in []float64, resolution int) []float64 {
	step := len(in) / resolution
	data := sma(in, step, step)
	peaks, correlations, maxAcf := acf(data, int(float64(len(data)+5)/10))
	m := newMetrics(data)
	origKurt := m.kurtosis
	minObj := m.roughness
	windowSize := 1
	lb := 1
	leastFeasible := -1

	tail := len(data) / 10

	for i := len(peaks) - 1; i >= 0; i-- {
		w := peaks[i]
		if w < lb || w == 1 {
			break
		} else if math.Sqrt(1-correlations[w])*float64(windowSize) > math.Sqrt(1-correlations[windowSize])*float64(w) {
			continue
		}

		smoothed := sma(data, w, 1)
		m2 := newMetrics(smoothed)
		if m2.kurtosis >= origKurt {
			if m2.roughness < minObj {
				minObj = m2.roughness
				windowSize = w
			}
			t := int(float64(w) * math.Sqrt((maxAcf-1)/(correlations[w]-1)))
			if t > lb {
				lb = t
			}
			if leastFeasible < 0 {
				leastFeasible = i
			}
		}
	}
	if leastFeasible > 0 {
		if leastFeasible < len(peaks)-3 {
			tail = peaks[leastFeasible+1]
		}
		if peaks[leastFeasible+1] > lb {
			lb = peaks[leastFeasible+1]
		}
	}

	windowSize = binarySearch(lb, tail, data, minObj, origKurt, windowSize)
	return sma(data, windowSize, 1)
}

func binarySearch(head, tail int, in []float64, minObj, origKurt float64, windowSize int) int {
	for head <= tail {
		w := (head + tail + 1) / 2
		smoothed := sma(in, w, 1)
		m := newMetrics(smoothed)
		if m.kurtosis >= origKurt {
			if m.roughness < minObj {
				windowSize = w
				minObj = m.roughness
			}
			head = w + 1
		} else {
			tail = w - 1
		}

	}
	return windowSize
}

// simple moving average
func sma(in []float64, rng, slide int) []float64 {
	var (
		s           float64
		c           float64
		windowStart int
		oldStart    int
		ret         []float64
	)
	last := len(in) - 1
	for i, v := range in {
		if i-windowStart >= rng || i == last {
			if i == last || c == 0 {
				s += v
				c += 1
			}
			ret = append(ret, s/c)
			oldStart = windowStart
			for windowStart < len(in) && windowStart-oldStart < slide {
				s -= in[windowStart]
				c -= 1
				windowStart += 1
			}
		}
		s += v
		c += 1
	}
	return ret
}
