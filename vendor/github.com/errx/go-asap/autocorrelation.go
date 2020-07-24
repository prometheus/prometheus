package asap

import (
	"math"
	"github.com/mjibson/go-dsp/fft"
)

const acfCorrThreshold = 0.2

func acf(in []float64, maxLag int) ([]int, []float64, float64) {
	pow := float64(int(math.Log2(float64(len(in))) + 1))
	l := int(math.Pow(2.0, pow))

	fftv := make([]float64, len(in)+(l-len(in)))
	copy(fftv, in)
	if len(fftv) != l {
		panic("fftv wtf")
	}
	f_f := fft.FFTReal(fftv)
	s_f := make([]float64, len(fftv))
	for i, x := range f_f {
		s_f[i] = real(x)*real(x) + imag(x)*imag(x)
	}
	r_t := fft.IFFTReal(s_f)

	correlations := make([]float64, maxLag)
	r_t0 := real(r_t[0])
	for i := range correlations {
		if i < 1 {
			continue
		}
		correlations[i] = real(r_t[i]) / r_t0
	}

	var peaks []int
	var maxAcf float64
	if len(correlations) < 2 {
		return peaks, correlations, maxAcf
	}

	positive := correlations[1] > correlations[0]
	max := 1
	for i := range correlations {
		if i < 2 {
			continue
		}
		if !positive && correlations[i] > correlations[i-1] {
			max = i
			positive = !positive
		} else if positive && correlations[i] > correlations[max] {
			max = i
		} else if positive && correlations[i] < correlations[i-1] {
			if max > 1 && correlations[max] > acfCorrThreshold {
				peaks = append(peaks, max)
			}
			if correlations[max] > maxAcf {
				maxAcf = correlations[max]
			}
			positive = !positive
		}
	}
	return peaks, correlations, maxAcf
}
