// Copyright 2015 The Prometheus Authors
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

package api

import (
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local"
)

type appendIngester struct {
	storage local.Storage
	count   *int
}

func (i appendIngester) Ingest(s clientmodel.Samples) error {
	for _, sample := range s {
		i.storage.Append(sample)
		*i.count++
	}
	return nil
}

// Import handles the /api/import endpoint.
func (serv MetricsService) Import(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, fmt.Sprintf("invalid method %s for data import, must be POST", r.Method), http.StatusBadRequest)
		return
	}

	processOptions := &extraction.ProcessOptions{
		Timestamp: clientmodel.Now(),
	}
	count := 0
	ingester := appendIngester{
		storage: serv.Storage,
		count:   &count,
	}
	processor, err := extraction.ProcessorForRequestHeader(r.Header)
	if err != nil {
		http.Error(w, fmt.Sprintf("couldn't select format processor: %s", err), http.StatusBadRequest)
		return
	}
	if err := processor.ProcessSingle(r.Body, ingester, processOptions); err != nil {
		http.Error(w, fmt.Sprintf("error processing imported data: %s", err), http.StatusInternalServerError)
		return
	}
	glog.Infof("Imported %d samples via HTTP API.", *ingester.count)
}
