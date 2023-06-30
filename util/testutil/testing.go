// The MIT License (MIT)

// Copyright (c) 2014 Ben Johnson

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package testutil

import (
	"testing"

	"go.uber.org/goleak"
)

// TolerantVerifyLeak verifies go leaks but excludes the go routines that are
// launched as side effects of some of our dependencies.
func TolerantVerifyLeak(m *testing.M) {
	goleak.VerifyTestMain(m,
		// https://github.com/census-instrumentation/opencensus-go/blob/d7677d6af5953e0506ac4c08f349c62b917a443a/stats/view/worker.go#L34
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		// https://github.com/kubernetes/klog/blob/c85d02d1c76a9ebafa81eb6d35c980734f2c4727/klog.go#L417
		goleak.IgnoreTopFunction("k8s.io/klog/v2.(*loggingT).flushDaemon"),
		// This go routine uses a ticker to stop, so it can create false
		// positives.
		// https://github.com/kubernetes/client-go/blob/f6ce18ae578c8cca64d14ab9687824d9e1305a67/util/workqueue/queue.go#L201
		goleak.IgnoreTopFunction("k8s.io/client-go/util/workqueue.(*Type).updateUnfinishedWorkLoop"),
		// Ignore "ristretto" and its dependency "glog".
		goleak.IgnoreTopFunction("github.com/dgraph-io/ristretto.(*defaultPolicy).processItems"),
		goleak.IgnoreTopFunction("github.com/dgraph-io/ristretto.(*Cache).processItems"),
		goleak.IgnoreTopFunction("github.com/golang/glog.(*fileSink).flushDaemon"),
	)
}
