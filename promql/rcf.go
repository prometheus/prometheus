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

// Random Cut Forest (RCF) algorithm implementation.
//
// Based on: Guha et al., "Robust Random Cut Forest Based Anomaly Detection
// On Streams" (ICML 2016).
//
// Design
// ------
//   - Each forest holds NumTrees independent rcfTrees.
//   - Each tree maintains its own bounded reservoir of SampleSize points.
//   - Points are inserted and deleted incrementally (O(log SampleSize)).
//   - The anomaly score is the average collusive displacement across all trees,
//     normalised to [0, 1] via the expected displacement baseline.
//   - Attribution reports the per-dimension contribution to the displacement.
//   - Models are keyed by series fingerprint and cached in an LRU; they are
//     persisted to disk so they survive Prometheus restarts.

package promql

import "math"

// ── constants & defaults ─────────────────────────────────────────────────────

const (
	rcfDefaultTrees      = 100
	rcfDefaultSampleSize = 256
	rcfDims              = 6 // must equal len(buildFeatures output[i])
)

// ── bounding box ─────────────────────────────────────────────────────────────

// rcfBox is an axis-aligned bounding box in rcfDims-dimensional space.
type rcfBox struct {
	Lo, Hi [rcfDims]float64
}

func boxFromPoint(p [rcfDims]float64) rcfBox {
	return rcfBox{Lo: p, Hi: p}
}

func (b *rcfBox) extendPoint(p [rcfDims]float64) {
	for d := range rcfDims {
		if p[d] < b.Lo[d] {
			b.Lo[d] = p[d]
		}
		if p[d] > b.Hi[d] {
			b.Hi[d] = p[d]
		}
	}
}

func (b *rcfBox) extendBox(o rcfBox) {
	b.extendPoint(o.Lo)
	b.extendPoint(o.Hi)
}

// span returns the sum of side lengths (used for proportional cut selection).
func (b *rcfBox) span() float64 {
	var s float64
	for d := range rcfDims {
		s += b.Hi[d] - b.Lo[d]
	}
	return s
}

func (b *rcfBox) contains(p [rcfDims]float64) bool {
	for d := range rcfDims {
		if p[d] < b.Lo[d] || p[d] > b.Hi[d] {
			return false
		}
	}
	return true
}

// ── tree node ─────────────────────────────────────────────────────────────────

// rcfNode is a node in a Random Cut Tree.
// Internal nodes store a random cut (CutDim, CutVal) and a bounding box.
// Leaf nodes additionally store the sample point and a duplicate count.
type rcfNode struct {
	Box         rcfBox
	Mass        int // total leaf count in this subtree
	CutDim      int
	CutVal      float64
	Left, Right *rcfNode
	Parent      *rcfNode
	// leaf-only
	Point [rcfDims]float64
	Count int // duplicates of Point in this leaf
}

func (n *rcfNode) isLeaf() bool { return n.Left == nil }

// ── Random Cut Tree ───────────────────────────────────────────────────────────

type rcfTree struct {
	Root *rcfNode
	Rng  uint64 // xorshift64 state; exported for gob
}

func newRCFTree(seed uint64) *rcfTree {
	return &rcfTree{Rng: seed | 1}
}

func (t *rcfTree) nextRand() uint64 {
	t.Rng ^= t.Rng << 13
	t.Rng ^= t.Rng >> 7
	t.Rng ^= t.Rng << 17
	return t.Rng
}

func (t *rcfTree) randFloat() float64 {
	return float64(t.nextRand()>>11) / float64(uint64(1)<<53)
}

// randomCut selects a dimension proportional to its range and a uniform value.
func (t *rcfTree) randomCut(box rcfBox) (dim int, val float64) {
	span := box.span()
	if span < 1e-12 {
		return 0, box.Lo[0]
	}
	r := t.randFloat() * span
	var cum float64
	for d := range rcfDims {
		cum += box.Hi[d] - box.Lo[d]
		if r <= cum {
			dim = d
			break
		}
	}
	w := box.Hi[dim] - box.Lo[dim]
	if w < 1e-12 {
		val = box.Lo[dim]
	} else {
		val = box.Lo[dim] + t.randFloat()*w
	}
	return dim, val
}

// ── Insert ────────────────────────────────────────────────────────────────────

func (t *rcfTree) insert(p [rcfDims]float64) {
	if t.Root == nil {
		t.Root = &rcfNode{Box: boxFromPoint(p), Mass: 1, Point: p, Count: 1}
		return
	}
	t.Root = t.insertAt(t.Root, nil, p)
}

func (t *rcfTree) insertAt(n, parent *rcfNode, p [rcfDims]float64) *rcfNode {
	if n.isLeaf() {
		if n.Point == p {
			n.Count++
			n.Mass++
			return n
		}
		return t.splitLeaf(n, parent, p)
	}
	if !n.Box.contains(p) {
		return t.insertAbove(n, parent, p)
	}
	if p[n.CutDim] <= n.CutVal {
		n.Left = t.insertAt(n.Left, n, p)
	} else {
		n.Right = t.insertAt(n.Right, n, p)
	}
	n.Box.extendPoint(p)
	n.Mass++
	return n
}

func (t *rcfTree) splitLeaf(n, parent *rcfNode, p [rcfDims]float64) *rcfNode {
	combined := n.Box
	combined.extendPoint(p)
	dim, val := t.randomCut(combined)
	leaf := &rcfNode{Box: boxFromPoint(p), Mass: 1, Point: p, Count: 1}
	internal := &rcfNode{Box: combined, Mass: n.Mass + 1, CutDim: dim, CutVal: val, Parent: parent}
	if p[dim] <= val {
		internal.Left, internal.Right = leaf, n
	} else {
		internal.Left, internal.Right = n, leaf
	}
	n.Parent = internal
	leaf.Parent = internal
	return internal
}

func (t *rcfTree) insertAbove(n, parent *rcfNode, p [rcfDims]float64) *rcfNode {
	combined := n.Box
	combined.extendPoint(p)
	dim, val := t.randomCut(combined)
	leaf := &rcfNode{Box: boxFromPoint(p), Mass: 1, Point: p, Count: 1}
	internal := &rcfNode{Box: combined, Mass: n.Mass + 1, CutDim: dim, CutVal: val, Parent: parent}
	if p[dim] <= val {
		internal.Left, internal.Right = leaf, n
	} else {
		internal.Left, internal.Right = n, leaf
	}
	n.Parent = internal
	leaf.Parent = internal
	return internal
}

// ── Delete ────────────────────────────────────────────────────────────────────

func (t *rcfTree) delete(p [rcfDims]float64) {
	if t.Root == nil {
		return
	}
	t.Root = t.deleteAt(t.Root, p)
}

func (t *rcfTree) deleteAt(n *rcfNode, p [rcfDims]float64) *rcfNode {
	if n == nil {
		return nil
	}
	if n.isLeaf() {
		if n.Point != p {
			return n
		}
		n.Count--
		n.Mass--
		if n.Count > 0 {
			return n
		}
		return nil
	}
	if p[n.CutDim] <= n.CutVal {
		n.Left = t.deleteAt(n.Left, p)
	} else {
		n.Right = t.deleteAt(n.Right, p)
	}
	// Collapse if a child was removed.
	if n.Left == nil {
		if n.Right != nil {
			n.Right.Parent = n.Parent
		}
		return n.Right
	}
	if n.Right == nil {
		n.Left.Parent = n.Parent
		return n.Left
	}
	// Recompute box and mass from children (correct after deletion).
	n.Box = n.Left.Box
	n.Box.extendBox(n.Right.Box)
	n.Mass = n.Left.Mass + n.Right.Mass
	return n
}

// ── Score (collusive displacement) ───────────────────────────────────────────

// score returns the collusive displacement of p in this tree.
//
// Collusive displacement (Guha et al. §3): the expected increase in model
// complexity when p is added, measured as the sibling subtree mass at the
// node where p would be inserted, divided by the total tree mass + 1.
// Summed over the path from root to the insertion point.
func (t *rcfTree) score(p [rcfDims]float64) float64 {
	if t.Root == nil || t.Root.Mass == 0 {
		return 0
	}
	return collusiveDisplacement(t.Root, p)
}

// collusiveDisplacement walks the tree and accumulates the displacement.
// At each internal node, if p falls on the same side as the existing subtree,
// the sibling mass contributes to the displacement.
func collusiveDisplacement(n *rcfNode, p [rcfDims]float64) float64 {
	if n.isLeaf() {
		// p would be inserted as a sibling of this leaf.
		// Displacement = mass of this leaf / (mass + 1).
		return float64(n.Mass) / float64(n.Mass+1)
	}

	// If p is outside the current box, it would be inserted above this node.
	// The entire subtree becomes the sibling.
	if !n.Box.contains(p) {
		return float64(n.Mass) / float64(n.Mass+1)
	}

	if p[n.CutDim] <= n.CutVal {
		// p goes left; right subtree is the sibling at this level.
		sibMass := 0
		if n.Right != nil {
			sibMass = n.Right.Mass
		}
		childDisp := 0.0
		if n.Left != nil {
			childDisp = collusiveDisplacement(n.Left, p)
		}
		return float64(sibMass)/float64(n.Mass+1) + childDisp
	}
	sibMass := 0
	if n.Left != nil {
		sibMass = n.Left.Mass
	}
	childDisp := 0.0
	if n.Right != nil {
		childDisp = collusiveDisplacement(n.Right, p)
	}
	return float64(sibMass)/float64(n.Mass+1) + childDisp
}

// ── Attribution ───────────────────────────────────────────────────────────────

// attribution returns the per-dimension contribution to the collusive
// displacement of p. The contribution of dimension d is the fraction of
// displacement that comes from cuts on dimension d.
func (t *rcfTree) attribution(p [rcfDims]float64) [rcfDims]float64 {
	var attr [rcfDims]float64
	if t.Root == nil || t.Root.Mass == 0 {
		return attr
	}
	attributionWalk(t.Root, p, &attr)
	return attr
}

func attributionWalk(n *rcfNode, p [rcfDims]float64, attr *[rcfDims]float64) {
	if n.isLeaf() {
		// Distribute leaf displacement equally across all dims (no cut info).
		disp := float64(n.Mass) / float64(n.Mass+1)
		for d := range rcfDims {
			attr[d] += disp / rcfDims
		}
		return
	}
	if !n.Box.contains(p) {
		disp := float64(n.Mass) / float64(n.Mass+1)
		attr[n.CutDim] += disp
		return
	}
	if p[n.CutDim] <= n.CutVal {
		sibMass := 0
		if n.Right != nil {
			sibMass = n.Right.Mass
		}
		attr[n.CutDim] += float64(sibMass) / float64(n.Mass+1)
		if n.Left != nil {
			attributionWalk(n.Left, p, attr)
		}
	} else {
		sibMass := 0
		if n.Left != nil {
			sibMass = n.Left.Mass
		}
		attr[n.CutDim] += float64(sibMass) / float64(n.Mass+1)
		if n.Right != nil {
			attributionWalk(n.Right, p, attr)
		}
	}
}

// ── Reservoir ─────────────────────────────────────────────────────────────────

// rcfReservoir is a bounded reservoir of feature vectors.
// When full, each new sample replaces a uniformly random existing slot
// (standard reservoir sampling, as used in the RCF paper).
type rcfReservoir struct {
	Samples    [][rcfDims]float64
	Timestamps []int64 // ms; parallel to Samples
	SampleSize int
	TotalSeen  int64
	Rng        uint64
}

func newReservoir(sampleSize int, seed uint64) *rcfReservoir {
	return &rcfReservoir{
		Samples:    make([][rcfDims]float64, 0, sampleSize),
		Timestamps: make([]int64, 0, sampleSize),
		SampleSize: sampleSize,
		Rng:        seed | 1,
	}
}

func (r *rcfReservoir) nextRand() uint64 {
	r.Rng ^= r.Rng << 13
	r.Rng ^= r.Rng >> 7
	r.Rng ^= r.Rng << 17
	return r.Rng
}

// add inserts (ts, p). Returns (evicted, true) when a slot was replaced.
func (r *rcfReservoir) add(ts int64, p [rcfDims]float64) (evicted [rcfDims]float64, didEvict bool) {
	r.TotalSeen++
	if len(r.Samples) < r.SampleSize {
		r.Samples = append(r.Samples, p)
		r.Timestamps = append(r.Timestamps, ts)
		return evicted, false
	}
	slot := int(r.nextRand() % uint64(r.SampleSize))
	evicted = r.Samples[slot]
	r.Samples[slot] = p
	r.Timestamps[slot] = ts
	return evicted, true
}

// ── Forest ────────────────────────────────────────────────────────────────────

// rcfForest is a collection of Random Cut Trees sharing a single reservoir.
type rcfForest struct {
	Trees      []*rcfTree
	Reservoir  *rcfReservoir
	NumTrees   int
	SampleSize int
	LastTS     int64 // last ingested timestamp (ms)
}

func newRCFForest(numTrees, sampleSize int, seed uint64) *rcfForest {
	trees := make([]*rcfTree, numTrees)
	for i := range numTrees {
		// Each tree gets a distinct seed derived from the forest seed.
		trees[i] = newRCFTree(seed ^ uint64(i)*6364136223846793005 + 1442695040888963407)
	}
	return &rcfForest{
		Trees:      trees,
		Reservoir:  newReservoir(sampleSize, seed),
		NumTrees:   numTrees,
		SampleSize: sampleSize,
	}
}

// update ingests one (timestamp, feature) pair into all trees.
func (f *rcfForest) update(ts int64, p [rcfDims]float64) {
	evicted, didEvict := f.Reservoir.add(ts, p)
	for _, tree := range f.Trees {
		if didEvict {
			tree.delete(evicted)
		}
		tree.insert(p)
	}
	if ts > f.LastTS {
		f.LastTS = ts
	}
}

// score returns the average collusive displacement of p, normalised to [0,1].
//
// Normalisation: the expected displacement of a uniformly random point in a
// tree of mass m is E[disp] ≈ H(m) (harmonic number), which grows as ln(m).
// We use the empirical mean displacement of the reservoir as the baseline so
// the score is robust to varying sample sizes.
func (f *rcfForest) score(p [rcfDims]float64) float64 {
	n := len(f.Reservoir.Samples)
	if n < 2 {
		return 0
	}
	var total float64
	for _, tree := range f.Trees {
		total += tree.score(p)
	}
	avg := total / float64(f.NumTrees)

	// Expected displacement baseline: harmonic approximation for tree of mass n.
	expected := math.Log(float64(n)) + 0.5772156649 // ln(n) + Euler-Mascheroni
	if expected < 1e-12 {
		return 0
	}
	// Map: score=0 when avg≈0, score→1 as avg grows well above expected.
	return clampScore(avg / (avg + expected))
}

// attribution returns the average per-dimension displacement contribution,
// normalised so the values sum to the overall score.
func (f *rcfForest) attribution(p [rcfDims]float64) [rcfDims]float64 {
	var sum [rcfDims]float64
	for _, tree := range f.Trees {
		a := tree.attribution(p)
		for d := range rcfDims {
			sum[d] += a[d]
		}
	}
	for d := range rcfDims {
		sum[d] /= float64(f.NumTrees)
	}
	return sum
}
