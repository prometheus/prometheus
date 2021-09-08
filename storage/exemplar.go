// Copyright 2021 The Prometheus Authors
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

package storage

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/exemplar"
)

// CheckAppendExemplarError modifies the AppendExamplar's returned error based on the error cause.
func CheckAppendExemplarError(logger log.Logger, inErr error, e exemplar.Exemplar, outOfOrderCount prometheus.Counter, outOfOrderErrs *int) {
	var err error

	switch errors.Cause(inErr) {
	case ErrNotFound:
		err = ErrNotFound
	case ErrOutOfOrderExemplar:
		*outOfOrderErrs++
		_ = level.Debug(logger).Log("msg", "Out of order exemplar", "exemplar", fmt.Sprintf("%+v", e))
		if outOfOrderCount != nil {
			outOfOrderCount.Inc()
		}
	default:
		err = inErr
	}

	if err != nil {
		// Since exemplar storage is still experimental, we don't fail the scrape on ingestion errors.
		_ = level.Debug(logger).Log("msg", "Error while adding exemplar in AppendExemplar", "exemplar", fmt.Sprintf("%+v", e), "err", err)
	}
}
