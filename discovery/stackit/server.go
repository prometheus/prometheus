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

package stackit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stackitcloud/stackit-sdk-go/core/auth"
	stackitconfig "github.com/stackitcloud/stackit-sdk-go/core/config"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	stackitAPIEndpoint = "https://iaas.api.%s.stackit.cloud"

	stackitLabelPrivateIPv4  = stackitLabelPrefix + "private_ipv4_"
	stackitLabelType         = stackitLabelPrefix + "type"
	stackitLabelLabel        = stackitLabelPrefix + "label_"
	stackitLabelLabelPresent = stackitLabelPrefix + "labelpresent_"
)

// Discovery periodically performs STACKIT Cloud requests.
// It implements the Discoverer interface.
type iaasDiscovery struct {
	*refresh.Discovery
	httpClient  *http.Client
	logger      *slog.Logger
	apiEndpoint string
	project     string
	port        int
}

// newServerDiscovery returns a new iaasDiscovery, which periodically refreshes its targets.
func newServerDiscovery(conf *SDConfig, logger *slog.Logger) (*iaasDiscovery, error) {
	d := &iaasDiscovery{
		project:     conf.Project,
		port:        conf.Port,
		apiEndpoint: conf.Endpoint,
		logger:      logger,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "stackit_sd")
	if err != nil {
		return nil, err
	}

	d.apiEndpoint = conf.Endpoint
	if d.apiEndpoint == "" {
		d.apiEndpoint = fmt.Sprintf(stackitAPIEndpoint, conf.Region)
	}

	servers := stackitconfig.ServerConfigurations{stackitconfig.ServerConfiguration{
		URL:         d.apiEndpoint,
		Description: "STACKIT IAAS API",
	}}

	d.httpClient = &http.Client{
		Timeout:   time.Duration(conf.RefreshInterval),
		Transport: rt,
	}

	stackitConfiguration := &stackitconfig.Configuration{
		UserAgent:  userAgent,
		HTTPClient: d.httpClient,
		Servers:    servers,
		NoAuth:     conf.ServiceAccountKey == "" && conf.ServiceAccountKeyPath == "",

		ServiceAccountKey:     conf.ServiceAccountKey,
		PrivateKey:            conf.PrivateKey,
		ServiceAccountKeyPath: conf.ServiceAccountKeyPath,
		PrivateKeyPath:        conf.PrivateKeyPath,
		CredentialsFilePath:   conf.CredentialsFilePath,
	}

	if conf.tokenURL != "" {
		stackitConfiguration.TokenCustomUrl = conf.tokenURL
	}

	authRoundTripper, err := auth.SetupAuth(stackitConfiguration)
	if err != nil {
		return nil, fmt.Errorf("setting up authentication: %w", err)
	}

	d.httpClient.Transport = authRoundTripper

	return d, nil
}

func (i *iaasDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	apiURL, err := url.Parse(i.apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid API endpoint URL %s: %w", i.apiEndpoint, err)
	}

	apiURL.Path, err = url.JoinPath(apiURL.Path, "v1", "projects", i.project, "servers")
	if err != nil {
		return nil, fmt.Errorf("joining URL path: %w", err)
	}

	q := apiURL.Query()
	q.Set("details", "true")
	apiURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	res, err := i.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errorMessage, _ := io.ReadAll(res.Body)

		return nil, fmt.Errorf("unexpected status code %d: %s", res.StatusCode, string(errorMessage))
	}

	var serversResponse *ServerListResponse

	if err := json.NewDecoder(res.Body).Decode(&serversResponse); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if serversResponse == nil || serversResponse.Items == nil || len(*serversResponse.Items) == 0 {
		return []*targetgroup.Group{{Source: "stackit", Targets: []model.LabelSet{}}}, nil
	}

	targets := make([]model.LabelSet, 0, len(*serversResponse.Items))
	for _, server := range *serversResponse.Items {
		if server.Nics == nil {
			i.logger.Debug("server has no network interfaces. Skipping", slog.String("server_id", server.ID))
			continue
		}

		labels := model.LabelSet{
			stackitLabelProject:          model.LabelValue(i.project),
			stackitLabelID:               model.LabelValue(server.ID),
			stackitLabelName:             model.LabelValue(server.Name),
			stackitLabelAvailabilityZone: model.LabelValue(server.AvailabilityZone),
			stackitLabelStatus:           model.LabelValue(server.Status),
			stackitLabelPowerStatus:      model.LabelValue(server.PowerStatus),
			stackitLabelType:             model.LabelValue(server.MachineType),
		}

		var (
			addressLabel   string
			serverPublicIP string
		)

		for _, nic := range server.Nics {
			if nic.PublicIP != nil && *nic.PublicIP != "" && serverPublicIP == "" {
				serverPublicIP = *nic.PublicIP
				addressLabel = serverPublicIP
			}

			if nic.IPv4 != nil && *nic.IPv4 != "" {
				networkLabel := model.LabelName(stackitLabelPrivateIPv4 + strutil.SanitizeLabelName(nic.NetworkName))
				labels[networkLabel] = model.LabelValue(*nic.IPv4)
				if addressLabel == "" {
					addressLabel = *nic.IPv4
				}
			}
		}

		if addressLabel == "" {
			// Skip servers without IPs.
			continue
		}

		// Public IPs for servers are optional.
		if serverPublicIP != "" {
			labels[stackitLabelPublicIPv4] = model.LabelValue(serverPublicIP)
		}

		labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(addressLabel, strconv.FormatUint(uint64(i.port), 10)))

		for labelKey, labelValue := range server.Labels {
			if labelStringValue, ok := labelValue.(string); ok {
				presentLabel := model.LabelName(stackitLabelLabelPresent + strutil.SanitizeLabelName(labelKey))
				labels[presentLabel] = "true"

				label := model.LabelName(stackitLabelLabel + strutil.SanitizeLabelName(labelKey))
				labels[label] = model.LabelValue(labelStringValue)
			}
		}

		targets = append(targets, labels)
	}

	return []*targetgroup.Group{{Source: "stackit", Targets: targets}}, nil
}
