package ecs

import (
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	awsecs "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/golang/mock/gomock"

	awsmock "github.com/prometheus/prometheus/discovery/ecs/mock/aws"
	"github.com/prometheus/prometheus/discovery/ecs/mock/aws/sdk"
)

func TestGetClusters(t *testing.T) {
	tests := []struct {
		clusters  []*awsecs.Cluster
		errorList bool
		errorDesc bool
		wantError bool
	}{
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
			},
		},
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
			},
			errorList: true,
			wantError: true,
		},
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
			},
			errorDesc: true,
			wantError: true,
		},
		{
			clusters: []*awsecs.Cluster{
				&awsecs.Cluster{ClusterArn: aws.String("c1")},
				&awsecs.Cluster{ClusterArn: aws.String("c2")},
				&awsecs.Cluster{ClusterArn: aws.String("c3")},
				&awsecs.Cluster{ClusterArn: aws.String("c4")},
			},
		},
	}

	for _, test := range tests {

		cIDs := make([]string, len(test.clusters))
		for i, cs := range test.clusters {
			cIDs[i] = aws.StringValue(cs.ClusterArn)
		}

		// Mock
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockECS := sdk.NewMockECSAPI(ctrl)
		awsmock.MockECSListClusters(t, mockECS, test.errorList, cIDs...)
		awsmock.MockECSDescribeClusters(t, mockECS, test.errorDesc, test.clusters...)

		r := &awsRetriever{
			client: mockECS,
		}

		res, err := r.getClusters()

		if !test.wantError {
			if len(res) != len(test.clusters) {
				t.Errorf("- %+v\n -The length of the retrieved clusters differ, want: %d; got: %d", test, len(test.clusters), len(res))
			}

			for i, got := range res {
				want := test.clusters[i]
				if !reflect.DeepEqual(want, got) {
					t.Errorf("\n- %v\n-  Received clusters from API is wrong, want: %v; got: %v", test, want, got)
				}
			}

		} else {
			if err == nil {
				t.Errorf("- %+v\n -Getting clusters shoud error, it didn't", test)
			}
		}
	}
}
