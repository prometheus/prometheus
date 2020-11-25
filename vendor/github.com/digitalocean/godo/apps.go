package godo

import (
	"context"
	"fmt"
	"net/http"
)

const (
	appsBasePath = "/v2/apps"
)

// AppLogType is the type of app logs.
type AppLogType string

const (
	// AppLogTypeBuild represents build logs.
	AppLogTypeBuild AppLogType = "BUILD"
	// AppLogTypeDeploy represents deploy logs.
	AppLogTypeDeploy AppLogType = "DEPLOY"
	// AppLogTypeRun represents run logs.
	AppLogTypeRun AppLogType = "RUN"
)

// AppsService is an interface for interfacing with the App Platform endpoints
// of the DigitalOcean API.
type AppsService interface {
	Create(ctx context.Context, create *AppCreateRequest) (*App, *Response, error)
	Get(ctx context.Context, appID string) (*App, *Response, error)
	List(ctx context.Context, opts *ListOptions) ([]*App, *Response, error)
	Update(ctx context.Context, appID string, update *AppUpdateRequest) (*App, *Response, error)
	Delete(ctx context.Context, appID string) (*Response, error)

	GetDeployment(ctx context.Context, appID, deploymentID string) (*Deployment, *Response, error)
	ListDeployments(ctx context.Context, appID string, opts *ListOptions) ([]*Deployment, *Response, error)
	CreateDeployment(ctx context.Context, appID string, create ...*DeploymentCreateRequest) (*Deployment, *Response, error)

	GetLogs(ctx context.Context, appID, deploymentID, component string, logType AppLogType, follow bool) (*AppLogs, *Response, error)

	ListRegions(ctx context.Context) ([]*AppRegion, *Response, error)

	ListTiers(ctx context.Context) ([]*AppTier, *Response, error)
	GetTier(ctx context.Context, slug string) (*AppTier, *Response, error)

	ListInstanceSizes(ctx context.Context) ([]*AppInstanceSize, *Response, error)
	GetInstanceSize(ctx context.Context, slug string) (*AppInstanceSize, *Response, error)
}

// AppLogs represent app logs.
type AppLogs struct {
	LiveURL      string   `json:"live_url"`
	HistoricURLs []string `json:"historic_urls"`
}

// AppCreateRequest represents a request to create an app.
type AppCreateRequest struct {
	Spec *AppSpec `json:"spec"`
}

// AppUpdateRequest represents a request to update an app.
type AppUpdateRequest struct {
	Spec *AppSpec `json:"spec"`
}

// DeploymentCreateRequest represents a request to create a deployment.
type DeploymentCreateRequest struct {
	ForceBuild bool `json:"force_build"`
}

type appRoot struct {
	App *App `json:"app"`
}

type appsRoot struct {
	Apps []*App `json:"apps"`
}

type deploymentRoot struct {
	Deployment *Deployment `json:"deployment"`
}

type deploymentsRoot struct {
	Deployments []*Deployment `json:"deployments"`
}

type appTierRoot struct {
	Tier *AppTier `json:"tier"`
}

type appTiersRoot struct {
	Tiers []*AppTier `json:"tiers"`
}

type instanceSizeRoot struct {
	InstanceSize *AppInstanceSize `json:"instance_size"`
}

type instanceSizesRoot struct {
	InstanceSizes []*AppInstanceSize `json:"instance_sizes"`
}

type appRegionsRoot struct {
	Regions []*AppRegion `json:"regions"`
}

// AppsServiceOp handles communication with Apps methods of the DigitalOcean API.
type AppsServiceOp struct {
	client *Client
}

// Create an app.
func (s *AppsServiceOp) Create(ctx context.Context, create *AppCreateRequest) (*App, *Response, error) {
	path := appsBasePath
	req, err := s.client.NewRequest(ctx, http.MethodPost, path, create)
	if err != nil {
		return nil, nil, err
	}

	root := new(appRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.App, resp, nil
}

// Get an app.
func (s *AppsServiceOp) Get(ctx context.Context, appID string) (*App, *Response, error) {
	path := fmt.Sprintf("%s/%s", appsBasePath, appID)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(appRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.App, resp, nil
}

// List apps.
func (s *AppsServiceOp) List(ctx context.Context, opts *ListOptions) ([]*App, *Response, error) {
	path := appsBasePath
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(appsRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Apps, resp, nil
}

// Update an app.
func (s *AppsServiceOp) Update(ctx context.Context, appID string, update *AppUpdateRequest) (*App, *Response, error) {
	path := fmt.Sprintf("%s/%s", appsBasePath, appID)
	req, err := s.client.NewRequest(ctx, http.MethodPut, path, update)
	if err != nil {
		return nil, nil, err
	}

	root := new(appRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.App, resp, nil
}

// Delete an app.
func (s *AppsServiceOp) Delete(ctx context.Context, appID string) (*Response, error) {
	path := fmt.Sprintf("%s/%s", appsBasePath, appID)
	req, err := s.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// GetDeployment gets an app deployment.
func (s *AppsServiceOp) GetDeployment(ctx context.Context, appID, deploymentID string) (*Deployment, *Response, error) {
	path := fmt.Sprintf("%s/%s/deployments/%s", appsBasePath, appID, deploymentID)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(deploymentRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Deployment, resp, nil
}

// ListDeployments lists an app deployments.
func (s *AppsServiceOp) ListDeployments(ctx context.Context, appID string, opts *ListOptions) ([]*Deployment, *Response, error) {
	path := fmt.Sprintf("%s/%s/deployments", appsBasePath, appID)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(deploymentsRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Deployments, resp, nil
}

// CreateDeployment creates an app deployment.
func (s *AppsServiceOp) CreateDeployment(ctx context.Context, appID string, create ...*DeploymentCreateRequest) (*Deployment, *Response, error) {
	path := fmt.Sprintf("%s/%s/deployments", appsBasePath, appID)

	var createReq *DeploymentCreateRequest
	for _, c := range create {
		createReq = c
	}

	req, err := s.client.NewRequest(ctx, http.MethodPost, path, createReq)
	if err != nil {
		return nil, nil, err
	}
	root := new(deploymentRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Deployment, resp, nil
}

// GetLogs retrieves app logs.
func (s *AppsServiceOp) GetLogs(ctx context.Context, appID, deploymentID, component string, logType AppLogType, follow bool) (*AppLogs, *Response, error) {
	url := fmt.Sprintf("%s/%s/deployments/%s/logs?type=%s&follow=%t", appsBasePath, appID, deploymentID, logType, follow)
	if component != "" {
		url = fmt.Sprintf("%s&component_name=%s", url, component)
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	logs := new(AppLogs)
	resp, err := s.client.Do(ctx, req, logs)
	if err != nil {
		return nil, resp, err
	}
	return logs, resp, nil
}

// ListRegions lists all regions supported by App Platform.
func (s *AppsServiceOp) ListRegions(ctx context.Context) ([]*AppRegion, *Response, error) {
	path := fmt.Sprintf("%s/regions", appsBasePath)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(appRegionsRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Regions, resp, nil
}

// ListTiers lists available app tiers.
func (s *AppsServiceOp) ListTiers(ctx context.Context) ([]*AppTier, *Response, error) {
	path := fmt.Sprintf("%s/tiers", appsBasePath)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(appTiersRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Tiers, resp, nil
}

// GetTier retrieves information about a specific app tier.
func (s *AppsServiceOp) GetTier(ctx context.Context, slug string) (*AppTier, *Response, error) {
	path := fmt.Sprintf("%s/tiers/%s", appsBasePath, slug)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(appTierRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Tier, resp, nil
}

// ListInstanceSizes lists available instance sizes for service, worker, and job components.
func (s *AppsServiceOp) ListInstanceSizes(ctx context.Context) ([]*AppInstanceSize, *Response, error) {
	path := fmt.Sprintf("%s/tiers/instance_sizes", appsBasePath)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(instanceSizesRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.InstanceSizes, resp, nil
}

// GetInstanceSize retreives information about a specific instance size for service, worker, and job components.
func (s *AppsServiceOp) GetInstanceSize(ctx context.Context, slug string) (*AppInstanceSize, *Response, error) {
	path := fmt.Sprintf("%s/tiers/instance_sizes/%s", appsBasePath, slug)
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(instanceSizeRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.InstanceSize, resp, nil
}
