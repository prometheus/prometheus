// Copyright 2019 The Prometheus Authors
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
//
// Inspired / copied / modified from https://gitlab.com/cznic/strutil/blob/master/strutil.go,
// which is MIT licensed, so:
//
// Copyright (c) 2014 The strutil Authors. All rights reserved.

package remote

import "sync"

var interner = newPool()

type pool struct {
	mtx  sync.RWMutex
	pool map[string]string
}

func newPool() *pool {
	return &pool{
		pool: map[string]string{},
	}
}

func (p *pool) intern(s string) string {
	if s == "" {
		return ""
	}

	p.mtx.RLock()
	interned, ok := p.pool[s]
	p.mtx.RUnlock()
	if ok {
		return interned
	}

	p.mtx.Lock()
	defer p.mtx.Unlock()
	if interned, ok := p.pool[s]; ok {
		return interned
	}

	s = pack(s)
	p.pool[s] = s
	return s
}

// StrPack returns a new instance of s which is tightly packed in memory.
// It is intended for avoiding the situation where having a live reference
// to a string slice over an unreferenced biger underlying string keeps the biger one
// in memory anyway - it can't be GCed.
func pack(s string) string {
	return string([]byte(s))
}
