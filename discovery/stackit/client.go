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
	stackitIAASAPIEndpoint         = "https://iaas.api.%s.stackit.cloud"
	stackitPostgresAPIEndpoint     = "https://postgres-flex-service.api.stackit.cloud"
	stackitPostgresMetricsEndpoint = "https://postgres-flex-metrics.api.stackit.cloud/"

	stackitLabelPrivateIPv4  = stackitLabelPrefix + "private_ipv4_"
	stackitLabelType         = stackitLabelPrefix + "type"
	stackitLabelLabel        = stackitLabelPrefix + "label_"
	stackitLabelLabelPresent = stackitLabelPrefix + "labelpresent_"

	stackitSource             = "stackit"
	stackitPostgresType       = "postgres"
	stackitLabelPresentValue  = "true"
	stackitQueryParamDetails  = "details"
	contentTypeJSON           = "application/json"
	defaultHTTPPort           = "80"
	defaultHTTPSPort          = "443"
	schemeHTTPS               = "https"
	postgresMetricsPathFormat = "/v1alpha1/projects/%s/regions/%s/instances/%s/advanced/metrics"
)

type client interface {
	getInstances(ctx context.Context) ([]model.LabelSet, error)
	getPostgresInstances(ctx context.Context) ([]model.LabelSet, error)
}

type stackitClient struct {
	*refresh.Discovery
	httpClient          *http.Client
	project             string
	region              string
	role                Role
	port                int
	logger              *slog.Logger
	iaasAPIEndpoint     string
	postgresAPIEndpoint string
}

var _ client = &stackitClient{}

// newClient returns a new client, which periodically refreshes its targets.
func newClient(conf *SDConfig, logger *slog.Logger) (*stackitClient, error) {
	iaasEndpoint := conf.Endpoint
	if iaasEndpoint == "" {
		iaasEndpoint = fmt.Sprintf(stackitIAASAPIEndpoint, conf.Region)
	}

	postgresEndpoint := conf.Endpoint
	if postgresEndpoint == "" {
		postgresEndpoint = stackitPostgresAPIEndpoint
	}

	r := conf.Role
	if r == "" {
		r = RoleAll
	}

	c := &stackitClient{
		project:             conf.Project,
		region:              conf.Region,
		role:                r,
		port:                conf.Port,
		logger:              logger,
		iaasAPIEndpoint:     iaasEndpoint,
		postgresAPIEndpoint: postgresEndpoint,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "stackit_sd")
	if err != nil {
		return nil, err
	}

	servers := stackitconfig.ServerConfigurations{stackitconfig.ServerConfiguration{
		URL:         c.iaasAPIEndpoint,
		Description: "STACKIT IAAS API",
	}}

	c.httpClient = &http.Client{
		Timeout:   time.Duration(conf.RefreshInterval),
		Transport: rt,
	}

	stackitConfiguration := &stackitconfig.Configuration{
		UserAgent:  userAgent,
		HTTPClient: c.httpClient,
		Servers:    servers,
		NoAuth:     conf.ServiceAccountKey == "" && conf.ServiceAccountKeyPath == "",

		ServiceAccountKey:     string(conf.ServiceAccountKey),
		PrivateKey:            string(conf.PrivateKey),
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

	c.httpClient.Transport = authRoundTripper

	return c, nil
}

func (sc *stackitClient) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	var targets []model.LabelSet

	if sc.role == RoleServer || sc.role == RoleAll {
		serverTargets, err := sc.getInstances(ctx)
		if err != nil {
			return nil, err
		}
		targets = append(targets, serverTargets...)
	}

	if sc.role == RolePostgres || sc.role == RoleAll {
		postgresTargets, err := sc.getPostgresInstances(ctx)
		if err != nil {
			return nil, err
		}
		targets = append(targets, postgresTargets...)
	}

	return []*targetgroup.Group{{Source: stackitSource, Targets: targets}}, nil
}

func (sc *stackitClient) getInstances(ctx context.Context) ([]model.LabelSet, error) {
	reqURL, err := buildURL(sc.iaasAPIEndpoint, "v1", "projects", sc.project, "servers")
	if err != nil {
		return nil, err
	}

	var serversResponse ServerListResponse
	if err := sc.fetchJSON(ctx, reqURL, &serversResponse); err != nil {
		return nil, err
	}

	if serversResponse.Items == nil || len(*serversResponse.Items) == 0 {
		return []model.LabelSet{}, nil
	}

	targets := make([]model.LabelSet, 0, len(*serversResponse.Items))
	for _, server := range *serversResponse.Items {
		if server.Nics == nil {
			sc.logger.Debug("server has no network interfaces. Skipping", slog.String("server_id", server.ID))
			continue
		}

		labels := model.LabelSet{
			stackitLabelProject:          model.LabelValue(sc.project),
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

		labels[model.AddressLabel] = model.LabelValue(net.JoinHostPort(addressLabel, strconv.FormatUint(uint64(sc.port), 10)))

		for labelKey, labelValue := range server.Labels {
			if labelStringValue, ok := labelValue.(string); ok {
				presentLabel := model.LabelName(stackitLabelLabelPresent + strutil.SanitizeLabelName(labelKey))
				labels[presentLabel] = model.LabelValue(stackitLabelPresentValue)

				label := model.LabelName(stackitLabelLabel + strutil.SanitizeLabelName(labelKey))
				labels[label] = model.LabelValue(labelStringValue)
			}
		}

		targets = append(targets, labels)
	}

	return targets, nil
}

func (sc *stackitClient) getPostgresInstances(ctx context.Context) ([]model.LabelSet, error) {
	reqURL, err := buildURL(sc.postgresAPIEndpoint, "v2", "projects", sc.project, "regions", sc.region, "instances")
	if err != nil {
		return nil, err
	}

	var postgresListResponse PostgresListResponse
	if err := sc.fetchJSON(ctx, reqURL, &postgresListResponse); err != nil {
		return nil, err
	}

	if postgresListResponse.Items == nil || len(*postgresListResponse.Items) == 0 {
		return []model.LabelSet{}, nil
	}

	parsedURL, err := url.Parse(stackitPostgresMetricsEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid API endpoint URL %s: %w", stackitPostgresMetricsEndpoint, err)
	}

	host := parsedURL.Host
	if parsedURL.Port() == "" {
		if parsedURL.Scheme == schemeHTTPS {
			host = net.JoinHostPort(host, defaultHTTPSPort)
		} else {
			host = net.JoinHostPort(host, defaultHTTPPort)
		}
	}

	targets := make([]model.LabelSet, 0, len(*postgresListResponse.Items))
	for _, postgresInstance := range *postgresListResponse.Items {
		metricsPath := fmt.Sprintf(postgresMetricsPathFormat, sc.project, sc.region, postgresInstance.ID)
		labels := model.LabelSet{
			model.AddressLabel:     model.LabelValue(host),
			model.MetricsPathLabel: model.LabelValue(metricsPath),
			model.SchemeLabel:      model.LabelValue(parsedURL.Scheme),
			stackitLabelProject:    model.LabelValue(sc.project),
			stackitLabelID:         model.LabelValue(postgresInstance.ID),
			stackitLabelName:       model.LabelValue(postgresInstance.Name),
			stackitLabelStatus:     model.LabelValue(postgresInstance.Status),
			stackitLabelType:       model.LabelValue(stackitPostgresType),
		}

		targets = append(targets, labels)
	}

	return targets, nil
}

func buildURL(baseEndpoint string, pathSegments ...string) (string, error) {
	apiURL, err := url.Parse(baseEndpoint)
	if err != nil {
		return "", fmt.Errorf("invalid API endpoint URL %s: %w", baseEndpoint, err)
	}

	apiURL.Path, err = url.JoinPath(apiURL.Path, pathSegments...)
	if err != nil {
		return "", fmt.Errorf("joining URL path: %w", err)
	}

	q := apiURL.Query()
	q.Set(stackitQueryParamDetails, "true")
	apiURL.RawQuery = q.Encode()

	return apiURL.String(), nil
}

func (sc *stackitClient) fetchJSON(ctx context.Context, rawURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", contentTypeJSON)

	res, err := sc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		errorMessage, _ := io.ReadAll(res.Body)

		return fmt.Errorf("unexpected status code %d: %s", res.StatusCode, string(errorMessage))
	}

	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}
