package asap

import "math"

type metrics struct {
	mean      float64
	std       float64
	roughness float64
	kurtosis  float64
}

func newMetrics(in []float64) *metrics {
	m := &metrics{}
	m.mean = mean(in)
	m.roughness = roughness(in)
	m.kurtosis = kurtosis(in, m.mean)
	return m
}

func mean(in []float64) float64 {
	var t float64
	for _, v := range in {
		t += v
	}
	return t / float64(len(in))
}

func std(in []float64, mean float64) float64 {
	var t float64
	for _, v := range in {
		diff := v - mean
		t += diff * diff
	}
	return math.Sqrt(t / float64(len(in)))
}

func kurtosis(in []float64, mean float64) float64 {
	var variance, u4 float64
	for _, v := range in {
		diff := v - mean
		variance += diff * diff
		u4 += diff * diff * diff * diff
	}
	return float64(len(in)) * u4 / (variance * variance)
}

func roughness(in []float64) float64 {
	diff := make([]float64, len(in)-1)
	var t float64
	for i := 1; i < len(in); i++ {
		diff[i-1] = in[i] - in[i-1]
		t += diff[i-1]
	}
	dm := t / float64(len(diff))
	return std(diff, dm)
}
