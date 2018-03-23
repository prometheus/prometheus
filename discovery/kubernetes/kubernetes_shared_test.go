// Copyright 2018 The Prometheus Authors
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

package kubernetes

import (
	"fmt"
	"testing"

	"github.com/go-kit/kit/log"
)

func TestKubernetesSharedCache(t *testing.T) {
	{
		// test GetOrCreate
		cache := NewKubernetesSharedCache(log.NewNopLogger())
		tmpShared := &kubernetesShared{}
		testcases := []struct {
			name                     string
			key                      string
			create                   func() (*kubernetesShared, error)
			expectedKubernetesShared KubernetesShared
			expectedErr              bool
		}{
			{
				name: "create successfully",
				key:  "test",
				create: func() (*kubernetesShared, error) {
					return tmpShared, nil
				},
				expectedKubernetesShared: tmpShared,
				expectedErr:              false,
			},
			{
				name: "create failed",
				key:  "test2",
				create: func() (*kubernetesShared, error) {
					return nil, fmt.Errorf("create error")
				},
				expectedKubernetesShared: nil,
				expectedErr:              true,
			},
		}
		for _, v := range testcases {
			shared, err := cache.GetOrCreate(v.key, v.create)
			if v.expectedErr && err == nil {
				t.Errorf("test %s: err should be non-nil", v.name)
			}
			if v.expectedKubernetesShared != shared {
				t.Errorf("test %s: shared should be %+v, got %+v", v.name, v.expectedKubernetesShared, shared)
			}
		}
	}

	{
		// test GetOrCreate with same key
		cache := NewKubernetesSharedCache(log.NewNopLogger())
		tmpShared := &kubernetesShared{}
		count := 0
		for i := 0; i < 10; i++ {
			cache.GetOrCreate("test", func() (*kubernetesShared, error) {
				count++
				return tmpShared, nil
			})
		}
		if count != 1 {
			t.Errorf("create function for same key should only be called once, called %d times", count)
		}

		shared, err := cache.GetOrCreate("test", nil)
		if err != nil {
			t.Errorf("err should be nil, got: %v", err)
		}
		if shared != tmpShared {
			t.Errorf("shared should be %v, got %v", tmpShared, shared)
		}
	}

	{
		// test GetOrCreate then release
		cache := NewKubernetesSharedCache(log.NewNopLogger())
		tmpShared := &kubernetesShared{}
		if cache.Count() != 0 {
			t.Errorf("count should be 0 at beginning")
		}
		cache.GetOrCreate("test", func() (*kubernetesShared, error) {
			return tmpShared, nil
		})
		if cache.Count() != 1 {
			t.Errorf("count should be 1, got: %d", cache.Count())
		}
		cache.Release("test")
		if cache.Count() != 0 {
			t.Errorf("count should be 0, got: %d", cache.Count())
		}
	}
}
