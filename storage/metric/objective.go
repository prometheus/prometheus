// Copyright 2013 Prometheus Team
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

package metric

import (
	"fmt"

	"github.com/golang/glog"

	"github.com/prometheus/prometheus/storage/raw/leveldb"

	dto "github.com/prometheus/prometheus/model/generated"
)

type iteratorSeekerState struct {
	// Immutable State
	i leveldb.Iterator

	obj *SampleKey

	first, last *SampleKey

	dtoSampleKeys *dtoSampleKeyList
	sampleKeys    *sampleKeyList

	// Mutable State
	iteratorInvalid bool
	seriesOperable  bool
	err             error

	key    *SampleKey
	keyDto *dto.SampleKey
}

// iteratorSeeker is a function that models a state machine state and
// is responsible for choosing the subsequent state given the present
// disposition.
//
// It returns the next state or nil if no remaining transition is possible.
// It returns an error if one occurred and finally a truth value indicating
// whether the current iterator state is usable and whether it can proceed with
// the current fingerprint.
type iteratorSeeker func() iteratorSeeker

func (s *iteratorSeekerState) initialize() iteratorSeeker {
	s.key, _ = s.sampleKeys.Get()
	s.keyDto, _ = s.dtoSampleKeys.Get()

	return s.start
}

func (s *iteratorSeekerState) destroy() iteratorSeeker {
	s.sampleKeys.Give(s.key)
	s.dtoSampleKeys.Give(s.keyDto)

	return nil
}

func (s *iteratorSeekerState) start() iteratorSeeker {
	switch {
	case s.obj.Fingerprint.Less(s.first.Fingerprint):
		// The fingerprint does not exist in the database.
		return s.destroy

	case s.last.Fingerprint.Less(s.obj.Fingerprint):
		// The fingerprint does not exist in the database.
		return s.destroy

	case s.obj.Fingerprint.Equal(s.first.Fingerprint) && s.obj.FirstTimestamp.Before(s.first.FirstTimestamp):
		// The fingerprint is the first fingerprint, but we've requested a value
		// before what exists in the database.
		return s.seekBeginning

	case s.last.Before(s.obj.Fingerprint, s.obj.FirstTimestamp):
		// The requested time for work is after the last sample in the database; we
		// can't do anything!
		return s.destroy

	default:
		return s.initialSeek
	}
}

func (s *iteratorSeekerState) seekBeginning() iteratorSeeker {
	s.i.SeekToFirst()
	if !s.i.Valid() {
		s.err = s.i.Error()
		// If we can't seek to the beginning, there isn't any hope for us.
		glog.Warning("iterator went bad: %s", s.err)
		s.iteratorInvalid = true
		return s.destroy
	}

	return s.initialMatchFingerprint
}

func (s *iteratorSeekerState) initialSeek() iteratorSeeker {
	s.obj.Dump(s.keyDto)

	s.i.Seek(s.keyDto)
	if !s.i.Valid() {
		s.err = s.i.Error()
		glog.Warningf("iterator went bad %s", s.err)
		s.iteratorInvalid = true
		return s.destroy
	}

	return s.initialMatchFingerprint
}

func (s *iteratorSeekerState) initialMatchFingerprint() iteratorSeeker {
	if err := s.i.Key(s.keyDto); err != nil {
		s.err = err
		return s.destroy
	}

	s.key.Load(s.keyDto)

	switch {
	case s.obj.Fingerprint.Less(s.key.Fingerprint):
		return s.initialFingerprintOvershot

	case s.key.Fingerprint.Less(s.obj.Fingerprint):
		panic("violated invariant")

	default:
		return s.initialMatchTime
	}
}

func (s *iteratorSeekerState) initialFingerprintOvershot() iteratorSeeker {
	s.i.Previous()
	if !s.i.Valid() {
		glog.Warningf("Could not backtrack for %s", s)
		panic("violated invariant")
	}

	if err := s.i.Key(s.keyDto); err != nil {
		s.err = err
		return s.destroy
	}

	s.key.Load(s.keyDto)

	if !s.key.Fingerprint.Equal(s.obj.Fingerprint) {
		return s.destroy
	}

	return s.initialMatchTime
}

func (s *iteratorSeekerState) initialMatchTime() iteratorSeeker {
	switch {
	case s.key.MayContain(s.obj.FirstTimestamp):
		s.seriesOperable = true
		return s.destroy

	case s.key.Equal(s.first), s.obj.FirstTimestamp.Equal(s.key.FirstTimestamp):
		s.seriesOperable = true
		return s.destroy

	case s.obj.FirstTimestamp.Before(s.key.FirstTimestamp):
		return s.reCueBackward

	case s.obj.FirstTimestamp.After(s.key.LastTimestamp):
		return s.reCueForward
	default:
		panic("violated invariant " + fmt.Sprintln(s.obj, s.key))
	}
}

func (s *iteratorSeekerState) reCueBackward() iteratorSeeker {
	s.i.Previous()
	if !s.i.Valid() {
		glog.Warningf("Could not backtrack for %s", s)
		panic("violated invariant")
	}

	if err := s.i.Key(s.keyDto); err != nil {
		s.err = err
		return s.destroy
	}

	s.key.Load(s.keyDto)

	if !s.key.Fingerprint.Equal(s.obj.Fingerprint) {
		return s.fastForward
	}

	s.seriesOperable = true
	return s.destroy
}

func (s *iteratorSeekerState) reCueForward() iteratorSeeker {
	if s.key.Equal(s.last) {
		glog.Info("Reached the end of the database; stopping.")
		return s.destroy
	}

	s.i.Next()
	if !s.i.Valid() {
		glog.Warningf("Could not poke forward for for %s", s)
		panic("violated invariant")
	}

	if err := s.i.Key(s.keyDto); err != nil {
		s.err = err
		return s.destroy
	}

	s.key.Load(s.keyDto)

	if !s.key.Fingerprint.Equal(s.obj.Fingerprint) {
		glog.Info("Poked forward and didn't match; aborting.")
		return s.destroy
	}

	glog.Info("Poked forward and found target; may be sign of bad FSM.")
	s.seriesOperable = true
	return s.destroy
}

func (s *iteratorSeekerState) fastForward() iteratorSeeker {
	s.i.Next()
	if !s.i.Valid() {
		glog.Warningf("Could not fast-forward for %s", s)
		panic("violated invariant")
	}

	s.seriesOperable = true
	return s.destroy
}
