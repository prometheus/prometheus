package cloudmap

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"net"
	"strings"

	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/servicediscovery"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	cloudMapLabel              = "" //model.MetaLabelPrefix + "cloudMap_"
	cloudMapLabelAZ            = cloudMapLabel + "availability_zone"
	cloudMapLabelInstanceID    = cloudMapLabel + "instance_id"
	cloudMapLabelInstanceState = cloudMapLabel + "instance_state"
	cloudMapLabelClusterName   = cloudMapLabel + "cluster_name"
	cloudMapLabelPrivateDNS    = cloudMapLabel + "private_dns_name"
	cloudMapLabelPrivateIP     = cloudMapLabel + "private_ip"
	cloudMapLabelAccountId     = cloudMapLabel + "account_id"
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


// Discovery periodically performs Cloud Map requests. It implements
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

	// Page through the namespaces in the Cloud Map directory
	err := mapper.ListNamespacesPages(&servicediscovery.ListNamespacesInput{},
		func(namespaceOutputPage *servicediscovery.ListNamespacesOutput, isLastPageOfNamespaces bool) bool {

			for _, namespace := range namespaceOutputPage.Namespaces {

				for {
					// Build a filter to select any services in the given namespace
					filter := servicediscovery.ServiceFilter { Name: &namespaceFilter, Values: []*string {namespace.Id} }

					err := mapper.ListServicesPages(&servicediscovery.ListServicesInput{Filters: []*servicediscovery.ServiceFilter{&filter}},
						func(servicesOutputPage *servicediscovery.ListServicesOutput, isLastPageOfServices bool) bool {

							for _, service := range servicesOutputPage.Services {

								err := mapper.ListInstancesPages(&servicediscovery.ListInstancesInput{ServiceId: service.Id},
									func(instancesOutputPage *servicediscovery.ListInstancesOutput, isLastPageOfInstances bool) bool {

										for _, instance := range instancesOutputPage.Instances {
											labels := model.LabelSet{
												cloudMapLabelInstanceID: model.LabelValue(*instance.Id),
											}

											labels[cloudMapLabelPrivateIP] = model.LabelValue(*instance.Attributes["AWS_INSTANCE_IPV4"])

											//if inst.PrivateDnsName != nil {
											//	labels[cloudMapLabelPrivateDNS] = model.LabelValue(*inst.PrivateDnsName) // Can be built from Service.Name + Namespace.Properties.HttpProperties.HttpName
											//}

											addr := net.JoinHostPort(*instance.Attributes["AWS_INSTANCE_IPV4"], fmt.Sprintf("%d", d.port))
											labels[model.AddressLabel] = model.LabelValue(addr)
											labels[cloudMapLabelAZ] = model.LabelValue(*instance.Attributes["AVAILABILITY_ZONE"])
											labels[cloudMapLabelInstanceState] = model.LabelValue(*instance.Attributes["AWS_INIT_HEALTH_STATUS"])
											labels[cloudMapLabelClusterName] = model.LabelValue(*instance.Attributes["ECS_CLUSTER_NAME"])
											labels[cloudMapLabelAccountId] = model.LabelValue(ParseAccountNumberFromArn(d.roleARN))

											tg.Targets = append(tg.Targets, labels)
										}

										return !isLastPageOfInstances
									})

								if err != nil {
									fmt.Println("could not list service instances")
									fmt.Println(err)
									return false
								}
							}

							return !isLastPageOfServices
						})

					if err != nil {
						fmt.Println("could not list namespace services, stopping")
						fmt.Println(err)
						return false
					}
				}
			}

			return !isLastPageOfNamespaces
		})

	if err != nil {
		fmt.Println("could not list directory namespaces")
		fmt.Println(err)
		return nil, errors.Wrap(err, "could not list directory namespaces")
	}

	return []*targetgroup.Group{tg}, nil
}

func ParseAccountNumberFromArn(arn string) string {
	arnParts := strings.Split(arn, ":")
	return arnParts[4]
}
