// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package slice provides a slice sorting function.
package slice

import (
	"fmt"
	"reflect"
	"sort"

	"go4.org/reflectutil"
)

// Sort sorts the provided slice using the function less.
// If slice is not a slice, Sort panics.
func Sort(slice interface{}, less func(i, j int) bool) {
	sort.Sort(SortInterface(slice, less))
}

// SortInterface returns a sort.Interface to sort the provided slice
// using the function less.
func SortInterface(slice interface{}, less func(i, j int) bool) sort.Interface {
	sv := reflect.ValueOf(slice)
	if sv.Kind() != reflect.Slice {
		panic(fmt.Sprintf("slice.Sort called with non-slice value of type %T", slice))
	}
	return &funcs{
		length: sv.Len(),
		less:   less,
		swap:   reflectutil.Swapper(slice),
	}
}

type funcs struct {
	length int
	less   func(i, j int) bool
	swap   func(i, j int)
}

func (f *funcs) Len() int           { return f.length }
func (f *funcs) Less(i, j int) bool { return f.less(i, j) }
func (f *funcs) Swap(i, j int)      { f.swap(i, j) }
