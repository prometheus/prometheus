// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

func LazySeriesSet(init func() (SeriesSet, bool)) SeriesSet {
	return &lazySeriesSet{init: init}
}

// lazySeriesSet is a wrapped series set that is initialised on first call to Next().
type lazySeriesSet struct {
	init func() (s SeriesSet, ok bool)

	set SeriesSet
}

func (c *lazySeriesSet) Next() bool {
	if c.set != nil {
		return c.set.Next()
	}

	var ok bool
	c.set, ok = c.init()
	return ok
}

func (c *lazySeriesSet) Err() error {
	if c.set != nil {
		return c.set.Err()
	}
	return nil
}

func (c *lazySeriesSet) At() Series {
	if c.set != nil {
		return c.set.At()
	}
	return nil
}

func (c *lazySeriesSet) Warnings() Warnings {
	if c.set != nil {
		return c.set.Warnings()
	}
	return nil
}

// lazyGenericSeriesSet is a wrapped series set that is initialised on first call to Next().
type lazyGenericSeriesSet struct {
	init func() (genericSeriesSet, bool)

	set genericSeriesSet
}

func (c *lazyGenericSeriesSet) Next() bool {
	if c.set != nil {
		return c.set.Next()
	}
	var ok bool
	c.set, ok = c.init()
	return ok
}

func (c *lazyGenericSeriesSet) Err() error {
	if c.set != nil {
		return c.set.Err()
	}
	return nil
}

func (c *lazyGenericSeriesSet) At() Labels {
	if c.set != nil {
		return c.set.At()
	}
	return nil
}

func (c *lazyGenericSeriesSet) Warnings() Warnings {
	if c.set != nil {
		return c.set.Warnings()
	}
	return nil
}

type warningsOnlyGenericSeriesSet Warnings

func (warningsOnlyGenericSeriesSet) Next() bool           { return false }
func (warningsOnlyGenericSeriesSet) Err() error           { return nil }
func (warningsOnlyGenericSeriesSet) At() Labels           { return nil }
func (c warningsOnlyGenericSeriesSet) Warnings() Warnings { return Warnings(c) }

type errorOnlyGenericSeriesSet struct {
	err error
}

func (errorOnlyGenericSeriesSet) Next() bool         { return false }
func (errorOnlyGenericSeriesSet) At() Labels         { return nil }
func (s errorOnlyGenericSeriesSet) Err() error       { return s.err }
func (errorOnlyGenericSeriesSet) Warnings() Warnings { return nil }
