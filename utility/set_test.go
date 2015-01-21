// Copyright 2013 The Prometheus Authors
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

package utility

import (
	"testing"
	"testing/quick"
)

func TestSetEqualMemberships(t *testing.T) {
	f := func(x int) bool {
		first := make(Set)
		second := make(Set)

		first.Add(x)
		second.Add(x)

		intersection := first.Intersection(second)

		members := intersection.Elements()

		return members != nil && len(members) == 1 && members[0] == x
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestSetInequalMemberships(t *testing.T) {
	f := func(x int) bool {
		first := make(Set)
		second := make(Set)

		first.Add(x)

		intersection := first.Intersection(second)

		members := intersection.Elements()

		return members != nil && len(members) == 0
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestSetAsymmetricMemberships(t *testing.T) {
	f := func(x int) bool {
		first := make(Set)
		second := make(Set)

		first.Add(x)
		second.Add(x)
		first.Add(x + 1)
		second.Add(x + 1)
		second.Add(x + 2)
		first.Add(x + 2)
		first.Add(x + 3)
		second.Add(x + 4)

		intersection := first.Intersection(second)

		members := intersection.Elements()

		return members != nil && len(members) == 3
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestSetRemoval(t *testing.T) {
	f := func(x int) bool {
		first := make(Set)

		first.Add(x)
		first.Remove(x)

		members := first.Elements()

		return members != nil && len(members) == 0
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestSetAdditionAndRemoval(t *testing.T) {
	f := func(x int) bool {
		first := make(Set)
		second := make(Set)

		first.Add(x)
		second.Add(x)
		first.Add(x + 1)
		first.Remove(x + 1)

		intersection := first.Intersection(second)
		members := intersection.Elements()

		return members != nil && len(members) == 1 && members[0] == x
	}

	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
