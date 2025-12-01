// Copyright The Prometheus Authors
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

package scrape

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

// For readability.
type sample = teststorage.Sample

func withCtx(ctx context.Context) func(sl *scrapeLoop) {
	return func(sl *scrapeLoop) {
		sl.ctx = ctx
	}
}

func withAppendable(appendable storage.Appendable) func(sl *scrapeLoop) {
	return func(sl *scrapeLoop) {
		sl.appendable = appendable
	}
}

// newTestScrapeLoop is the initial scrape loop for all tests.
// It returns scrapeLoop and mock scraper you can customize.
//
// It's recommended to use withXYZ functions for simple option customizations, e.g:
//
//	appTest := teststorage.NewAppendable()
//	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))
//
// However, when changing more than one scrapeLoop options it's more readable to have one explicit opt function:
//
//	ctx, cancel := context.WithCancel(t.Context())
//	appTest := teststorage.NewAppendable()
//	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
//		sl.ctx = ctx
//		sl.appendable = appTest
//		// Since we're writing samples directly below we need to provide a protocol fallback.
//		sl.fallbackScrapeProtocol = "text/plain"
//	})
//
// NOTE: Try to NOT add more parameter to this function. Try to NOT add more
// newTestScrapeLoop-like constructors. It should be flexible enough with scrapeLoop
// used for initial options.
func newTestScrapeLoop(t testing.TB, opts ...func(sl *scrapeLoop)) (_ *scrapeLoop, scraper *testScraper) {
	scraper = &testScraper{}

	ctx := t.Context()
	sl := &scrapeLoop{
		appendable:          teststorage.NewAppendable(), // Serves as a nop appendable, unless replaced by option.
		sampleMutator:       nopMutator,
		reportSampleMutator: nopMutator,
		validationScheme:    model.UTF8Validation,
		symbolTable:         labels.NewSymbolTable(),
		metrics:             newTestScrapeMetrics(t),
		scrapeLoopOptions: scrapeLoopOptions{
			interval:          10 * time.Millisecond,
			maxSchema:         histogram.ExponentialSchemaMax,
			timeout:           1 * time.Hour,
			honorTimestamps:   true,
			enableCompression: true,
		},
		appendMetadataToWAL: true, // Tests assumes it's enabled, unless explicitly turned off.
	}
	for _, o := range opts {
		o(sl)
	}
	// Use sl.ctx for context injection.
	// True contexts (sl.appenderCtx, sl.parentCtx, sl.ctx) are populated in init.
	if sl.ctx != nil {
		ctx = sl.ctx
	}

	// Validate user opts for convenience.
	require.Nil(t, sl.parentCtx, "newTestScrapeLoop does not support injecting non-nil parent context")
	require.Nil(t, sl.appenderCtx, "newTestScrapeLoop does not support injecting non-nil appender context")
	require.Nil(t, sl.cancel, "newTestScrapeLoop does not support injecting custom cancel function")
	require.Nil(t, sl.scraper, "newTestScrapeLoop does not support injecting scraper, it's mocked, use the returned scraper")
	sl.scraper = scraper
	sl.init(ctx, true)
	return sl, scraper
}

// protoMarshalDelimited marshals a MetricFamily into a delimited
// Prometheus proto exposition format bytes (known as `encoding=delimited`)
//
// See also https://eli.thegreenplace.net/2011/08/02/length-prefix-framing-for-protocol-buffers
func protoMarshalDelimited(t *testing.T, mf *dto.MetricFamily) []byte {
	t.Helper()

	protoBuf, err := proto.Marshal(mf)
	require.NoError(t, err)

	varintBuf := make([]byte, binary.MaxVarintLen32)
	varintLength := binary.PutUvarint(varintBuf, uint64(len(protoBuf)))

	buf := &bytes.Buffer{}
	buf.Write(varintBuf[:varintLength])
	buf.Write(protoBuf)
	return buf.Bytes()
}
