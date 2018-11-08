// Copyright 2015 The Prometheus Authors
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

package model

// ForeachBaggageItem implements opentracing.SpanContext
func (s SpanContext) ForeachBaggageItem(handler func(k, v string) bool) {
	for _, b := range s.Baggage {
		if !handler(b.Key, b.Value) {
			return
		}
	}
}

func (s SpanContext) baggageItem(k string) string {
	for _, b := range s.Baggage {
		if b.Key == k {
			return b.Value
		}
	}
	return ""
}

func (s SpanContext) withBaggageItem(k, v string) SpanContext {
	result := s
	result.Baggage = append(result.Baggage, Baggage{
		Key:   k,
		Value: v,
	})
	return result
}
