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

package fft

import (
	"math"
	"sync"

	"github.com/mjibson/go-dsp/dsputils"
)

var (
	bluesteinLock       sync.RWMutex
	bluesteinFactors    = map[int][]complex128{}
	bluesteinInvFactors = map[int][]complex128{}
)

func getBluesteinFactors(input_len int) ([]complex128, []complex128) {
	bluesteinLock.RLock()

	if hasBluesteinFactors(input_len) {
		defer bluesteinLock.RUnlock()
		return bluesteinFactors[input_len], bluesteinInvFactors[input_len]
	}

	bluesteinLock.RUnlock()
	bluesteinLock.Lock()
	defer bluesteinLock.Unlock()

	if !hasBluesteinFactors(input_len) {
		bluesteinFactors[input_len] = make([]complex128, input_len)
		bluesteinInvFactors[input_len] = make([]complex128, input_len)

		var sin, cos float64
		for i := 0; i < input_len; i++ {
			if i == 0 {
				sin, cos = 0, 1
			} else {
				sin, cos = math.Sincos(math.Pi / float64(input_len) * float64(i*i))
			}
			bluesteinFactors[input_len][i] = complex(cos, sin)
			bluesteinInvFactors[input_len][i] = complex(cos, -sin)
		}
	}

	return bluesteinFactors[input_len], bluesteinInvFactors[input_len]
}

func hasBluesteinFactors(idx int) bool {
	return bluesteinFactors[idx] != nil
}

// bluesteinFFT returns the FFT calculated using the Bluestein algorithm.
func bluesteinFFT(x []complex128) []complex128 {
	lx := len(x)
	a := dsputils.ZeroPad(x, dsputils.NextPowerOf2(lx*2-1))
	la := len(a)
	factors, invFactors := getBluesteinFactors(lx)

	for n, v := range x {
		a[n] = v * invFactors[n]
	}

	b := make([]complex128, la)
	for i := 0; i < lx; i++ {
		b[i] = factors[i]

		if i != 0 {
			b[la-i] = factors[i]
		}
	}

	r := Convolve(a, b)

	for i := 0; i < lx; i++ {
		r[i] *= invFactors[i]
	}

	return r[:lx]
}
