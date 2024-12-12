// Copyright 2024 The Prometheus Authors
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

package batch

import (
	"errors"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
)

var ErrMissingTenant = errors.New("tsdbv2/batch: missing tenant ID")

// GetTenantID extracts a tenant ID frol labels.
//
// Tehant ID is a mapping between arbitrary blobs set in label with name tenant_id.
// Same blob will always return same integer value. The returned labels strips
// the tenant_id label.
//
// Labels without tenant_id key are valid and assigned the root tenant ID.
func (ba *Batch) GetTenantID(la labels.Labels) (uint64, labels.Labels, error) {
	for i := range la {
		if la[i].Name == encoding.TenantID {
			value := la[i].Value
			id, err := ba.translateBlob(encoding.RootTenant, fields.Tenant.Hash(), []byte(value))
			if err != nil {
				return 0, nil, err
			}
			bitmaps.Equality(ba.shards, ba.shard[id], id)
			return id, la, nil
		}
	}
	if ba.defaultRootTenant {
		return encoding.RootTenant, la, nil
	}
	return 0, nil, ErrMissingTenant
}
