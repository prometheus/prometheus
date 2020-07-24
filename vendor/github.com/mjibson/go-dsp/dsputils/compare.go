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

package dsputils

import (
	"math"
)

const (
	closeFactor = 1e-8
)

// PrettyClose returns true if the slices a and b are very close, else false.
func PrettyClose(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !Float64Equal(c, b[i]) {
			return false
		}
	}
	return true
}

// PrettyCloseC returns true if the slices a and b are very close, else false.
func PrettyCloseC(a, b []complex128) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !ComplexEqual(c, b[i]) {
			return false
		}
	}
	return true
}

// PrettyClose2 returns true if the matrixes a and b are very close, else false.
func PrettyClose2(a, b [][]complex128) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !PrettyCloseC(c, b[i]) {
			return false
		}
	}
	return true
}

// PrettyClose2F returns true if the matrixes a and b are very close, else false.
func PrettyClose2F(a, b [][]float64) bool {
	if len(a) != len(b) {
		return false
	}

	for i, c := range a {
		if !PrettyClose(c, b[i]) {
			return false
		}
	}
	return true
}

// ComplexEqual returns true if a and b are very close, else false.
func ComplexEqual(a, b complex128) bool {
	r_a := real(a)
	r_b := real(b)
	i_a := imag(a)
	i_b := imag(b)

	return Float64Equal(r_a, r_b) && Float64Equal(i_a, i_b)
}

// Float64Equal returns true if a and b are very close, else false.
func Float64Equal(a, b float64) bool {
	return math.Abs(a-b) <= closeFactor || math.Abs(1-a/b) <= closeFactor
}
