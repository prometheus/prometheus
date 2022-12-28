package tsdb

import (
	"container/list"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/index"
)

const (
	defaultPostingsForMatchersCacheTTL  = 10 * time.Second
	defaultPostingsForMatchersCacheSize = 100
)

// IndexPostingsReader is a subset of IndexReader methods, the minimum required to evaluate PostingsForMatchers
type IndexPostingsReader interface {
	// LabelValues returns possible label values which may not be sorted.
	LabelValues(name string, matchers ...*labels.Matcher) ([]string, error)

	// Postings returns the postings list iterator for the label pairs.
	// The Postings here contain the offsets to the series inside the index.
	// Found IDs are not strictly required to point to a valid Series, e.g.
	// during background garbage collections. Input values must be sorted.
	Postings(name string, values ...string) (index.Postings, error)
}

// NewPostingsForMatchersCache creates a new PostingsForMatchersCache.
// If `ttl` is 0, then it only deduplicates in-flight requests.
// If `force` is true, then all requests go through cache, regardless of the `concurrent` param provided.
func NewPostingsForMatchersCache(ttl time.Duration, cacheSize int, force bool) *PostingsForMatchersCache {
	b := &PostingsForMatchersCache{
		calls:  &sync.Map{},
		cached: list.New(),

		ttl:       ttl,
		cacheSize: cacheSize,
		force:     force,

		timeNow:             time.Now,
		postingsForMatchers: PostingsForMatchers,
	}

	return b
}

// PostingsForMatchersCache caches PostingsForMatchers call results when the concurrent hint is passed in or force is true.
type PostingsForMatchersCache struct {
	calls *sync.Map

	cachedMtx sync.RWMutex
	cached    *list.List

	ttl       time.Duration
	cacheSize int
	force     bool

	// timeNow is the time.Now that can be replaced for testing purposes
	timeNow func() time.Time
	// postingsForMatchers can be replaced for testing purposes
	postingsForMatchers func(ix IndexPostingsReader, ms ...*labels.Matcher) (index.Postings, error)
}

func (c *PostingsForMatchersCache) PostingsForMatchers(ix IndexPostingsReader, concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	if !concurrent && !c.force {
		return c.postingsForMatchers(ix, ms...)
	}
	c.expire()
	return c.postingsForMatchersPromise(ix, ms)()
}

func (c *PostingsForMatchersCache) postingsForMatchersPromise(ix IndexPostingsReader, ms []*labels.Matcher) func() (index.Postings, error) {
	var (
		wg       sync.WaitGroup
		cloner   *index.PostingsCloner
		outerErr error
	)
	wg.Add(1)

	promise := func() (index.Postings, error) {
		wg.Wait()
		if outerErr != nil {
			return nil, outerErr
		}
		return cloner.Clone(), nil
	}

	key := matchersKey(ms)
	oldPromise, loaded := c.calls.LoadOrStore(key, promise)
	if loaded {
		return oldPromise.(func() (index.Postings, error))
	}
	defer wg.Done()

	if postings, err := c.postingsForMatchers(ix, ms...); err != nil {
		outerErr = err
	} else {
		cloner = index.NewPostingsCloner(postings)
	}

	c.created(key, c.timeNow())
	return promise
}

type postingsForMatchersCachedCall struct {
	key string
	ts  time.Time
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
// should be called while read lock is held on cachedMtx
func (c *PostingsForMatchersCache) shouldEvictHead() bool {
	if c.cached.Len() > c.cacheSize {
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
}

// created has to be called when returning from the PostingsForMatchers call that creates the promise.
// the ts provided should be the call time.
func (c *PostingsForMatchersCache) created(key string, ts time.Time) {
	if c.ttl <= 0 {
		c.calls.Delete(key)
		return
	}

	c.cachedMtx.Lock()
	defer c.cachedMtx.Unlock()

	c.cached.PushBack(&postingsForMatchersCachedCall{
		key: key,
		ts:  ts,
	})
}

// matchersKey provides a unique string key for the given matchers slice
// NOTE: different orders of matchers will produce different keys,
// but it's unlikely that we'll receive same matchers in different orders at the same time
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

// indexReaderWithPostingsForMatchers adapts an index.Reader to be an IndexReader by adding the PostingsForMatchers method
type indexReaderWithPostingsForMatchers struct {
	*index.Reader
	pfmc *PostingsForMatchersCache
}

func (ir indexReaderWithPostingsForMatchers) PostingsForMatchers(concurrent bool, ms ...*labels.Matcher) (index.Postings, error) {
	return ir.pfmc.PostingsForMatchers(ir, concurrent, ms...)
}

var _ IndexReader = indexReaderWithPostingsForMatchers{}
