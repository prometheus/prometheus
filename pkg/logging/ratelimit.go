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

package logging

import (
	"github.com/go-kit/kit/log"
	"golang.org/x/time/rate"
)

type ratelimiter struct {
	limiter *rate.Limiter
	next    log.Logger
}

// RateLimit write to a logger.
func RateLimit(next log.Logger, limit rate.Limit) log.Logger {
	return &ratelimiter{
		limiter: rate.NewLimiter(limit, int(limit)),
		next:    next,
	}
}

func (r *ratelimiter) Log(keyvals ...interface{}) error {
	if r.limiter.Allow() {
		return r.next.Log(keyvals...)
	}
	return nil
}
