// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.  // You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package prometheus

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// emptyHTTPClient must be used when no prometheus URL has been defined.
type emptyHTTPClient struct {
	MetadataService
}

func (c *emptyHTTPClient) MetricMetadata(_ context.Context, _ string) (v1.Metadata, error) {
	return v1.Metadata{}, nil
}

func (c *emptyHTTPClient) AllMetricMetadata(_ context.Context) (map[string][]v1.Metadata, error) {
	return make(map[string][]v1.Metadata), nil
}

func (c *emptyHTTPClient) LabelNames(_ context.Context, _ string) ([]string, error) {
	return []string{}, nil
}

func (c *emptyHTTPClient) LabelValues(_ context.Context, _ string) ([]model.LabelValue, error) {
	return []model.LabelValue{}, nil
}

func (c *emptyHTTPClient) ChangeDataSource(_ string) error {
	return fmt.Errorf("method not supported")
}

func (c *emptyHTTPClient) SetLookbackInterval(_ time.Duration) {

}

func (c *emptyHTTPClient) GetURL() string {
	return ""
}
