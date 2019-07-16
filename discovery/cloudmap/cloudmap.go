package cloudmap

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"net"

	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/service/servicediscovery"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	cloudmapLabel                = model.MetaLabelPrefix + "cloudmap_"
	cloudmapLabelAZ              = cloudmapLabel + "availability_zone"
	cloudmapLabelInstanceID      = cloudmapLabel + "instance_id"
	cloudmapLabelInstanceState   = cloudmapLabel + "instance_state"
	cloudmapLabelClusterName     = cloudmapLabel + "cluser_name"
	cloudmapLabelPrivateDNS      = cloudmapLabel + "private_dns_name"
	cloudmapLabelPrivateIP       = cloudmapLabel + "private_ip"
)

// DefaultSDConfig is the default EC2 SD configuration.
var DefaultSDConfig = SDConfig{
	Port:            5000,
	RefreshInterval: model.Duration(60 * time.Second),
}

// SDConfig is the configuration for EC2 based service discovery.
type SDConfig struct {
	RoleARN         string             `yaml:"role_arn,omitempty"`
	RefreshInterval model.Duration     `yaml:"refresh_interval,omitempty"`
	Port            int                `yaml:"port"`
	Accounts		[]string		   `yaml:"accounts,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return nil
}


// Discovery periodically performs Cloudmap requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	interval time.Duration
	roleARN  string
	port     int
}

func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {

	if logger == nil {
		logger = log.NewNopLogger()
	}

	d := &Discovery{
		roleARN:  conf.RoleARN,
		interval: time.Duration(conf.RefreshInterval),
		port:     conf.Port,
	}

	d.Discovery = refresh.NewDiscovery(
		logger,
		"cloudmap",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)

	return d
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {

	// Initial credentials loaded from SDK's default credential chain. Such as the environment,
	// shared credentials (~/.aws/credentials), or Instance Role. These credentials will be used
	// to to make the STS Assume Role API, and therefore need the sts:AssumeRole IAM permission
	sess := session.Must(session.NewSession())

	// Create the credentials from AssumeRoleProvider to assume the role provided in config
	creds := stscreds.NewCredentials(sess, d.roleARN)

	// Create service client value configured for credentials from assumed role.
	mapper := servicediscovery.New(sess, &aws.Config{Credentials: creds})


	namespaceFilter := "NAMESPACE_ID"

	tg := &targetgroup.Group{
		Source: "aws",
	}

	for {

		// List the namespaces in the cloud map directory
		nsResponse, err := mapper.ListNamespaces(&servicediscovery.ListNamespacesInput{})

		if err != nil {
			return nil, errors.Wrap(err, "could not list directory namespaces")
		}

		for _, namespace := range nsResponse.Namespaces {

			for {

				// Build a filter to select any services in the given namespace
				filter := servicediscovery.ServiceFilter { Name: &namespaceFilter, Values: []*string {namespace.Id} }

				svcResponse, err := mapper.ListServices(&servicediscovery.ListServicesInput{Filters: []*servicediscovery.ServiceFilter{&filter}})

				if err != nil {
					return nil, errors.Wrap(err, "could not list namespace services")
				}

				for _, service := range svcResponse.Services {

					for {

						instResponse, err := mapper.ListInstances(&servicediscovery.ListInstancesInput{ServiceId: service.Id})

						if err != nil {
							return nil, errors.Wrap(err, "could not list service instances")
						}

						for _, instance := range instResponse.Instances {

							labels := model.LabelSet{
								cloudmapLabelInstanceID: model.LabelValue(*instance.Id),
							}

							labels[cloudmapLabelPrivateIP] = model.LabelValue(*instance.Attributes["AWS_INSTANCE_IPV4"])

							//if inst.PrivateDnsName != nil {
							//	labels[cloudmapLabelPrivateDNS] = model.LabelValue(*inst.PrivateDnsName) // Can be built from Service.Name + Namespace.Properties.HttpProperties.HttpName
							//}

							addr := net.JoinHostPort(*instance.Attributes["AWS_INSTANCE_IPV4"], fmt.Sprintf("%d", d.port))
							labels[model.AddressLabel] = model.LabelValue(addr)

							labels[cloudmapLabelAZ] = model.LabelValue(*instance.Attributes["AVAILABILITY_ZONE"])
							labels[cloudmapLabelInstanceState] = model.LabelValue(*instance.Attributes["AWS_INIT_HEALTH_STATUS"])
							labels[cloudmapLabelClusterName] = model.LabelValue(*instance.Attributes["ECS_CLUSTER_NAME"])


							tg.Targets = append(tg.Targets, labels)

						}

						if instResponse.NextToken == nil {
							break;
						}

					}
				}

				if svcResponse.NextToken == nil {
					break;
				}
			}
		}

		if nsResponse.NextToken == nil {
			break;
		}
	}

	return []*targetgroup.Group{tg}, nil

}
