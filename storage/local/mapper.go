package local

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"

	"github.com/prometheus/common/model"
)

const maxMappedFP = 1 << 20 // About 1M fingerprints reserved for mapping.

var separatorString = string([]byte{model.SeparatorByte})

// fpMappings maps original fingerprints to a map of string representations of
// metrics to the truly unique fingerprint.
type fpMappings map[model.Fingerprint]map[string]model.Fingerprint

// fpMapper is used to map fingerprints in order to work around fingerprint
// collisions.
type fpMapper struct {
	// highestMappedFP has to be aligned for atomic operations.
	highestMappedFP model.Fingerprint

	mtx      sync.RWMutex // Protects mappings.
	mappings fpMappings

	fpToSeries *seriesMap
	p          *persistence

	mappingsCounter prometheus.Counter
}

// newFPMapper loads the collision map from the persistence and
// returns an fpMapper ready to use.
func newFPMapper(fpToSeries *seriesMap, p *persistence) (*fpMapper, error) {
	m := &fpMapper{
		fpToSeries: fpToSeries,
		p:          p,
		mappingsCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "fingerprint_mappings_total",
			Help:      "The total number of fingerprints being mapped to avoid collisions.",
		}),
	}
	mappings, nextFP, err := p.loadFPMappings()
	if err != nil {
		return nil, err
	}
	m.mappings = mappings
	m.mappingsCounter.Set(float64(len(m.mappings)))
	m.highestMappedFP = nextFP
	return m, nil
}

// mapFP takes a raw fingerprint (as returned by Metrics.FastFingerprint) and
// returns a truly unique fingerprint. The caller must have locked the raw
// fingerprint.
//
// If an error is encountered, it is returned together with the unchanged raw
// fingerprint.
func (m *fpMapper) mapFP(fp model.Fingerprint, metric model.Metric) (model.Fingerprint, error) {
	// First check if we are in the reserved FP space, in which case this is
	// automatically a collision that has to be mapped.
	if fp <= maxMappedFP {
		return m.maybeAddMapping(fp, metric)
	}

	// Then check the most likely case: This fp belongs to a series that is
	// already in memory.
	s, ok := m.fpToSeries.get(fp)
	if ok {
		// FP exists in memory, but is it for the same metric?
		if metric.Equal(s.metric) {
			// Yupp. We are done.
			return fp, nil
		}
		// Collision detected!
		return m.maybeAddMapping(fp, metric)
	}
	// Metric is not in memory. Before doing the expensive archive lookup,
	// check if we have a mapping for this metric in place already.
	m.mtx.RLock()
	mappedFPs, fpAlreadyMapped := m.mappings[fp]
	m.mtx.RUnlock()
	if fpAlreadyMapped {
		// We indeed have mapped fp historically.
		ms := metricToUniqueString(metric)
		// fp is locked by the caller, so no further locking of
		// 'collisions' required (it is specific to fp).
		mappedFP, ok := mappedFPs[ms]
		if ok {
			// Historical mapping found, return the mapped FP.
			return mappedFP, nil
		}
	}
	// If we are here, FP does not exist in memory and is either not mapped
	// at all, or existing mappings for FP are not for m. Check if we have
	// something for FP in the archive.
	archivedMetric, err := m.p.archivedMetric(fp)
	if err != nil {
		return fp, err
	}
	if archivedMetric != nil {
		// FP exists in archive, but is it for the same metric?
		if metric.Equal(archivedMetric) {
			// Yupp. We are done.
			return fp, nil
		}
		// Collision detected!
		return m.maybeAddMapping(fp, metric)
	}
	// As fp does not exist, neither in memory nor in archive, we can safely
	// keep it unmapped.
	return fp, nil
}

// maybeAddMapping is only used internally. It takes a detected collision and
// adds it to the collisions map if not yet there. In any case, it returns the
// truly unique fingerprint for the colliding metric.
func (m *fpMapper) maybeAddMapping(
	fp model.Fingerprint,
	collidingMetric model.Metric,
) (model.Fingerprint, error) {
	ms := metricToUniqueString(collidingMetric)
	m.mtx.RLock()
	mappedFPs, ok := m.mappings[fp]
	m.mtx.RUnlock()
	if ok {
		// fp is locked by the caller, so no further locking required.
		mappedFP, ok := mappedFPs[ms]
		if ok {
			return mappedFP, nil // Existing mapping.
		}
		// A new mapping has to be created.
		mappedFP = m.nextMappedFP()
		mappedFPs[ms] = mappedFP
		m.mtx.Lock()
		// Checkpoint mappings after each change.
		err := m.p.checkpointFPMappings(m.mappings)
		m.mtx.Unlock()
		log.Infof(
			"Collision detected for fingerprint %v, metric %v, mapping to new fingerprint %v.",
			fp, collidingMetric, mappedFP,
		)
		return mappedFP, err
	}
	// This is the first collision for fp.
	mappedFP := m.nextMappedFP()
	mappedFPs = map[string]model.Fingerprint{ms: mappedFP}
	m.mtx.Lock()
	m.mappings[fp] = mappedFPs
	m.mappingsCounter.Inc()
	// Checkpoint mappings after each change.
	err := m.p.checkpointFPMappings(m.mappings)
	m.mtx.Unlock()
	log.Infof(
		"Collision detected for fingerprint %v, metric %v, mapping to new fingerprint %v.",
		fp, collidingMetric, mappedFP,
	)
	return mappedFP, err
}

func (m *fpMapper) nextMappedFP() model.Fingerprint {
	mappedFP := model.Fingerprint(atomic.AddUint64((*uint64)(&m.highestMappedFP), 1))
	if mappedFP > maxMappedFP {
		panic(fmt.Errorf("more than %v fingerprints mapped in collision detection", maxMappedFP))
	}
	return mappedFP
}

// Describe implements prometheus.Collector.
func (m *fpMapper) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.mappingsCounter.Desc()
}

// Collect implements prometheus.Collector.
func (m *fpMapper) Collect(ch chan<- prometheus.Metric) {
	ch <- m.mappingsCounter
}

// metricToUniqueString turns a metric into a string in a reproducible and
// unique way, i.e. the same metric will always create the same string, and
// different metrics will always create different strings. In a way, it is the
// "ideal" fingerprint function, only that it is more expensive than the
// FastFingerprint function, and its result is not suitable as a key for maps
// and indexes as it might become really large, causing a lot of hashing effort
// in maps and a lot of storage overhead in indexes.
func metricToUniqueString(m model.Metric) string {
	parts := make([]string, 0, len(m))
	for ln, lv := range m {
		parts = append(parts, string(ln)+separatorString+string(lv))
	}
	sort.Strings(parts)
	return strings.Join(parts, separatorString)
}
