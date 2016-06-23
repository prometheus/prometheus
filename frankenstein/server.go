// Copyright 2016 The Prometheus Authors
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

package frankenstein

import (
	"bytes"
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote/generic"
)

// AppenderHandler returns a http.Handler that accepts protobuf formatted
// metrics and sends them to the supplied appender.
func AppenderHandler(appender storage.SampleAppender) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := &generic.GenericWriteRequest{}
		buf := bytes.Buffer{}
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			log.Errorf(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = proto.Unmarshal(buf.Bytes(), req)
		if err != nil {
			log.Errorf(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, ts := range req.Timeseries {
			metric := model.Metric{}
			if ts.Name != nil {
				metric[model.MetricNameLabel] = model.LabelValue(ts.GetName())
			}
			for _, l := range ts.Labels {
				metric[model.LabelName(l.GetName())] = model.LabelValue(l.GetValue())
			}

			for _, s := range ts.Samples {
				err := appender.Append(&model.Sample{
					Metric:    metric,
					Value:     model.SampleValue(s.GetValue()),
					Timestamp: model.Time(s.GetTimestampMs()),
				})
				if err != nil {
					log.Errorf(err.Error())
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
