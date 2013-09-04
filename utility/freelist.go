// Copyright 2013 Prometheus Team
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

type FreeList chan interface{}

func NewFreeList(cap int) FreeList {
	return make(FreeList, cap)
}

func (l FreeList) Get() (interface{}, bool) {
	select {
	case v := <-l:
		return v, true
	default:
		return nil, false
	}
}

func (l FreeList) Give(v interface{}) bool {
	select {
	case l <- v:
		return true
	default:
		return false
	}
}

func (l FreeList) Close() {
	close(l)

	for _ = range l {
	}
}
