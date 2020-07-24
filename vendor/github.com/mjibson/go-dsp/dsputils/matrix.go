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

// Matrix is a multidimensional matrix of arbitrary size and dimension.
// It cannot be resized after creation. Arrays in any axis can be set or fetched.
type Matrix struct {
	list          []complex128
	dims, offsets []int
}

// MakeMatrix returns a new Matrix populated with x having dimensions dims.
// For example, to create a 3-dimensional Matrix with 2 components, 3 rows, and 4 columns:
//   MakeMatrix([]complex128 {
//     1, 2, 3, 4,
//     5, 6, 7, 8,
//     9, 0, 1, 2,
//
//     3, 4, 5, 6,
//     7, 8, 9, 0,
//     4, 3, 2, 1},
//   []int {2, 3, 4})
func MakeMatrix(x []complex128, dims []int) *Matrix {
	length := 1
	offsets := make([]int, len(dims))

	for i := len(dims) - 1; i >= 0; i-- {
		if dims[i] < 1 {
			panic("invalid dimensions")
		}

		offsets[i] = length
		length *= dims[i]
	}

	if len(x) != length {
		panic("incorrect dimensions")
	}

	dc := make([]int, len(dims))
	copy(dc, dims)
	return &Matrix{x, dc, offsets}
}

// MakeMatrix2 is a helper function to convert a 2-d array to a matrix.
func MakeMatrix2(x [][]complex128) *Matrix {
	dims := []int{len(x), len(x[0])}
	r := make([]complex128, dims[0]*dims[1])
	for n, v := range x {
		if len(v) != dims[1] {
			panic("ragged array")
		}

		copy(r[n*dims[1]:(n+1)*dims[1]], v)
	}

	return MakeMatrix(r, dims)
}

// Copy returns a new copy of m.
func (m *Matrix) Copy() *Matrix {
	r := &Matrix{m.list, m.dims, m.offsets}
	r.list = make([]complex128, len(m.list))
	copy(r.list, m.list)
	return r
}

// MakeEmptyMatrix creates an empty Matrix with given dimensions.
func MakeEmptyMatrix(dims []int) *Matrix {
	x := 1
	for _, v := range dims {
		x *= v
	}

	return MakeMatrix(make([]complex128, x), dims)
}

// offset returns the index in the one-dimensional array
func (s *Matrix) offset(dims []int) int {
	if len(dims) != len(s.dims) {
		panic("incorrect dimensions")
	}

	i := 0
	for n, v := range dims {
		if v > s.dims[n] {
			panic("incorrect dimensions")
		}

		i += v * s.offsets[n]
	}

	return i
}

func (m *Matrix) indexes(dims []int) []int {
	i := -1
	for n, v := range dims {
		if v == -1 {
			if i >= 0 {
				panic("only one dimension index allowed")
			}

			i = n
		} else if v >= m.dims[n] {
			panic("dimension out of bounds")
		}
	}

	if i == -1 {
		panic("must specify one dimension index")
	}

	x := 0
	for n, v := range dims {
		if v >= 0 {
			x += m.offsets[n] * v
		}
	}

	r := make([]int, m.dims[i])
	for j := range r {
		r[j] = x + m.offsets[i]*j
	}

	return r
}

// Dimensions returns the dimension array of the Matrix.
func (m *Matrix) Dimensions() []int {
	r := make([]int, len(m.dims))
	copy(r, m.dims)
	return r
}

// Dim returns the array of any given index of the Matrix.
// Exactly one value in dims must be -1. This is the array dimension returned.
// For example, using the Matrix documented in MakeMatrix:
//   m.Dim([]int {1, 0, -1}) = []complex128 {3, 4, 5, 6}
//   m.Dim([]int {0, -1, 2}) = []complex128 {3, 7, 1}
//   m.Dim([]int {-1, 1, 3}) = []complex128 {8, 0}
func (s *Matrix) Dim(dims []int) []complex128 {
	inds := s.indexes(dims)
	r := make([]complex128, len(inds))
	for n, v := range inds {
		r[n] = s.list[v]
	}

	return r
}

func (m *Matrix) SetDim(x []complex128, dims []int) {
	inds := m.indexes(dims)
	if len(x) != len(inds) {
		panic("incorrect array length")
	}

	for n, v := range inds {
		m.list[v] = x[n]
	}
}

// Value returns the value at the given index.
// m.Value([]int {1, 2, 3, 4}) is equivalent to m[1][2][3][4].
func (s *Matrix) Value(dims []int) complex128 {
	return s.list[s.offset(dims)]
}

// SetValue sets the value at the given index.
// m.SetValue(10, []int {1, 2, 3, 4}) is equivalent to m[1][2][3][4] = 10.
func (s *Matrix) SetValue(x complex128, dims []int) {
	s.list[s.offset(dims)] = x
}

// To2D returns the 2-D array equivalent of the Matrix.
// Only works on Matrixes of 2 dimensions.
func (m *Matrix) To2D() [][]complex128 {
	if len(m.dims) != 2 {
		panic("can only convert 2-D Matrixes")
	}

	r := make([][]complex128, m.dims[0])
	for i := 0; i < m.dims[0]; i++ {
		r[i] = make([]complex128, m.dims[1])
		copy(r[i], m.list[i*m.dims[1]:(i+1)*m.dims[1]])
	}

	return r
}

// PrettyClose returns true if the Matrixes are very close, else false.
// Comparison done using dsputils.PrettyCloseC().
func (m *Matrix) PrettyClose(n *Matrix) bool {
	// todo: use new slice equality comparison
	for i, v := range m.dims {
		if v != n.dims[i] {
			return false
		}
	}

	return PrettyCloseC(m.list, n.list)
}
