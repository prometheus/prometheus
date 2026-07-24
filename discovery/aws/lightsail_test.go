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

package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	lightsailTypes "github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/stretchr/testify/require"
)

func TestLightsailDiscoveryListInstancesPaginated(t *testing.T) {
	ctx := context.Background()

	pages := []*lightsail.GetInstancesOutput{
		{
			Instances:     []lightsailTypes.Instance{{Name: strptr("instance-1")}},
			NextPageToken: strptr("page-2"),
		},
		{
			Instances: []lightsailTypes.Instance{{Name: strptr("instance-2")}},
		},
	}

	var calls int
	discovery := &LightsailDiscovery{lightsail: &lightsailClientAdapter{
		getInstances: func(_ context.Context, params *lightsail.GetInstancesInput, _ ...func(*lightsail.Options)) (*lightsail.GetInstancesOutput, error) {
			if calls == 0 {
				require.Nil(t, params.PageToken)
			} else {
				require.Equal(t, "page-2", *params.PageToken)
			}
			out := pages[calls]
			calls++
			return out, nil
		},
	}}

	instances, err := discovery.listInstances(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, []lightsailTypes.Instance{
		{Name: strptr("instance-1")},
		{Name: strptr("instance-2")},
	}, instances)
}
