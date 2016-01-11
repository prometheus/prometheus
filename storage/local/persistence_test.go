// Copyright 2014 The Prometheus Authors
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

package local

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/local/codable"
	"github.com/prometheus/prometheus/storage/local/index"
	"github.com/prometheus/prometheus/util/testutil"
)

var (
	m1 = model.Metric{"label": "value1"}
	m2 = model.Metric{"label": "value2"}
	m3 = model.Metric{"label": "value3"}
	m4 = model.Metric{"label": "value4"}
	m5 = model.Metric{"label": "value5"}
)

func newTestPersistence(t *testing.T, encoding chunkEncoding) (*persistence, testutil.Closer) {
	DefaultChunkEncoding = encoding
	dir := testutil.NewTemporaryDirectory("test_persistence", t)
	p, err := newPersistence(dir.Path(), false, false, func() bool { return false }, 0.1)
	if err != nil {
		dir.Close()
		t.Fatal(err)
	}
	go p.run()
	return p, testutil.NewCallbackCloser(func() {
		p.close()
		dir.Close()
	})
}

func buildTestChunks(encoding chunkEncoding) map[model.Fingerprint][]chunk {
	fps := model.Fingerprints{
		m1.FastFingerprint(),
		m2.FastFingerprint(),
		m3.FastFingerprint(),
	}
	fpToChunks := map[model.Fingerprint][]chunk{}

	for _, fp := range fps {
		fpToChunks[fp] = make([]chunk, 0, 10)
		for i := 0; i < 10; i++ {
			fpToChunks[fp] = append(fpToChunks[fp], newChunkForEncoding(encoding).add(&model.SamplePair{
				Timestamp: model.Time(i),
				Value:     model.SampleValue(fp),
			})[0])
		}
	}
	return fpToChunks
}

func chunksEqual(c1, c2 chunk) bool {
	values2 := c2.newIterator().values()
	for v1 := range c1.newIterator().values() {
		v2 := <-values2
		if !v1.Equal(v2) {
			return false
		}
	}
	return true
}

func testPersistLoadDropChunks(t *testing.T, encoding chunkEncoding) {
	p, closer := newTestPersistence(t, encoding)
	defer closer.Close()

	fpToChunks := buildTestChunks(encoding)

	for fp, chunks := range fpToChunks {
		firstTimeNotDropped, offset, numDropped, allDropped, err :=
			p.dropAndPersistChunks(fp, model.Earliest, chunks)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := firstTimeNotDropped, model.Time(0); got != want {
			t.Errorf("Want firstTimeNotDropped %v, got %v.", got, want)
		}
		if got, want := offset, 0; got != want {
			t.Errorf("Want offset %v, got %v.", got, want)
		}
		if got, want := numDropped, 0; got != want {
			t.Errorf("Want numDropped %v, got %v.", got, want)
		}
		if allDropped {
			t.Error("All dropped.")
		}
	}

	for fp, expectedChunks := range fpToChunks {
		indexes := make([]int, 0, len(expectedChunks))
		for i := range expectedChunks {
			indexes = append(indexes, i)
		}
		actualChunks, err := p.loadChunks(fp, indexes, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range indexes {
			if !chunksEqual(expectedChunks[i], actualChunks[i]) {
				t.Errorf("%d. Chunks not equal.", i)
			}
		}
		// Load all chunk descs.
		actualChunkDescs, err := p.loadChunkDescs(fp, 0)
		if len(actualChunkDescs) != 10 {
			t.Errorf("Got %d chunkDescs, want %d.", len(actualChunkDescs), 10)
		}
		for i, cd := range actualChunkDescs {
			if cd.firstTime() != model.Time(i) || cd.lastTime() != model.Time(i) {
				t.Errorf(
					"Want ts=%v, got firstTime=%v, lastTime=%v.",
					i, cd.firstTime(), cd.lastTime(),
				)
			}

		}
		// Load chunk descs partially.
		actualChunkDescs, err = p.loadChunkDescs(fp, 5)
		if len(actualChunkDescs) != 5 {
			t.Errorf("Got %d chunkDescs, want %d.", len(actualChunkDescs), 5)
		}
		for i, cd := range actualChunkDescs {
			if cd.firstTime() != model.Time(i) || cd.lastTime() != model.Time(i) {
				t.Errorf(
					"Want ts=%v, got firstTime=%v, lastTime=%v.",
					i, cd.firstTime(), cd.lastTime(),
				)
			}

		}
	}
	// Drop half of the chunks.
	for fp, expectedChunks := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 5, nil)
		if err != nil {
			t.Fatal(err)
		}
		if offset != 5 {
			t.Errorf("want offset 5, got %d", offset)
		}
		if firstTime != 5 {
			t.Errorf("want first time 5, got %d", firstTime)
		}
		if numDropped != 5 {
			t.Errorf("want 5 dropped chunks, got %v", numDropped)
		}
		if allDropped {
			t.Error("all chunks dropped")
		}
		indexes := make([]int, 5)
		for i := range indexes {
			indexes[i] = i
		}
		actualChunks, err := p.loadChunks(fp, indexes, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range indexes {
			if !chunksEqual(expectedChunks[i+5], actualChunks[i]) {
				t.Errorf("%d. Chunks not equal.", i)
			}
		}
	}
	// Drop all the chunks.
	for fp := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 100, nil)
		if firstTime != 0 {
			t.Errorf("want first time 0, got %d", firstTime)
		}
		if err != nil {
			t.Fatal(err)
		}
		if offset != 0 {
			t.Errorf("want offset 0, got %d", offset)
		}
		if numDropped != 5 {
			t.Errorf("want 5 dropped chunks, got %v", numDropped)
		}
		if !allDropped {
			t.Error("not all chunks dropped")
		}
	}
	// Re-add first two of the chunks.
	for fp, chunks := range fpToChunks {
		firstTimeNotDropped, offset, numDropped, allDropped, err :=
			p.dropAndPersistChunks(fp, model.Earliest, chunks[:2])
		if err != nil {
			t.Fatal(err)
		}
		if got, want := firstTimeNotDropped, model.Time(0); got != want {
			t.Errorf("Want firstTimeNotDropped %v, got %v.", got, want)
		}
		if got, want := offset, 0; got != want {
			t.Errorf("Want offset %v, got %v.", got, want)
		}
		if got, want := numDropped, 0; got != want {
			t.Errorf("Want numDropped %v, got %v.", got, want)
		}
		if allDropped {
			t.Error("All dropped.")
		}
	}
	// Drop the first of the chunks while adding two more.
	for fp, chunks := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 1, chunks[2:4])
		if err != nil {
			t.Fatal(err)
		}
		if offset != 1 {
			t.Errorf("want offset 1, got %d", offset)
		}
		if firstTime != 1 {
			t.Errorf("want first time 1, got %d", firstTime)
		}
		if numDropped != 1 {
			t.Errorf("want 1 dropped chunk, got %v", numDropped)
		}
		if allDropped {
			t.Error("all chunks dropped")
		}
		wantChunks := chunks[1:4]
		indexes := make([]int, len(wantChunks))
		for i := range indexes {
			indexes[i] = i
		}
		gotChunks, err := p.loadChunks(fp, indexes, 0)
		if err != nil {
			t.Fatal(err)
		}
		for i, wantChunk := range wantChunks {
			if !chunksEqual(wantChunk, gotChunks[i]) {
				t.Errorf("%d. Chunks not equal.", i)
			}
		}
	}
	// Drop all the chunks while adding two more.
	for fp, chunks := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 4, chunks[4:6])
		if err != nil {
			t.Fatal(err)
		}
		if offset != 0 {
			t.Errorf("want offset 0, got %d", offset)
		}
		if firstTime != 4 {
			t.Errorf("want first time 4, got %d", firstTime)
		}
		if numDropped != 3 {
			t.Errorf("want 3 dropped chunks, got %v", numDropped)
		}
		if allDropped {
			t.Error("all chunks dropped")
		}
		wantChunks := chunks[4:6]
		indexes := make([]int, len(wantChunks))
		for i := range indexes {
			indexes[i] = i
		}
		gotChunks, err := p.loadChunks(fp, indexes, 0)
		if err != nil {
			t.Fatal(err)
		}
		for i, wantChunk := range wantChunks {
			if !chunksEqual(wantChunk, gotChunks[i]) {
				t.Errorf("%d. Chunks not equal.", i)
			}
		}
	}
	// While adding two more, drop all but one of the added ones.
	for fp, chunks := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 7, chunks[6:8])
		if err != nil {
			t.Fatal(err)
		}
		if offset != 0 {
			t.Errorf("want offset 0, got %d", offset)
		}
		if firstTime != 7 {
			t.Errorf("want first time 7, got %d", firstTime)
		}
		if numDropped != 3 {
			t.Errorf("want 3 dropped chunks, got %v", numDropped)
		}
		if allDropped {
			t.Error("all chunks dropped")
		}
		wantChunks := chunks[7:8]
		indexes := make([]int, len(wantChunks))
		for i := range indexes {
			indexes[i] = i
		}
		gotChunks, err := p.loadChunks(fp, indexes, 0)
		if err != nil {
			t.Fatal(err)
		}
		for i, wantChunk := range wantChunks {
			if !chunksEqual(wantChunk, gotChunks[i]) {
				t.Errorf("%d. Chunks not equal.", i)
			}
		}
	}
	// While adding two more, drop all chunks including the added ones.
	for fp, chunks := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 10, chunks[8:])
		if err != nil {
			t.Fatal(err)
		}
		if offset != 0 {
			t.Errorf("want offset 0, got %d", offset)
		}
		if firstTime != 0 {
			t.Errorf("want first time 0, got %d", firstTime)
		}
		if numDropped != 3 {
			t.Errorf("want 3 dropped chunks, got %v", numDropped)
		}
		if !allDropped {
			t.Error("not all chunks dropped")
		}
	}
	// Now set minShrinkRatio to 0.25 and play with it.
	p.minShrinkRatio = 0.25
	// Re-add 8 chunks.
	for fp, chunks := range fpToChunks {
		firstTimeNotDropped, offset, numDropped, allDropped, err :=
			p.dropAndPersistChunks(fp, model.Earliest, chunks[:8])
		if err != nil {
			t.Fatal(err)
		}
		if got, want := firstTimeNotDropped, model.Time(0); got != want {
			t.Errorf("Want firstTimeNotDropped %v, got %v.", got, want)
		}
		if got, want := offset, 0; got != want {
			t.Errorf("Want offset %v, got %v.", got, want)
		}
		if got, want := numDropped, 0; got != want {
			t.Errorf("Want numDropped %v, got %v.", got, want)
		}
		if allDropped {
			t.Error("All dropped.")
		}
	}
	// Drop only the first chunk should not happen, but persistence should still work.
	for fp, chunks := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 1, chunks[8:9])
		if err != nil {
			t.Fatal(err)
		}
		if offset != 8 {
			t.Errorf("want offset 8, got %d", offset)
		}
		if firstTime != 0 {
			t.Errorf("want first time 0, got %d", firstTime)
		}
		if numDropped != 0 {
			t.Errorf("want 0 dropped chunk, got %v", numDropped)
		}
		if allDropped {
			t.Error("all chunks dropped")
		}
	}
	// Drop only the first two chunks should not happen, either.
	for fp := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 2, nil)
		if err != nil {
			t.Fatal(err)
		}
		if offset != 0 {
			t.Errorf("want offset 0, got %d", offset)
		}
		if firstTime != 0 {
			t.Errorf("want first time 0, got %d", firstTime)
		}
		if numDropped != 0 {
			t.Errorf("want 0 dropped chunk, got %v", numDropped)
		}
		if allDropped {
			t.Error("all chunks dropped")
		}
	}
	// Drop the first three chunks should finally work.
	for fp, chunks := range fpToChunks {
		firstTime, offset, numDropped, allDropped, err := p.dropAndPersistChunks(fp, 3, chunks[9:])
		if err != nil {
			t.Fatal(err)
		}
		if offset != 6 {
			t.Errorf("want offset 6, got %d", offset)
		}
		if firstTime != 3 {
			t.Errorf("want first time 3, got %d", firstTime)
		}
		if numDropped != 3 {
			t.Errorf("want 3 dropped chunk, got %v", numDropped)
		}
		if allDropped {
			t.Error("all chunks dropped")
		}
	}
}

func TestPersistLoadDropChunksType0(t *testing.T) {
	testPersistLoadDropChunks(t, 0)
}

func TestPersistLoadDropChunksType1(t *testing.T) {
	testPersistLoadDropChunks(t, 1)
}

func testCheckpointAndLoadSeriesMapAndHeads(t *testing.T, encoding chunkEncoding) {
	p, closer := newTestPersistence(t, encoding)
	defer closer.Close()

	fpLocker := newFingerprintLocker(10)
	sm := newSeriesMap()
	s1 := newMemorySeries(m1, nil, time.Time{})
	s2 := newMemorySeries(m2, nil, time.Time{})
	s3 := newMemorySeries(m3, nil, time.Time{})
	s4 := newMemorySeries(m4, nil, time.Time{})
	s5 := newMemorySeries(m5, nil, time.Time{})
	s1.add(&model.SamplePair{Timestamp: 1, Value: 3.14})
	s3.add(&model.SamplePair{Timestamp: 2, Value: 2.7})
	s3.headChunkClosed = true
	s3.persistWatermark = 1
	for i := 0; i < 10000; i++ {
		s4.add(&model.SamplePair{
			Timestamp: model.Time(i),
			Value:     model.SampleValue(i) / 2,
		})
		s5.add(&model.SamplePair{
			Timestamp: model.Time(i),
			Value:     model.SampleValue(i * i),
		})
	}
	s5.persistWatermark = 3
	chunkCountS4 := len(s4.chunkDescs)
	chunkCountS5 := len(s5.chunkDescs)
	sm.put(m1.FastFingerprint(), s1)
	sm.put(m2.FastFingerprint(), s2)
	sm.put(m3.FastFingerprint(), s3)
	sm.put(m4.FastFingerprint(), s4)
	sm.put(m5.FastFingerprint(), s5)

	if err := p.checkpointSeriesMapAndHeads(sm, fpLocker); err != nil {
		t.Fatal(err)
	}

	loadedSM, _, err := p.loadSeriesMapAndHeads()
	if err != nil {
		t.Fatal(err)
	}
	if loadedSM.length() != 4 {
		t.Errorf("want 4 series in map, got %d", loadedSM.length())
	}
	if loadedS1, ok := loadedSM.get(m1.FastFingerprint()); ok {
		if !reflect.DeepEqual(loadedS1.metric, m1) {
			t.Errorf("want metric %v, got %v", m1, loadedS1.metric)
		}
		if !reflect.DeepEqual(loadedS1.head().c, s1.head().c) {
			t.Error("head chunks differ")
		}
		if loadedS1.chunkDescsOffset != 0 {
			t.Errorf("want chunkDescsOffset 0, got %d", loadedS1.chunkDescsOffset)
		}
		if loadedS1.headChunkClosed {
			t.Error("headChunkClosed is true")
		}
	} else {
		t.Errorf("couldn't find %v in loaded map", m1)
	}
	if loadedS3, ok := loadedSM.get(m3.FastFingerprint()); ok {
		if !reflect.DeepEqual(loadedS3.metric, m3) {
			t.Errorf("want metric %v, got %v", m3, loadedS3.metric)
		}
		if loadedS3.head().c != nil {
			t.Error("head chunk not evicted")
		}
		if loadedS3.chunkDescsOffset != 0 {
			t.Errorf("want chunkDescsOffset 0, got %d", loadedS3.chunkDescsOffset)
		}
		if !loadedS3.headChunkClosed {
			t.Error("headChunkClosed is false")
		}
	} else {
		t.Errorf("couldn't find %v in loaded map", m3)
	}
	if loadedS4, ok := loadedSM.get(m4.FastFingerprint()); ok {
		if !reflect.DeepEqual(loadedS4.metric, m4) {
			t.Errorf("want metric %v, got %v", m4, loadedS4.metric)
		}
		if got, want := len(loadedS4.chunkDescs), chunkCountS4; got != want {
			t.Errorf("got %d chunkDescs, want %d", got, want)
		}
		if got, want := loadedS4.persistWatermark, 0; got != want {
			t.Errorf("got persistWatermark %d, want %d", got, want)
		}
		if loadedS4.chunkDescs[2].isEvicted() {
			t.Error("3rd chunk evicted")
		}
		if loadedS4.chunkDescs[3].isEvicted() {
			t.Error("4th chunk evicted")
		}
		if loadedS4.chunkDescsOffset != 0 {
			t.Errorf("want chunkDescsOffset 0, got %d", loadedS4.chunkDescsOffset)
		}
		if loadedS4.headChunkClosed {
			t.Error("headChunkClosed is true")
		}
	} else {
		t.Errorf("couldn't find %v in loaded map", m4)
	}
	if loadedS5, ok := loadedSM.get(m5.FastFingerprint()); ok {
		if !reflect.DeepEqual(loadedS5.metric, m5) {
			t.Errorf("want metric %v, got %v", m5, loadedS5.metric)
		}
		if got, want := len(loadedS5.chunkDescs), chunkCountS5; got != want {
			t.Errorf("got %d chunkDescs, want %d", got, want)
		}
		if got, want := loadedS5.persistWatermark, 3; got != want {
			t.Errorf("got persistWatermark %d, want %d", got, want)
		}
		if !loadedS5.chunkDescs[2].isEvicted() {
			t.Error("3rd chunk not evicted")
		}
		if loadedS5.chunkDescs[3].isEvicted() {
			t.Error("4th chunk evicted")
		}
		if loadedS5.chunkDescsOffset != 0 {
			t.Errorf("want chunkDescsOffset 0, got %d", loadedS5.chunkDescsOffset)
		}
		if loadedS5.headChunkClosed {
			t.Error("headChunkClosed is true")
		}
	} else {
		t.Errorf("couldn't find %v in loaded map", m5)
	}
}

func TestCheckpointAndLoadSeriesMapAndHeadsChunkType0(t *testing.T) {
	testCheckpointAndLoadSeriesMapAndHeads(t, 0)
}

func TestCheckpointAndLoadSeriesMapAndHeadsChunkType1(t *testing.T) {
	testCheckpointAndLoadSeriesMapAndHeads(t, 1)
}

func TestCheckpointAndLoadFPMappings(t *testing.T) {
	p, closer := newTestPersistence(t, 1)
	defer closer.Close()

	in := fpMappings{
		1: map[string]model.Fingerprint{
			"foo": 1,
			"bar": 2,
		},
		3: map[string]model.Fingerprint{
			"baz": 4,
		},
	}

	if err := p.checkpointFPMappings(in); err != nil {
		t.Fatal(err)
	}

	out, fp, err := p.loadFPMappings()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fp, model.Fingerprint(4); got != want {
		t.Errorf("got highest FP %v, want %v", got, want)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("got collision map %v, want %v", out, in)
	}
}

func testFingerprintsModifiedBefore(t *testing.T, encoding chunkEncoding) {
	p, closer := newTestPersistence(t, encoding)
	defer closer.Close()

	m1 := model.Metric{"n1": "v1"}
	m2 := model.Metric{"n2": "v2"}
	m3 := model.Metric{"n1": "v2"}
	p.archiveMetric(1, m1, 2, 4)
	p.archiveMetric(2, m2, 1, 6)
	p.archiveMetric(3, m3, 5, 5)

	expectedFPs := map[model.Time][]model.Fingerprint{
		0: {},
		1: {},
		2: {2},
		3: {1, 2},
		4: {1, 2},
		5: {1, 2},
		6: {1, 2, 3},
	}

	for ts, want := range expectedFPs {
		got, err := p.fingerprintsModifiedBefore(ts)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("timestamp: %v, want FPs %v, got %v", ts, want, got)
		}
	}

	unarchived, err := p.unarchiveMetric(1)
	if err != nil {
		t.Fatal(err)
	}
	if !unarchived {
		t.Error("expected actual unarchival")
	}
	unarchived, err = p.unarchiveMetric(1)
	if err != nil {
		t.Fatal(err)
	}
	if unarchived {
		t.Error("expected no unarchival")
	}

	expectedFPs = map[model.Time][]model.Fingerprint{
		0: {},
		1: {},
		2: {2},
		3: {2},
		4: {2},
		5: {2},
		6: {2, 3},
	}

	for ts, want := range expectedFPs {
		got, err := p.fingerprintsModifiedBefore(ts)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("timestamp: %v, want FPs %v, got %v", ts, want, got)
		}
	}
}

func TestFingerprintsModifiedBeforeChunkType0(t *testing.T) {
	testFingerprintsModifiedBefore(t, 0)
}

func TestFingerprintsModifiedBeforeChunkType1(t *testing.T) {
	testFingerprintsModifiedBefore(t, 1)
}

func testDropArchivedMetric(t *testing.T, encoding chunkEncoding) {
	p, closer := newTestPersistence(t, encoding)
	defer closer.Close()

	m1 := model.Metric{"n1": "v1"}
	m2 := model.Metric{"n2": "v2"}
	p.archiveMetric(1, m1, 2, 4)
	p.archiveMetric(2, m2, 1, 6)
	p.indexMetric(1, m1)
	p.indexMetric(2, m2)
	p.waitForIndexing()

	outFPs, err := p.fingerprintsForLabelPair(model.LabelPair{Name: "n1", Value: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	want := model.Fingerprints{1}
	if !reflect.DeepEqual(outFPs, want) {
		t.Errorf("want %#v, got %#v", want, outFPs)
	}
	outFPs, err = p.fingerprintsForLabelPair(model.LabelPair{Name: "n2", Value: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	want = model.Fingerprints{2}
	if !reflect.DeepEqual(outFPs, want) {
		t.Errorf("want %#v, got %#v", want, outFPs)
	}
	if archived, _, _, err := p.hasArchivedMetric(1); err != nil || !archived {
		t.Error("want FP 1 archived")
	}
	if archived, _, _, err := p.hasArchivedMetric(2); err != nil || !archived {
		t.Error("want FP 2 archived")
	}

	if err != p.purgeArchivedMetric(1) {
		t.Fatal(err)
	}
	if err != p.purgeArchivedMetric(3) {
		// Purging something that has not beet archived is not an error.
		t.Fatal(err)
	}
	p.waitForIndexing()

	outFPs, err = p.fingerprintsForLabelPair(model.LabelPair{Name: "n1", Value: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	want = nil
	if !reflect.DeepEqual(outFPs, want) {
		t.Errorf("want %#v, got %#v", want, outFPs)
	}
	outFPs, err = p.fingerprintsForLabelPair(model.LabelPair{Name: "n2", Value: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	want = model.Fingerprints{2}
	if !reflect.DeepEqual(outFPs, want) {
		t.Errorf("want %#v, got %#v", want, outFPs)
	}
	if archived, _, _, err := p.hasArchivedMetric(1); err != nil || archived {
		t.Error("want FP 1 not archived")
	}
	if archived, _, _, err := p.hasArchivedMetric(2); err != nil || !archived {
		t.Error("want FP 2 archived")
	}
}

func TestDropArchivedMetricChunkType0(t *testing.T) {
	testDropArchivedMetric(t, 0)
}

func TestDropArchivedMetricChunkType1(t *testing.T) {
	testDropArchivedMetric(t, 1)
}

type incrementalBatch struct {
	fpToMetric      index.FingerprintMetricMapping
	expectedLnToLvs index.LabelNameLabelValuesMapping
	expectedLpToFps index.LabelPairFingerprintsMapping
}

func testIndexing(t *testing.T, encoding chunkEncoding) {
	batches := []incrementalBatch{
		{
			fpToMetric: index.FingerprintMetricMapping{
				0: {
					model.MetricNameLabel: "metric_0",
					"label_1":             "value_1",
				},
				1: {
					model.MetricNameLabel: "metric_0",
					"label_2":             "value_2",
					"label_3":             "value_3",
				},
				2: {
					model.MetricNameLabel: "metric_1",
					"label_1":             "value_2",
				},
			},
			expectedLnToLvs: index.LabelNameLabelValuesMapping{
				model.MetricNameLabel: codable.LabelValueSet{
					"metric_0": struct{}{},
					"metric_1": struct{}{},
				},
				"label_1": codable.LabelValueSet{
					"value_1": struct{}{},
					"value_2": struct{}{},
				},
				"label_2": codable.LabelValueSet{
					"value_2": struct{}{},
				},
				"label_3": codable.LabelValueSet{
					"value_3": struct{}{},
				},
			},
			expectedLpToFps: index.LabelPairFingerprintsMapping{
				model.LabelPair{
					Name:  model.MetricNameLabel,
					Value: "metric_0",
				}: codable.FingerprintSet{0: struct{}{}, 1: struct{}{}},
				model.LabelPair{
					Name:  model.MetricNameLabel,
					Value: "metric_1",
				}: codable.FingerprintSet{2: struct{}{}},
				model.LabelPair{
					Name:  "label_1",
					Value: "value_1",
				}: codable.FingerprintSet{0: struct{}{}},
				model.LabelPair{
					Name:  "label_1",
					Value: "value_2",
				}: codable.FingerprintSet{2: struct{}{}},
				model.LabelPair{
					Name:  "label_2",
					Value: "value_2",
				}: codable.FingerprintSet{1: struct{}{}},
				model.LabelPair{
					Name:  "label_3",
					Value: "value_3",
				}: codable.FingerprintSet{1: struct{}{}},
			},
		}, {
			fpToMetric: index.FingerprintMetricMapping{
				3: {
					model.MetricNameLabel: "metric_0",
					"label_1":             "value_3",
				},
				4: {
					model.MetricNameLabel: "metric_2",
					"label_2":             "value_2",
					"label_3":             "value_1",
				},
				5: {
					model.MetricNameLabel: "metric_1",
					"label_1":             "value_3",
				},
			},
			expectedLnToLvs: index.LabelNameLabelValuesMapping{
				model.MetricNameLabel: codable.LabelValueSet{
					"metric_0": struct{}{},
					"metric_1": struct{}{},
					"metric_2": struct{}{},
				},
				"label_1": codable.LabelValueSet{
					"value_1": struct{}{},
					"value_2": struct{}{},
					"value_3": struct{}{},
				},
				"label_2": codable.LabelValueSet{
					"value_2": struct{}{},
				},
				"label_3": codable.LabelValueSet{
					"value_1": struct{}{},
					"value_3": struct{}{},
				},
			},
			expectedLpToFps: index.LabelPairFingerprintsMapping{
				model.LabelPair{
					Name:  model.MetricNameLabel,
					Value: "metric_0",
				}: codable.FingerprintSet{0: struct{}{}, 1: struct{}{}, 3: struct{}{}},
				model.LabelPair{
					Name:  model.MetricNameLabel,
					Value: "metric_1",
				}: codable.FingerprintSet{2: struct{}{}, 5: struct{}{}},
				model.LabelPair{
					Name:  model.MetricNameLabel,
					Value: "metric_2",
				}: codable.FingerprintSet{4: struct{}{}},
				model.LabelPair{
					Name:  "label_1",
					Value: "value_1",
				}: codable.FingerprintSet{0: struct{}{}},
				model.LabelPair{
					Name:  "label_1",
					Value: "value_2",
				}: codable.FingerprintSet{2: struct{}{}},
				model.LabelPair{
					Name:  "label_1",
					Value: "value_3",
				}: codable.FingerprintSet{3: struct{}{}, 5: struct{}{}},
				model.LabelPair{
					Name:  "label_2",
					Value: "value_2",
				}: codable.FingerprintSet{1: struct{}{}, 4: struct{}{}},
				model.LabelPair{
					Name:  "label_3",
					Value: "value_1",
				}: codable.FingerprintSet{4: struct{}{}},
				model.LabelPair{
					Name:  "label_3",
					Value: "value_3",
				}: codable.FingerprintSet{1: struct{}{}},
			},
		},
	}

	p, closer := newTestPersistence(t, encoding)
	defer closer.Close()

	indexedFpsToMetrics := index.FingerprintMetricMapping{}
	for i, b := range batches {
		for fp, m := range b.fpToMetric {
			p.indexMetric(fp, m)
			if err := p.archiveMetric(fp, m, 1, 2); err != nil {
				t.Fatal(err)
			}
			indexedFpsToMetrics[fp] = m
		}
		verifyIndexedState(i, t, b, indexedFpsToMetrics, p)
	}

	for i := len(batches) - 1; i >= 0; i-- {
		b := batches[i]
		verifyIndexedState(i, t, batches[i], indexedFpsToMetrics, p)
		for fp, m := range b.fpToMetric {
			p.unindexMetric(fp, m)
			unarchived, err := p.unarchiveMetric(fp)
			if err != nil {
				t.Fatal(err)
			}
			if !unarchived {
				t.Errorf("%d. metric not unarchived", i)
			}
			delete(indexedFpsToMetrics, fp)
		}
	}
}

func TestIndexingChunkType0(t *testing.T) {
	testIndexing(t, 0)
}

func TestIndexingChunkType1(t *testing.T) {
	testIndexing(t, 1)
}

func verifyIndexedState(i int, t *testing.T, b incrementalBatch, indexedFpsToMetrics index.FingerprintMetricMapping, p *persistence) {
	p.waitForIndexing()
	for fp, m := range indexedFpsToMetrics {
		// Compare archived metrics with input metrics.
		mOut, err := p.archivedMetric(fp)
		if err != nil {
			t.Fatal(err)
		}
		if !mOut.Equal(m) {
			t.Errorf("%d. %v: Got: %s; want %s", i, fp, mOut, m)
		}

		// Check that archived metrics are in membership index.
		has, first, last, err := p.hasArchivedMetric(fp)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Errorf("%d. fingerprint %v not found", i, fp)
		}
		if first != 1 || last != 2 {
			t.Errorf(
				"%d. %v: Got first: %d, last %d; want first: %d, last %d",
				i, fp, first, last, 1, 2,
			)
		}
	}

	// Compare label name -> label values mappings.
	for ln, lvs := range b.expectedLnToLvs {
		outLvs, err := p.labelValuesForLabelName(ln)
		if err != nil {
			t.Fatal(err)
		}

		outSet := codable.LabelValueSet{}
		for _, lv := range outLvs {
			outSet[lv] = struct{}{}
		}

		if !reflect.DeepEqual(lvs, outSet) {
			t.Errorf("%d. label values don't match. Got: %v; want %v", i, outSet, lvs)
		}
	}

	// Compare label pair -> fingerprints mappings.
	for lp, fps := range b.expectedLpToFps {
		outFPs, err := p.fingerprintsForLabelPair(lp)
		if err != nil {
			t.Fatal(err)
		}

		outSet := codable.FingerprintSet{}
		for _, fp := range outFPs {
			outSet[fp] = struct{}{}
		}

		if !reflect.DeepEqual(fps, outSet) {
			t.Errorf("%d. %v: fingerprints don't match. Got: %v; want %v", i, lp, outSet, fps)
		}
	}
}

var fpStrings = []string{
	"b004b821ca50ba26",
	"b037c21e884e4fc5",
	"b037de1e884e5469",
}

func BenchmarkLoadChunksSequentially(b *testing.B) {
	p := persistence{
		basePath: "fixtures",
		bufPool:  sync.Pool{New: func() interface{} { return make([]byte, 0, 3*chunkLenWithHeader) }},
	}
	sequentialIndexes := make([]int, 47)
	for i := range sequentialIndexes {
		sequentialIndexes[i] = i
	}

	var fp model.Fingerprint
	for i := 0; i < b.N; i++ {
		for _, s := range fpStrings {
			fp, _ = model.FingerprintFromString(s)
			cds, err := p.loadChunks(fp, sequentialIndexes, 0)
			if err != nil {
				b.Error(err)
			}
			if len(cds) == 0 {
				b.Error("could not read any chunks")
			}
		}
	}
}

func BenchmarkLoadChunksRandomly(b *testing.B) {
	p := persistence{
		basePath: "fixtures",
		bufPool:  sync.Pool{New: func() interface{} { return make([]byte, 0, 3*chunkLenWithHeader) }},
	}
	randomIndexes := []int{1, 5, 6, 8, 11, 14, 18, 23, 29, 33, 42, 46}

	var fp model.Fingerprint
	for i := 0; i < b.N; i++ {
		for _, s := range fpStrings {
			fp, _ = model.FingerprintFromString(s)
			cds, err := p.loadChunks(fp, randomIndexes, 0)
			if err != nil {
				b.Error(err)
			}
			if len(cds) == 0 {
				b.Error("could not read any chunks")
			}
		}
	}
}

func BenchmarkLoadChunkDescs(b *testing.B) {
	p := persistence{
		basePath: "fixtures",
	}

	var fp model.Fingerprint
	for i := 0; i < b.N; i++ {
		for _, s := range fpStrings {
			fp, _ = model.FingerprintFromString(s)
			cds, err := p.loadChunkDescs(fp, 0)
			if err != nil {
				b.Error(err)
			}
			if len(cds) == 0 {
				b.Error("could not read any chunk descs")
			}
		}
	}
}
