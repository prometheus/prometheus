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

// Package dsputils provides functions useful in digital signal processing.
package dsputils

import (
	"math"
)

// ToComplex returns the complex equivalent of the real-valued slice.
func ToComplex(x []float64) []complex128 {
	y := make([]complex128, len(x))
	for n, v := range x {
		y[n] = complex(v, 0)
	}
	return y
}

// IsPowerOf2 returns true if x is a power of 2, else false.
func IsPowerOf2(x int) bool {
	return x&(x-1) == 0
}

// NextPowerOf2 returns the next power of 2 >= x.
func NextPowerOf2(x int) int {
	if IsPowerOf2(x) {
		return x
	}

	return int(math.Pow(2, math.Ceil(math.Log2(float64(x)))))
}

// ZeroPad returns x with zeros appended to the end to the specified length.
// If len(x) >= length, x is returned, otherwise a new array is returned.
func ZeroPad(x []complex128, length int) []complex128 {
	if len(x) >= length {
		return x
	}

	r := make([]complex128, length)
	copy(r, x)
	return r
}

// ZeroPadF returns x with zeros appended to the end to the specified length.
// If len(x) >= length, x is returned, otherwise a new array is returned.
func ZeroPadF(x []float64, length int) []float64 {
	if len(x) >= length {
		return x
	}

	r := make([]float64, length)
	copy(r, x)
	return r
}

// ZeroPad2 returns ZeroPad of x, with the length as the next power of 2 >= len(x).
func ZeroPad2(x []complex128) []complex128 {
	return ZeroPad(x, NextPowerOf2(len(x)))
}

// ToComplex2 returns the complex equivalent of the real-valued matrix.
func ToComplex2(x [][]float64) [][]complex128 {
	y := make([][]complex128, len(x))
	for n, v := range x {
		y[n] = ToComplex(v)
	}
	return y
}

// Segment returns segs equal-length slices that are segments of x with noverlap% of overlap.
// The returned slices are not copies of x, but slices into it.
// Trailing entries in x that connot be included in the equal-length segments are discarded.
// noverlap is a percentage, thus 0 <= noverlap <= 1, and noverlap = 0.5 is 50% overlap.
func Segment(x []complex128, segs int, noverlap float64) [][]complex128 {
	lx := len(x)

	// determine step, length, and overlap
	var overlap, length, step, tot int
	for length = lx; length > 0; length-- {
		overlap = int(float64(length) * noverlap)
		tot = segs*(length-overlap) + overlap
		if tot <= lx {
			step = length - overlap
			break
		}
	}

	if length == 0 {
		panic("too many segments")
	}

	r := make([][]complex128, segs)
	s := 0
	for n := range r {
		r[n] = x[s : s+length]
		s += step
	}

	return r
}
