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
	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"
)

type fingerprintDTOLessor chan *dto.Fingerprint

func (l fingerprintDTOLessor) lease() *dto.Fingerprint {
	select {
	case v := <-l:
		return v
	default:
		return &dto.Fingerprint{}
	}
}

func (l fingerprintDTOLessor) credit(d *dto.Fingerprint) {
	// d.Reset()

	select {
	case l <- d:
	default:
	}
}

func (l fingerprintDTOLessor) close() {
	close(l)

	for _ = range l {
	}
}

type fingerprintLessor chan *clientmodel.Fingerprint

func (l fingerprintLessor) lease() *clientmodel.Fingerprint {
	select {
	case v := <-l:
		return v
	default:
		return &clientmodel.Fingerprint{}
	}
}

func (l fingerprintLessor) credit(d *clientmodel.Fingerprint) {
	select {
	case l <- d:
	default:
	}
}

func (l fingerprintLessor) close() {
	close(l)

	for _ = range l {
	}
}

type highWatermarkDTOLessor chan *dto.MetricHighWatermark

func (l highWatermarkDTOLessor) lease() *dto.MetricHighWatermark {
	select {
	case v := <-l:
		return v
	default:
		return &dto.MetricHighWatermark{}
	}
}

func (l highWatermarkDTOLessor) credit(d *dto.MetricHighWatermark) {
	// d.Reset()

	select {
	case l <- d:
	default:
	}
}

func (l highWatermarkDTOLessor) close() {
	close(l)

	for _ = range l {
	}
}

type watermarksLessor chan *watermarks

func (l watermarksLessor) lease() *watermarks {
	select {
	case v := <-l:
		return v
	default:
		return &watermarks{}
	}
}

func (l watermarksLessor) credit(d *watermarks) {
	select {
	case l <- d:
	default:
	}
}

func (l watermarksLessor) close() {
	close(l)

	for _ = range l {
	}
}
