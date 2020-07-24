/*
 * Copyright (c) 2011 Matt Jibson <matt.jibson@gmail.com>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

// Package fft provides forward and inverse fast Fourier transform functions.
package fft

import (
	"github.com/mjibson/go-dsp/dsputils"
)

// FFTReal returns the forward FFT of the real-valued slice.
func FFTReal(x []float64) []complex128 {
	return FFT(dsputils.ToComplex(x))
}

// IFFTReal returns the inverse FFT of the real-valued slice.
func IFFTReal(x []float64) []complex128 {
	return IFFT(dsputils.ToComplex(x))
}

// IFFT returns the inverse FFT of the complex-valued slice.
func IFFT(x []complex128) []complex128 {
	lx := len(x)
	r := make([]complex128, lx)

	// Reverse inputs, which is calculated with modulo N, hence x[0] as an outlier
	r[0] = x[0]
	for i := 1; i < lx; i++ {
		r[i] = x[lx-i]
	}

	r = FFT(r)

	N := complex(float64(lx), 0)
	for n := range r {
		r[n] /= N
	}
	return r
}

// Convolve returns the convolution of x ∗ y.
func Convolve(x, y []complex128) []complex128 {
	if len(x) != len(y) {
		panic("arrays not of equal size")
	}

	fft_x := FFT(x)
	fft_y := FFT(y)

	r := make([]complex128, len(x))
	for i := 0; i < len(r); i++ {
		r[i] = fft_x[i] * fft_y[i]
	}

	return IFFT(r)
}

// FFT returns the forward FFT of the complex-valued slice.
func FFT(x []complex128) []complex128 {
	lx := len(x)

	// todo: non-hack handling length <= 1 cases
	if lx <= 1 {
		r := make([]complex128, lx)
		copy(r, x)
		return r
	}

	if dsputils.IsPowerOf2(lx) {
		return radix2FFT(x)
	}

	return bluesteinFFT(x)
}

var (
	worker_pool_size = 0
)

// SetWorkerPoolSize sets the number of workers during FFT computation on multicore systems.
// If n is 0 (the default), then GOMAXPROCS workers will be created.
func SetWorkerPoolSize(n int) {
	if n < 0 {
		n = 0
	}

	worker_pool_size = n
}

// FFT2Real returns the 2-dimensional, forward FFT of the real-valued matrix.
func FFT2Real(x [][]float64) [][]complex128 {
	return FFT2(dsputils.ToComplex2(x))
}

// FFT2 returns the 2-dimensional, forward FFT of the complex-valued matrix.
func FFT2(x [][]complex128) [][]complex128 {
	return computeFFT2(x, FFT)
}

// IFFT2Real returns the 2-dimensional, inverse FFT of the real-valued matrix.
func IFFT2Real(x [][]float64) [][]complex128 {
	return IFFT2(dsputils.ToComplex2(x))
}

// IFFT2 returns the 2-dimensional, inverse FFT of the complex-valued matrix.
func IFFT2(x [][]complex128) [][]complex128 {
	return computeFFT2(x, IFFT)
}

func computeFFT2(x [][]complex128, fftFunc func([]complex128) []complex128) [][]complex128 {
	rows := len(x)
	if rows == 0 {
		panic("empty input array")
	}

	cols := len(x[0])
	r := make([][]complex128, rows)
	for i := 0; i < rows; i++ {
		if len(x[i]) != cols {
			panic("ragged input array")
		}
		r[i] = make([]complex128, cols)
	}

	for i := 0; i < cols; i++ {
		t := make([]complex128, rows)
		for j := 0; j < rows; j++ {
			t[j] = x[j][i]
		}

		for n, v := range fftFunc(t) {
			r[n][i] = v
		}
	}

	for n, v := range r {
		r[n] = fftFunc(v)
	}

	return r
}

// FFTN returns the forward FFT of the matrix m, computed in all N dimensions.
func FFTN(m *dsputils.Matrix) *dsputils.Matrix {
	return computeFFTN(m, FFT)
}

// IFFTN returns the forward FFT of the matrix m, computed in all N dimensions.
func IFFTN(m *dsputils.Matrix) *dsputils.Matrix {
	return computeFFTN(m, IFFT)
}

func computeFFTN(m *dsputils.Matrix, fftFunc func([]complex128) []complex128) *dsputils.Matrix {
	dims := m.Dimensions()
	t := m.Copy()
	r := dsputils.MakeEmptyMatrix(dims)

	for n := range dims {
		dims[n] -= 1
	}

	for n := range dims {
		d := make([]int, len(dims))
		copy(d, dims)
		d[n] = -1

		for {
			r.SetDim(fftFunc(t.Dim(d)), d)

			if !decrDim(d, dims) {
				break
			}
		}

		r, t = t, r
	}

	return t
}

// decrDim decrements an element of x by 1, skipping all -1s, and wrapping up to d.
// If a value is 0, it will be reset to its correspending value in d, and will carry one from the next non -1 value to the right.
// Returns true if decremented, else false.
func decrDim(x, d []int) bool {
	for n, v := range x {
		if v == -1 {
			continue
		} else if v == 0 {
			i := n
			// find the next element to decrement
			for ; i < len(x); i++ {
				if x[i] == -1 {
					continue
				} else if x[i] == 0 {
					x[i] = d[i]
				} else {
					x[i] -= 1
					return true
				}
			}

			// no decrement
			return false
		} else {
			x[n] -= 1
			return true
		}
	}

	return false
}
