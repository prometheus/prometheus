package hashcache

import (
	"sync"

	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/storage"
)

const (
	numGenerations = 4

	// approxBytesPerEntry is the estimated memory footprint (in bytes) of 1 cache
	// entry, measured with TestSeriesHashCache_MeasureApproximateSizePerEntry().
	approxBytesPerEntry = 28
)

// SeriesHashCache is a bounded cache mapping the per-block series ID with
// its labels hash.
type SeriesHashCache struct {
	maxEntriesPerGeneration uint64

	generationsMx sync.RWMutex
	generations   [numGenerations]cacheGeneration
}

func NewSeriesHashCache(maxBytes uint64) *SeriesHashCache {
	maxEntriesPerGeneration := maxBytes / approxBytesPerEntry / numGenerations
	if maxEntriesPerGeneration < 1 {
		maxEntriesPerGeneration = 1
	}

	c := &SeriesHashCache{maxEntriesPerGeneration: maxEntriesPerGeneration}

	// Init generations.
	for idx := 0; idx < numGenerations; idx++ {
		c.generations[idx].blocks = &sync.Map{}
		c.generations[idx].length = atomic.NewUint64(0)
	}

	return c
}

// GetBlockCache returns a reference to the series hash cache for the provided blockID.
// The returned cache reference should be retained only for a short period (ie. the duration
// of the execution of 1 single query).
func (c *SeriesHashCache) GetBlockCache(blockID string) *BlockSeriesHashCache {
	blockCache := &BlockSeriesHashCache{}

	c.generationsMx.RLock()
	defer c.generationsMx.RUnlock()

	// Trigger a garbage collection if the current generation reached the max size.
	if c.generations[0].length.Load() >= c.maxEntriesPerGeneration {
		c.generationsMx.RUnlock()
		c.gc()
		c.generationsMx.RLock()
	}

	for idx := 0; idx < numGenerations; idx++ {
		gen := c.generations[idx]

		if value, ok := gen.blocks.Load(blockID); ok {
			blockCache.generations[idx] = value.(*blockCacheGeneration)
			continue
		}

		// Create a new per-block cache only for the current generation.
		// If the cache for the older generation doesn't exist, then its
		// value will be null and skipped when reading.
		if idx == 0 {
			value, _ := gen.blocks.LoadOrStore(blockID, newBlockCacheGeneration(gen.length))
			blockCache.generations[idx] = value.(*blockCacheGeneration)
		}
	}

	return blockCache
}

// GetBlockCacheProvider returns a cache provider bounded to the provided blockID.
func (c *SeriesHashCache) GetBlockCacheProvider(blockID string) *BlockSeriesHashCacheProvider {
	return NewBlockSeriesHashCacheProvider(c, blockID)
}

func (c *SeriesHashCache) gc() {
	c.generationsMx.Lock()
	defer c.generationsMx.Unlock()

	// Make sure no other goroutines already GCed the current generation.
	if c.generations[0].length.Load() < c.maxEntriesPerGeneration {
		return
	}

	// Shift the current generation to old.
	for idx := numGenerations - 2; idx >= 0; idx-- {
		c.generations[idx+1] = c.generations[idx]
	}

	// Initialise a new empty current generation.
	c.generations[0] = cacheGeneration{
		blocks: &sync.Map{},
		length: atomic.NewUint64(0),
	}
}

// cacheGeneration holds a multi-blocks cache generation.
type cacheGeneration struct {
	// blocks maps the block ID with blockCacheGeneration.
	blocks *sync.Map

	// Keeps track of the number of items added to the cache. This counter
	// is passed to each blockCacheGeneration belonging to this generation.
	length *atomic.Uint64
}

// blockCacheGeneration holds a per-block cache generation.
type blockCacheGeneration struct {
	// hashes maps per-block series ID with its hash.
	hashesMx sync.RWMutex
	hashes   map[storage.SeriesRef]uint64

	// Keeps track of the number of items added to the cache. This counter is
	// shared with all blockCacheGeneration in the "parent" cacheGeneration.
	length *atomic.Uint64
}

func newBlockCacheGeneration(length *atomic.Uint64) *blockCacheGeneration {
	return &blockCacheGeneration{
		hashes: make(map[storage.SeriesRef]uint64),
		length: length,
	}
}

type BlockSeriesHashCache struct {
	generations [numGenerations]*blockCacheGeneration
}

// Fetch the hash of the given seriesID from the cache and returns a boolean
// whether the series was found in the cache or not.
func (c *BlockSeriesHashCache) Fetch(seriesID storage.SeriesRef) (uint64, bool) {
	// Look for it in all generations, starting from the most recent one (index 0).
	for idx := 0; idx < numGenerations; idx++ {
		gen := c.generations[idx]

		// Skip if the cache doesn't exist for this generation.
		if gen == nil {
			continue
		}

		gen.hashesMx.RLock()
		value, ok := gen.hashes[seriesID]
		gen.hashesMx.RUnlock()

		if ok {
			return value, true
		}
	}

	return 0, false
}

// Store the hash of the given seriesID in the cache.
func (c *BlockSeriesHashCache) Store(seriesID storage.SeriesRef, hash uint64) {
	// Store it in the most recent generation (index 0).
	gen := c.generations[0]

	gen.hashesMx.Lock()
	gen.hashes[seriesID] = hash
	gen.hashesMx.Unlock()

	gen.length.Add(1)
}

type BlockSeriesHashCacheProvider struct {
	cache   *SeriesHashCache
	blockID string
}

// NewBlockSeriesHashCacheProvider makes a new BlockSeriesHashCacheProvider.
func NewBlockSeriesHashCacheProvider(cache *SeriesHashCache, blockID string) *BlockSeriesHashCacheProvider {
	return &BlockSeriesHashCacheProvider{
		cache:   cache,
		blockID: blockID,
	}
}

// SeriesHashCache returns a reference to the cache bounded to block provided
// to NewBlockSeriesHashCacheProvider().
func (p *BlockSeriesHashCacheProvider) SeriesHashCache() *BlockSeriesHashCache {
	return p.cache.GetBlockCache(p.blockID)
}
