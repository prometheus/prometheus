package tsdb

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DmitriyVTitov/size"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/index"
)

const (
	// NOTE: keep them exported to reference them in Mimir.

	DefaultPostingsForMatchersCacheTTL      = 10 * time.Second
	DefaultPostingsForMatchersCacheMaxItems = 100
	DefaultPostingsForMatchersCacheMaxBytes = 10 * 1024 * 1024 // Based on the default max items, 10MB / 100 = 100KB per cached entry on average.
	DefaultPostingsForMatchersCacheForce    = false
)

// IndexPostingsReader is a subset of IndexReader methods, the minimum required to evaluate PostingsForMatchers.
type IndexPostingsReader interface {
	// LabelValues returns possible label values which may not be sorted.
	LabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error)

	// Postings returns the postings list iterator for the label pairs.
	// The Postings here contain the offsets to the series inside the index.
	// Found IDs are not strictly required to point to a valid Series, e.g.
	// during background garbage collections. Input values must be sorted.
	Postings(ctx context.Context, name string, values ...string) (index.Postings, error)
}

// NewPostingsForMatchersCache creates a new PostingsForMatchersCache.
// If `ttl` is 0, then it only deduplicates in-flight requests.
// If `force` is true, then all requests go through cache, regardless of the `concurrent` param provided to the PostingsForMatchers method.
func NewPostingsForMatchersCache(ttl time.Duration, maxItems int, maxBytes int64, force bool) *PostingsForMatchersCache {
	b := &PostingsForMatchersCache{
		calls:  &sync.Map{},
		cached: list.New(),

		ttl:      ttl,
		maxItems: maxItems,
		maxBytes: maxBytes,
		force:    force,

		timeNow:             time.Now,
		postingsForMatchers: PostingsForMatchers,

		tracer:      otel.Tracer(""),
		ttlAttrib:   attribute.Stringer("ttl", ttl),
		forceAttrib: attribute.Bool("force", force),
	}

	return b
}

// PostingsForMatchersCache caches PostingsForMatchers call results when the concurrent hint is passed in or force is true.
type PostingsForMatchersCache struct {
	calls *sync.Map

	cachedMtx   sync.RWMutex
	cached      *list.List
	cachedBytes int64

	ttl      time.Duration
	maxItems int
	maxBytes int64
	force    bool

	// timeNow is the time.Now that can be replaced for testing purposes
	timeNow func() time.Time
	// postingsForMatchers can be replaced for testing purposes
	postingsForMatchers func(ctx context.Context, ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error)

	tracer trace.Tracer
	// Preallocated for performance
	ttlAttrib   attribute.KeyValue
	forceAttrib attribute.KeyValue
}

func (c *PostingsForMatchersCache) PostingsForMatchers(ctx context.Context, ix IndexPostingsReader, concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	span := trace.SpanFromContext(ctx)
	defer func(startTime time.Time) {
		span.AddEvent(
			"PostingsForMatchers returned",
			trace.WithAttributes(attribute.Bool("concurrent", concurrent), c.ttlAttrib, c.forceAttrib, attribute.Stringer("duration", time.Since(startTime))),
		)
	}(time.Now())

	if !concurrent && !c.force {
		span.AddEvent("cache not used")
		p, err := c.postingsForMatchers(ctx, ix, ms...)
		if err != nil {
			span.SetStatus(codes.Error, "getting postings for matchers without cache failed")
			span.RecordError(err)
		}
		return p, err
	}

	span.AddEvent("using cache")
	c.expire()
	p, err := c.postingsForMatchersPromise(ctx, ix, ms)(ctx)
	if err != nil {
		span.SetStatus(codes.Error, "getting postings for matchers with cache failed")
		span.RecordError(err)
	}
	return p, err
}

type postingsForMatcherPromise struct {
	done chan struct{}

	cloner *index.PostingsCloner
	err    error
}

func (p *postingsForMatcherPromise) result(ctx context.Context) (index.Postings, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("interrupting wait on postingsForMatchers promise due to context error: %w", ctx.Err())
	case <-p.done:
		// Checking context error is necessary for deterministic tests,
		// as channel selection order is random
		if ctx.Err() != nil {
			return nil, fmt.Errorf("completed postingsForMatchers promise, but context has error: %w", ctx.Err())
		}
		if p.err != nil {
			return nil, fmt.Errorf("postingsForMatchers promise completed with error: %w", p.err)
		}
		return p.cloner.Clone(), nil
	}
}

func (c *PostingsForMatchersCache) postingsForMatchersPromise(ctx context.Context, ix IndexPostingsReader, ms []*labels.Matcher) func(context.Context) (index.Postings, error) {
	span := trace.SpanFromContext(ctx)

	promise := &postingsForMatcherPromise{
		done: make(chan struct{}),
	}

	key := matchersKey(ms)
	oldPromise, loaded := c.calls.LoadOrStore(key, promise)
	if loaded {
		// promise was not stored, we return a previously stored promise, that's possibly being fulfilled in another goroutine
		span.AddEvent("using cached postingsForMatchers promise", trace.WithAttributes(
			attribute.String("cache_key", key),
		))
		close(promise.done)
		return oldPromise.(*postingsForMatcherPromise).result
	}

	span.AddEvent("no postingsForMatchers promise in cache, executing query", trace.WithAttributes(attribute.String("cache_key", key)))

	// promise was stored, close its channel after fulfilment
	defer close(promise.done)

	// Don't let context cancellation fail the promise, since it may be used by multiple goroutines, each with
	// its own context. Also, keep the call independent of this particular context, since the promise will be reused.
	// FIXME: do we need to cancel the call to postingsForMatchers if all the callers waiting for the result have
	// cancelled their context?
	if postings, err := c.postingsForMatchers(context.Background(), ix, ms...); err != nil {
		promise.err = err
	} else {
		promise.cloner = index.NewPostingsCloner(postings)
	}

	sizeBytes := int64(len(key) + size.Of(promise))

	c.created(ctx, key, c.timeNow(), sizeBytes)
	return promise.result
}

type postingsForMatchersCachedCall struct {
	key string
	ts  time.Time

	// Size of the cached entry, in bytes.
	sizeBytes int64
}

func (c *PostingsForMatchersCache) expire() {
	if c.ttl <= 0 {
		return
	}

	c.cachedMtx.RLock()
	if !c.shouldEvictHead() {
		c.cachedMtx.RUnlock()
		return
	}
	c.cachedMtx.RUnlock()

	c.cachedMtx.Lock()
	defer c.cachedMtx.Unlock()

	for c.shouldEvictHead() {
		c.evictHead()
	}
}

// shouldEvictHead returns true if cache head should be evicted, either because it's too old,
// or because the cache has too many elements
// should be called while read lock is held on cachedMtx.
func (c *PostingsForMatchersCache) shouldEvictHead() bool {
	// The cache should be evicted for sure if the max size (either items or bytes) is reached.
	if c.cached.Len() > c.maxItems || c.cachedBytes > c.maxBytes {
		return true
	}

	h := c.cached.Front()
	if h == nil {
		return false
	}
	ts := h.Value.(*postingsForMatchersCachedCall).ts
	return c.timeNow().Sub(ts) >= c.ttl
}

func (c *PostingsForMatchersCache) evictHead() {
	front := c.cached.Front()
	oldest := front.Value.(*postingsForMatchersCachedCall)
	c.calls.Delete(oldest.key)
	c.cached.Remove(front)
	c.cachedBytes -= oldest.sizeBytes
}

// created has to be called when returning from the PostingsForMatchers call that creates the promise.
// the ts provided should be the call time.
func (c *PostingsForMatchersCache) created(ctx context.Context, key string, ts time.Time, sizeBytes int64) {
	span := trace.SpanFromContext(ctx)

	if c.ttl <= 0 {
		span.AddEvent("deleting cached promise since c.ttl <= 0")
		c.calls.Delete(key)
		return
	}

	c.cachedMtx.Lock()
	defer c.cachedMtx.Unlock()

	c.cached.PushBack(&postingsForMatchersCachedCall{
		key:       key,
		ts:        ts,
		sizeBytes: sizeBytes,
	})
	c.cachedBytes += sizeBytes
	span.AddEvent("added cached value to expiry queue", trace.WithAttributes(
		attribute.Stringer("timestamp", ts),
		attribute.Int64("size in bytes", sizeBytes),
		attribute.Int64("cached bytes", c.cachedBytes),
	))
}

// matchersKey provides a unique string key for the given matchers slice.
// NOTE: different orders of matchers will produce different keys,
// but it's unlikely that we'll receive same matchers in different orders at the same time.
func matchersKey(ms []*labels.Matcher) string {
	const (
		typeLen = 2
		sepLen  = 1
	)
	var size int
	for _, m := range ms {
		size += len(m.Name) + len(m.Value) + typeLen + sepLen
	}
	sb := strings.Builder{}
	sb.Grow(size)
	for _, m := range ms {
		sb.WriteString(m.Name)
		sb.WriteString(m.Type.String())
		sb.WriteString(m.Value)
		sb.WriteByte(0)
	}
	key := sb.String()
	return key
}

// indexReaderWithPostingsForMatchers adapts an index.Reader to be an IndexReader by adding the PostingsForMatchers method.
type indexReaderWithPostingsForMatchers struct {
	*index.Reader
	pfmc *PostingsForMatchersCache
}

func (ir indexReaderWithPostingsForMatchers) PostingsForMatchers(ctx context.Context, concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	return ir.pfmc.PostingsForMatchers(ctx, ir, concurrent, ms...)
}

var _ IndexReader = indexReaderWithPostingsForMatchers{}
