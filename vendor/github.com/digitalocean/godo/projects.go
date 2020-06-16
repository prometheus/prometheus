package godo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
)

const (
	// DefaultProject is the ID you should use if you are working with your
	// default project.
	DefaultProject = "default"

	projectsBasePath = "/v2/projects"
)

// ProjectsService is an interface for creating and managing Projects with the DigitalOcean API.
// See: https://developers.digitalocean.com/documentation/v2/#projects
type ProjectsService interface {
	List(context.Context, *ListOptions) ([]Project, *Response, error)
	GetDefault(context.Context) (*Project, *Response, error)
	Get(context.Context, string) (*Project, *Response, error)
	Create(context.Context, *CreateProjectRequest) (*Project, *Response, error)
	Update(context.Context, string, *UpdateProjectRequest) (*Project, *Response, error)
	Delete(context.Context, string) (*Response, error)

	ListResources(context.Context, string, *ListOptions) ([]ProjectResource, *Response, error)
	AssignResources(context.Context, string, ...interface{}) ([]ProjectResource, *Response, error)
}

// ProjectsServiceOp handles communication with Projects methods of the DigitalOcean API.
type ProjectsServiceOp struct {
	client *Client
}

// Project represents a DigitalOcean Project configuration.
type Project struct {
	ID          string `json:"id"`
	OwnerUUID   string `json:"owner_uuid"`
	OwnerID     uint64 `json:"owner_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Purpose     string `json:"purpose"`
	Environment string `json:"environment"`
	IsDefault   bool   `json:"is_default"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// String creates a human-readable description of a Project.
func (p Project) String() string {
	return Stringify(p)
}

// CreateProjectRequest represents the request to create a new project.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Purpose     string `json:"purpose"`
	Environment string `json:"environment"`
}

// UpdateProjectRequest represents the request to update project information.
// This type expects certain attribute types, but is built this way to allow
// nil values as well. See `updateProjectRequest` for the "real" types.
type UpdateProjectRequest struct {
	Name        interface{}
	Description interface{}
	Purpose     interface{}
	Environment interface{}
	IsDefault   interface{}
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Purpose     *string `json:"purpose"`
	Environment *string `json:"environment"`
	IsDefault   *bool   `json:"is_default"`
}

// MarshalJSON takes an UpdateRequest and converts it to the "typed" request
// which is sent to the projects API. This is a PATCH request, which allows
// partial attributes, so `null` values are OK.
func (upr *UpdateProjectRequest) MarshalJSON() ([]byte, error) {
	d := &updateProjectRequest{}
	if str, ok := upr.Name.(string); ok {
		d.Name = &str
	}
	if str, ok := upr.Description.(string); ok {
		d.Description = &str
	}
	if str, ok := upr.Purpose.(string); ok {
		d.Purpose = &str
	}
	if str, ok := upr.Environment.(string); ok {
		d.Environment = &str
	}
	if val, ok := upr.IsDefault.(bool); ok {
		d.IsDefault = &val
	}

	return json.Marshal(d)
}

type assignResourcesRequest struct {
	Resources []string `json:"resources"`
}

// ProjectResource is the projects API's representation of a resource.
type ProjectResource struct {
	URN        string                `json:"urn"`
	AssignedAt string                `json:"assigned_at"`
	Links      *ProjectResourceLinks `json:"links"`
	Status     string                `json:"status,omitempty"`
}

// ProjetResourceLinks specify the link for more information about the resource.
type ProjectResourceLinks struct {
	Self string `json:"self"`
}

type projectsRoot struct {
	Projects []Project `json:"projects"`
	Links    *Links    `json:"links"`
	Meta     *Meta     `json:"meta"`
}

type projectRoot struct {
	Project *Project `json:"project"`
}

type projectResourcesRoot struct {
	Resources []ProjectResource `json:"resources"`
	Links     *Links            `json:"links,omitempty"`
	Meta      *Meta             `json:"meta"`
}

var _ ProjectsService = &ProjectsServiceOp{}

// List Projects.
func (p *ProjectsServiceOp) List(ctx context.Context, opts *ListOptions) ([]Project, *Response, error) {
	path, err := addOptions(projectsBasePath, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := p.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(projectsRoot)
	resp, err := p.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Projects, resp, err
}

// GetDefault project.
func (p *ProjectsServiceOp) GetDefault(ctx context.Context) (*Project, *Response, error) {
	return p.getHelper(ctx, "default")
}

// Get retrieves a single project by its ID.
func (p *ProjectsServiceOp) Get(ctx context.Context, projectID string) (*Project, *Response, error) {
	return p.getHelper(ctx, projectID)
}

// Create a new project.
func (p *ProjectsServiceOp) Create(ctx context.Context, cr *CreateProjectRequest) (*Project, *Response, error) {
	req, err := p.client.NewRequest(ctx, http.MethodPost, projectsBasePath, cr)
	if err != nil {
		return nil, nil, err
	}

	root := new(projectRoot)
	resp, err := p.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Project, resp, err
}

// Update an existing project.
func (p *ProjectsServiceOp) Update(ctx context.Context, projectID string, ur *UpdateProjectRequest) (*Project, *Response, error) {
	path := path.Join(projectsBasePath, projectID)
	req, err := p.client.NewRequest(ctx, http.MethodPatch, path, ur)
	if err != nil {
		return nil, nil, err
	}

	root := new(projectRoot)
	resp, err := p.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Project, resp, err
}

// Delete an existing project. You cannot have any resources in a project
// before deleting it. See the API documentation for more details.
func (p *ProjectsServiceOp) Delete(ctx context.Context, projectID string) (*Response, error) {
	path := path.Join(projectsBasePath, projectID)
	req, err := p.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	return p.client.Do(ctx, req, nil)
}

// ListResources lists all resources in a project.
func (p *ProjectsServiceOp) ListResources(ctx context.Context, projectID string, opts *ListOptions) ([]ProjectResource, *Response, error) {
	basePath := path.Join(projectsBasePath, projectID, "resources")
	path, err := addOptions(basePath, opts)
	if err != nil {
		return nil, nil, err
	}

	req, err := p.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(projectResourcesRoot)
	resp, err := p.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Resources, resp, err
}

// AssignResources assigns one or more resources to a project. AssignResources
// accepts resources in two possible formats:

//  1. The resource type, like `&Droplet{ID: 1}` or `&FloatingIP{IP: "1.2.3.4"}`
//  2. A valid DO URN as a string, like "do:droplet:1234"
//
// There is no unassign. To move a resource to another project, just assign
// it to that other project.
func (p *ProjectsServiceOp) AssignResources(ctx context.Context, projectID string, resources ...interface{}) ([]ProjectResource, *Response, error) {
	path := path.Join(projectsBasePath, projectID, "resources")

	ar := &assignResourcesRequest{
		Resources: make([]string, len(resources)),
	}

	for i, resource := range resources {
		switch resource := resource.(type) {
		case ResourceWithURN:
			ar.Resources[i] = resource.URN()
		case string:
			ar.Resources[i] = resource
		default:
			return nil, nil, fmt.Errorf("%T must either be a string or have a valid URN method", resource)
		}
	}
	req, err := p.client.NewRequest(ctx, http.MethodPost, path, ar)
	if err != nil {
		return nil, nil, err
	}

	root := new(projectResourcesRoot)
	resp, err := p.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	if l := root.Links; l != nil {
		resp.Links = l
	}

	return root.Resources, resp, err
}

func (p *ProjectsServiceOp) getHelper(ctx context.Context, projectID string) (*Project, *Response, error) {
	path := path.Join(projectsBasePath, projectID)

	req, err := p.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(projectRoot)
	resp, err := p.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Project, resp, err
}
