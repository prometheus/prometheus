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
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/pool"
	"github.com/prometheus/prometheus/util/teststorage"
)

// For readability.
type sample = teststorage.Sample

type compatAppendable interface {
	storage.Appendable
	storage.AppendableV2
}

func withCtx(ctx context.Context) func(sl *scrapeLoop) {
	return func(sl *scrapeLoop) {
		sl.ctx = ctx
	}
}

func withAppendable(app compatAppendable, appV2 bool) func(sl *scrapeLoop) {
	return func(sl *scrapeLoop) {
		sa := selectAppendable(app, appV2)
		sl.appendable = sa.V1()
		sl.appendableV2 = sa.V2()
	}
}

// newTestScrapeLoop is the initial scrape loop for all tests.
// It returns scrapeLoop and mock scraper you can customize.
//
// It's recommended to use withXYZ functions for simple option customizations, e.g:
//
//	sl, _ := newTestScrapeLoop(t, withCtx(customCtx))
//
// However, when changing more than one scrapeLoop options it's more readable to have one explicit opt function:
//
//	ctx, cancel := context.WithCancel(t.Context())
//	appTest := teststorage.NewAppendable()
//	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
//		sl.ctx = ctx
//		sl.appendableV2 = appTest
//		// Since we're writing samples directly below we need to provide a protocol fallback.
//		sl.fallbackScrapeProtocol = "text/plain"
//	})
//
// NOTE: Try to NOT add more parameter to this function. Try to NOT add more
// newTestScrapeLoop-like constructors. It should be flexible enough with scrapeLoop
// used for initial options.
func newTestScrapeLoop(t testing.TB, opts ...func(sl *scrapeLoop)) (_ *scrapeLoop, scraper *testScraper) {
	metrics := newTestScrapeMetrics(t)
	sl := &scrapeLoop{
		stopped: make(chan struct{}),

		l:     promslog.NewNopLogger(),
		cache: newScrapeCache(metrics),

		interval:            10 * time.Millisecond,
		timeout:             1 * time.Hour,
		sampleMutator:       nopMutator,
		reportSampleMutator: nopMutator,
		buffers:             pool.New(1e3, 1e6, 3, func(sz int) any { return make([]byte, 0, sz) }),
		metrics:             metrics,
		maxSchema:           histogram.ExponentialSchemaMax,
		honorTimestamps:     true,
		enableCompression:   true,
		validationScheme:    model.UTF8Validation,
		symbolTable:         labels.NewSymbolTable(),
		appendMetadataToWAL: true, // Tests assumes it's enabled, unless explicitly turned off.
	}
	for _, o := range opts {
		o(sl)
	}

	if sl.appendable != nil && sl.appendableV2 != nil {
		t.Fatal("select the appendable to use, both were passed, likely a bug")
	}

	// Validate user opts for convenience.
	require.Nil(t, sl.parentCtx, "newTestScrapeLoop does not support injecting non-nil parent context")
	require.Nil(t, sl.appenderCtx, "newTestScrapeLoop does not support injecting non-nil appender context")
	require.Nil(t, sl.cancel, "newTestScrapeLoop does not support injecting custom cancel function")
	require.Nil(t, sl.scraper, "newTestScrapeLoop does not support injecting scraper, it's mocked, use the returned scraper")

	rootCtx := t.Context()
	// Use sl.ctx for context injection.
	// True contexts (sl.appenderCtx, sl.parentCtx, sl.ctx) are populated from it
	if sl.ctx != nil {
		rootCtx = sl.ctx
	}
	ctx, cancel := context.WithCancel(rootCtx)
	sl.ctx = ctx
	sl.cancel = cancel
	sl.appenderCtx = rootCtx
	sl.parentCtx = rootCtx

	scraper = &testScraper{}
	sl.scraper = scraper
	return sl, scraper
}

func newTestScrapePool(t *testing.T, app compatAppendable, appV2 bool, injectNewLoop func(options scrapeLoopOptions) loop) *scrapePool {
	sa := selectAppendable(app, appV2)
	return &scrapePool{
		ctx:     t.Context(),
		cancel:  func() {},
		logger:  promslog.NewNopLogger(),
		config:  &config.ScrapeConfig{},
		options: &Options{},
		client:  http.DefaultClient,

		activeTargets:     map[uint64]*Target{},
		loops:             map[uint64]loop{},
		injectTestNewLoop: injectNewLoop,

		appendable: sa.V1(), appendableV2: sa.V2(),

		symbolTable: labels.NewSymbolTable(),
		metrics:     newTestScrapeMetrics(t),
	}
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

type selectedAppendable struct {
	useV2 bool
	app   compatAppendable
}

// V1 returns Appendable if V1 is selected, otherwise nil.
func (s selectedAppendable) V1() storage.Appendable {
	if s.useV2 {
		return nil
	}
	return s.app
}

// V2 returns AppendableV2 if V2 is selected, otherwise nil.
func (s selectedAppendable) V2() storage.AppendableV2 {
	if !s.useV2 {
		return nil
	}
	return s.app
}

// selectAppendable allows to specify which appendable callers should use when the struct
// implements both. This is how all callers are making the decision - if one appendable is nil, they
// take another. selectAppendable allows to inject nil to e.g. storage.AppendableV2 when appV2 is false.
func selectAppendable(app compatAppendable, appV2 bool) selectedAppendable {
	s := selectedAppendable{
		app:   app,
		useV2: appV2,
	}
	return s
}

func foreachAppendable(t *testing.T, f func(t *testing.T, appV2 bool)) {
	for _, appV2 := range []bool{false, true} {
		t.Run(fmt.Sprintf("appV2=%v", appV2), func(t *testing.T) {
			f(t, appV2)
		})
	}
}

func TestSelectAppendable(t *testing.T) {
	var i int
	foreachAppendable(t, func(t *testing.T, appV2 bool) {
		defer func() { i++ }()
		switch i {
		case 0:
			require.False(t, appV2)

			s := selectAppendable(teststorage.NewAppendable(), appV2)
			require.NotNil(t, s.V1())
			require.Nil(t, s.V2())
		case 1:
			require.True(t, appV2)

			s := selectAppendable(teststorage.NewAppendable(), appV2)
			require.Nil(t, s.V1())
			require.NotNil(t, s.V2())
		default:
			t.Fatal("too many iterations")
		}
	})
}
