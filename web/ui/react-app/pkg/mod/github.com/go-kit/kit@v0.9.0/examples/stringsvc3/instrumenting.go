package main

import (
	"fmt"
	"time"

	"github.com/go-kit/kit/metrics"
)

func instrumentingMiddleware(
	requestCount metrics.Counter,
	requestLatency metrics.Histogram,
	countResult metrics.Histogram,
) ServiceMiddleware {
	return func(next StringService) StringService {
		return instrmw{requestCount, requestLatency, countResult, next}
	}
}

type instrmw struct {
	requestCount   metrics.Counter
	requestLatency metrics.Histogram
	countResult    metrics.Histogram
	StringService
}

func (mw instrmw) Uppercase(s string) (output string, err error) {
	defer func(begin time.Time) {
		lvs := []string{"method", "uppercase", "error", fmt.Sprint(err != nil)}
		mw.requestCount.With(lvs...).Add(1)
		mw.requestLatency.With(lvs...).Observe(time.Since(begin).Seconds())
	}(time.Now())

	output, err = mw.StringService.Uppercase(s)
	return
}

func (mw instrmw) Count(s string) (n int) {
	defer func(begin time.Time) {
		lvs := []string{"method", "count", "error", "false"}
		mw.requestCount.With(lvs...).Add(1)
		mw.requestLatency.With(lvs...).Observe(time.Since(begin).Seconds())
		mw.countResult.Observe(float64(n))
	}(time.Now())

	n = mw.StringService.Count(s)
	return
}
