package azuresf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	apiVersion = "api-version=2.0"
)

type sfClient struct {
	origin string
	client *http.Client
}

func httpNotOKError(method string, code int) error {
	return fmt.Errorf("%s: expecting 200, got %d", method, code)
}

type getApplicationsResponse struct {
	Items []struct {
		ID string `json:"Id"`
	} `json:"Items"`
}

func (c *sfClient) getApplications(ctx context.Context) ([]string, error) {
	url := fmt.Sprintf("%s/Applications/?"+apiVersion, c.origin)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, httpNotOKError("getApplications", resp.StatusCode)
	}

	var appResp getApplicationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&appResp); err != nil {
		return nil, err
	}

	apps := make([]string, 0, len(appResp.Items))
	for _, item := range appResp.Items {
		apps = append(apps, item.ID)
	}

	return apps, nil
}

type getServicesResponse struct {
	Items []struct {
		ID string `json:"Id"`
	} `json:"Items"`
}

func (c *sfClient) getServices(ctx context.Context, application string) ([]string, error) {
	url := fmt.Sprintf("%s/Applications/%s/$/GetServices?"+apiVersion, c.origin, application)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, httpNotOKError("getServices", resp.StatusCode)
	}

	var servicesResp getServicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&servicesResp); err != nil {
		return nil, err
	}

	services := make([]string, 0, len(servicesResp.Items))
	for _, item := range servicesResp.Items {
		services = append(services, item.ID)
	}

	return services, nil
}

type getPartitionsResponse struct {
	Items []struct {
		Info struct {
			ID string `json:"Id"`
		} `json:"PartitionInformation"`
	} `json:"Items"`
}

func (c *sfClient) getPartitions(ctx context.Context, application, service string) ([]string, error) {
	url := fmt.Sprintf("%s/Applications/%s/$/GetServices/%s/$/GetPartitions?"+apiVersion, c.origin, application, url.QueryEscape(service))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req.WithContext(ctx))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, httpNotOKError("getPartitions", resp.StatusCode)
	}

	var partitionsResp getPartitionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&partitionsResp); err != nil {
		return nil, err
	}

	partitions := make([]string, 0, len(partitionsResp.Items))
	for _, item := range partitionsResp.Items {
		partitions = append(partitions, item.Info.ID)
	}

	return partitions, nil
}

type getReplicaResponse struct {
	Items []struct {
		NodeName string `json:"NodeName"`
		Address  string `json:"Address"`
	} `json:"Items"`
}

type getReplicaResponseEndpoints struct {
	Endpoints map[string]string `json:"Endpoints"`
}

type nodeAndEndpoints struct {
	node      string
	endpoints map[string]string
}

func (c *sfClient) getReplicaEndpoints(ctx context.Context, application, service, partition string) ([]nodeAndEndpoints, error) {
	url := fmt.Sprintf("%s/Applications/%s/$/GetServices/%s/$/GetPartitions/%s/$/GetReplicas?"+apiVersion, c.origin, application, url.QueryEscape(service), partition)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req.WithContext(ctx))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, httpNotOKError("getReplicaEndpoints", resp.StatusCode)
	}

	var replicaResp getReplicaResponse
	if err := json.NewDecoder(resp.Body).Decode(&replicaResp); err != nil {
		return nil, err
	}

	nodeEndpoints := make([]nodeAndEndpoints, 0, len(replicaResp.Items))
	for _, item := range replicaResp.Items {
		var endpoints getReplicaResponseEndpoints
		if err := json.Unmarshal([]byte(item.Address), &endpoints); err != nil {
			// there are other valid formats for endpoints (such as containers). Ignore this endpoint and move on
			// TODO - support additional formats
			continue
		}

		nodeEndpoints = append(nodeEndpoints, nodeAndEndpoints{node: item.NodeName, endpoints: endpoints.Endpoints})
	}

	return nodeEndpoints, nil
}
