package local

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"
)

const maxMappedFP = 1 << 20 // About 1M fingerprints reserved for mapping.

var separatorString = string([]byte{clientmodel.SeparatorByte})

// fpMappings maps original fingerprints to a map of string representations of
// metrics to the truly unique fingerprint.
type fpMappings map[clientmodel.Fingerprint]map[string]clientmodel.Fingerprint

// fpMapper is used to map fingerprints in order to work around fingerprint
// collisions.
type fpMapper struct {
	mtx      sync.RWMutex // Protects mappings.
	mappings fpMappings

	fpToSeries      *seriesMap
	p               *persistence
	highestMappedFP clientmodel.Fingerprint
}

// newFPMapper loads the collision map from the persistence and
// returns an fpMapper ready to use.
func newFPMapper(fpToSeries *seriesMap, p *persistence) (*fpMapper, error) {
	r := &fpMapper{
		fpToSeries: fpToSeries,
		p:          p,
	}
	mappings, nextFP, err := p.loadFPMappings()
	if err != nil {
		return nil, err
	}
	r.mappings = mappings
	r.highestMappedFP = nextFP
	return r, nil
}

// mapFP takes a raw fingerprint (as returned by Metrics.FastFingerprint) and
// returns a truly unique fingerprint. The caller must have locked the raw
// fingerprint.
//
// If an error is encountered, it is returned together with the unchanged raw
// fingerprint.
func (r *fpMapper) mapFP(fp clientmodel.Fingerprint, m clientmodel.Metric) (clientmodel.Fingerprint, error) {
	// First check if we are in the reserved FP space, in which case this is
	// automatically a collision that has to be mapped.
	if fp <= maxMappedFP {
		return r.maybeAddMapping(fp, m)
	}

	// Then check the most likely case: This fp belongs to a series that is
	// already in memory.
	s, ok := r.fpToSeries.get(fp)
	if ok {
		// FP exists in memory, but is it for the same metric?
		if m.Equal(s.metric) {
			// Yupp. We are done.
			return fp, nil
		}
		// Collision detected!
		return r.maybeAddMapping(fp, m)
	}
	// Metric is not in memory. Before doing the expensive archive lookup,
	// check if we have a mapping for this metric in place already.
	r.mtx.RLock()
	mappedFPs, fpAlreadyMapped := r.mappings[fp]
	r.mtx.RUnlock()
	if fpAlreadyMapped {
		// We indeed have mapped fp historically.
		ms := metricToUniqueString(m)
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
	archivedMetric, err := r.p.archivedMetric(fp)
	if err != nil {
		return fp, err
	}
	if archivedMetric != nil {
		// FP exists in archive, but is it for the same metric?
		if m.Equal(archivedMetric) {
			// Yupp. We are done.
			return fp, nil
		}
		// Collision detected!
		return r.maybeAddMapping(fp, m)
	}
	// As fp does not exist, neither in memory nor in archive, we can safely
	// keep it unmapped.
	return fp, nil
}

// maybeAddMapping is only used internally. It takes a detected collision and
// adds it to the collisions map if not yet there. In any case, it returns the
// truly unique fingerprint for the colliding metric.
func (r *fpMapper) maybeAddMapping(
	fp clientmodel.Fingerprint,
	collidingMetric clientmodel.Metric,
) (clientmodel.Fingerprint, error) {
	ms := metricToUniqueString(collidingMetric)
	r.mtx.RLock()
	mappedFPs, ok := r.mappings[fp]
	r.mtx.RUnlock()
	if ok {
		// fp is locked by the caller, so no further locking required.
		mappedFP, ok := mappedFPs[ms]
		if ok {
			return mappedFP, nil // Existing mapping.
		}
		// A new mapping has to be created.
		mappedFP = r.nextMappedFP()
		mappedFPs[ms] = mappedFP
		r.mtx.RLock()
		// Checkpoint mappings after each change.
		err := r.p.checkpointFPMappings(r.mappings)
		r.mtx.RUnlock()
		glog.Infof(
			"Collision detected for fingerprint %v, metric %v, mapping to new fingerprint %v.",
			fp, collidingMetric, mappedFP,
		)
		return mappedFP, err
	}
	// This is the first collision for fp.
	mappedFP := r.nextMappedFP()
	mappedFPs = map[string]clientmodel.Fingerprint{ms: mappedFP}
	r.mtx.Lock()
	r.mappings[fp] = mappedFPs
	// Checkpoint mappings after each change.
	err := r.p.checkpointFPMappings(r.mappings)
	r.mtx.Unlock()
	glog.Infof(
		"Collision detected for fingerprint %v, metric %v, mapping to new fingerprint %v.",
		fp, collidingMetric, mappedFP,
	)
	return mappedFP, err
}

func (r *fpMapper) nextMappedFP() clientmodel.Fingerprint {
	mappedFP := clientmodel.Fingerprint(atomic.AddUint64((*uint64)(&r.highestMappedFP), 1))
	if mappedFP > maxMappedFP {
		panic(fmt.Errorf("more than %v fingerprints mapped in collision detection", maxMappedFP))
	}
	return mappedFP
}

// metricToUniqueString turns a metric into a string in a reproducible and
// unique way, i.e. the same metric will always create the same string, and
// different metrics will always create different strings. In a way, it is the
// "ideal" fingerprint function, only that it is more expensive than the
// FastFingerprint function, and its result is not suitable as a key for maps
// and indexes as it might become really large, causing a lot of hashing effort
// in maps and a lot of storage overhead in indexes.
func metricToUniqueString(m clientmodel.Metric) string {
	parts := make([]string, 0, len(m))
	for ln, lv := range m {
		parts = append(parts, string(ln)+separatorString+string(lv))
	}
	sort.Strings(parts)
	return strings.Join(parts, separatorString)
}
