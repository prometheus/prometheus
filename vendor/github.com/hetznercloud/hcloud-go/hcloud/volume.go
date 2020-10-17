package hcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/hetznercloud/hcloud-go/hcloud/schema"
)

// Volume represents a volume in the Hetzner Cloud.
type Volume struct {
	ID          int
	Name        string
	Status      VolumeStatus
	Server      *Server
	Location    *Location
	Size        int
	Protection  VolumeProtection
	Labels      map[string]string
	LinuxDevice string
	Created     time.Time
}

// VolumeProtection represents the protection level of a volume.
type VolumeProtection struct {
	Delete bool
}

// VolumeClient is a client for the volume API.
type VolumeClient struct {
	client *Client
}

// VolumeStatus specifies a volume's status.
type VolumeStatus string

const (
	// VolumeStatusCreating is the status when a volume is being created.
	VolumeStatusCreating VolumeStatus = "creating"

	// VolumeStatusAvailable is the status when a volume is available.
	VolumeStatusAvailable VolumeStatus = "available"
)

// GetByID retrieves a volume by its ID. If the volume does not exist, nil is returned.
func (c *VolumeClient) GetByID(ctx context.Context, id int) (*Volume, *Response, error) {
	req, err := c.client.NewRequest(ctx, "GET", fmt.Sprintf("/volumes/%d", id), nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.VolumeGetResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		if IsError(err, ErrorCodeNotFound) {
			return nil, resp, nil
		}
		return nil, nil, err
	}
	return VolumeFromSchema(body.Volume), resp, nil
}

// GetByName retrieves a volume by its name. If the volume does not exist, nil is returned.
func (c *VolumeClient) GetByName(ctx context.Context, name string) (*Volume, *Response, error) {
	if name == "" {
		return nil, nil, nil
	}
	volumes, response, err := c.List(ctx, VolumeListOpts{Name: name})
	if len(volumes) == 0 {
		return nil, response, err
	}
	return volumes[0], response, err
}

// Get retrieves a volume by its ID if the input can be parsed as an integer, otherwise it
// retrieves a volume by its name. If the volume does not exist, nil is returned.
func (c *VolumeClient) Get(ctx context.Context, idOrName string) (*Volume, *Response, error) {
	if id, err := strconv.Atoi(idOrName); err == nil {
		return c.GetByID(ctx, int(id))
	}
	return c.GetByName(ctx, idOrName)
}

// VolumeListOpts specifies options for listing volumes.
type VolumeListOpts struct {
	ListOpts
	Name   string
	Status []VolumeStatus
}

func (l VolumeListOpts) values() url.Values {
	vals := l.ListOpts.values()
	if l.Name != "" {
		vals.Add("name", l.Name)
	}
	for _, status := range l.Status {
		vals.Add("status", string(status))
	}
	return vals
}

// List returns a list of volumes for a specific page.
//
// Please note that filters specified in opts are not taken into account
// when their value corresponds to their zero value or when they are empty.
func (c *VolumeClient) List(ctx context.Context, opts VolumeListOpts) ([]*Volume, *Response, error) {
	path := "/volumes?" + opts.values().Encode()
	req, err := c.client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, nil, err
	}

	var body schema.VolumeListResponse
	resp, err := c.client.Do(req, &body)
	if err != nil {
		return nil, nil, err
	}
	volumes := make([]*Volume, 0, len(body.Volumes))
	for _, s := range body.Volumes {
		volumes = append(volumes, VolumeFromSchema(s))
	}
	return volumes, resp, nil
}

// All returns all volumes.
func (c *VolumeClient) All(ctx context.Context) ([]*Volume, error) {
	return c.AllWithOpts(ctx, VolumeListOpts{ListOpts: ListOpts{PerPage: 50}})
}

// AllWithOpts returns all volumes with the given options.
func (c *VolumeClient) AllWithOpts(ctx context.Context, opts VolumeListOpts) ([]*Volume, error) {
	allVolumes := []*Volume{}

	_, err := c.client.all(func(page int) (*Response, error) {
		opts.Page = page
		volumes, resp, err := c.List(ctx, opts)
		if err != nil {
			return resp, err
		}
		allVolumes = append(allVolumes, volumes...)
		return resp, nil
	})
	if err != nil {
		return nil, err
	}

	return allVolumes, nil
}

// VolumeCreateOpts specifies parameters for creating a volume.
type VolumeCreateOpts struct {
	Name      string
	Size      int
	Server    *Server
	Location  *Location
	Labels    map[string]string
	Automount *bool
	Format    *string
}

// Validate checks if options are valid.
func (o VolumeCreateOpts) Validate() error {
	if o.Name == "" {
		return errors.New("missing name")
	}
	if o.Size <= 0 {
		return errors.New("size must be greater than 0")
	}
	if o.Server == nil && o.Location == nil {
		return errors.New("one of server or location must be provided")
	}
	if o.Server != nil && o.Location != nil {
		return errors.New("only one of server or location must be provided")
	}
	if o.Server == nil && (o.Automount != nil && *o.Automount) {
		return errors.New("server must be provided when automount is true")
	}
	return nil
}

// VolumeCreateResult is the result of creating a volume.
type VolumeCreateResult struct {
	Volume      *Volume
	Action      *Action
	NextActions []*Action
}

// Create creates a new volume with the given options.
func (c *VolumeClient) Create(ctx context.Context, opts VolumeCreateOpts) (VolumeCreateResult, *Response, error) {
	if err := opts.Validate(); err != nil {
		return VolumeCreateResult{}, nil, err
	}
	reqBody := schema.VolumeCreateRequest{
		Name:      opts.Name,
		Size:      opts.Size,
		Automount: opts.Automount,
		Format:    opts.Format,
	}
	if opts.Labels != nil {
		reqBody.Labels = &opts.Labels
	}
	if opts.Server != nil {
		reqBody.Server = Int(opts.Server.ID)
	}
	if opts.Location != nil {
		if opts.Location.ID != 0 {
			reqBody.Location = opts.Location.ID
		} else {
			reqBody.Location = opts.Location.Name
		}
	}

	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return VolumeCreateResult{}, nil, err
	}

	req, err := c.client.NewRequest(ctx, "POST", "/volumes", bytes.NewReader(reqBodyData))
	if err != nil {
		return VolumeCreateResult{}, nil, err
	}

	var respBody schema.VolumeCreateResponse
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return VolumeCreateResult{}, resp, err
	}

	var action *Action
	if respBody.Action != nil {
		action = ActionFromSchema(*respBody.Action)
	}

	return VolumeCreateResult{
		Volume:      VolumeFromSchema(respBody.Volume),
		Action:      action,
		NextActions: ActionsFromSchema(respBody.NextActions),
	}, resp, nil
}

// Delete deletes a volume.
func (c *VolumeClient) Delete(ctx context.Context, volume *Volume) (*Response, error) {
	req, err := c.client.NewRequest(ctx, "DELETE", fmt.Sprintf("/volumes/%d", volume.ID), nil)
	if err != nil {
		return nil, err
	}
	return c.client.Do(req, nil)
}

// VolumeUpdateOpts specifies options for updating a volume.
type VolumeUpdateOpts struct {
	Name   string
	Labels map[string]string
}

// Update updates a volume.
func (c *VolumeClient) Update(ctx context.Context, volume *Volume, opts VolumeUpdateOpts) (*Volume, *Response, error) {
	reqBody := schema.VolumeUpdateRequest{
		Name: opts.Name,
	}
	if opts.Labels != nil {
		reqBody.Labels = &opts.Labels
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/volumes/%d", volume.ID)
	req, err := c.client.NewRequest(ctx, "PUT", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.VolumeUpdateResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return VolumeFromSchema(respBody.Volume), resp, nil
}

// VolumeAttachOpts specifies options for attaching a volume.
type VolumeAttachOpts struct {
	Server    *Server
	Automount *bool
}

// AttachWithOpts attaches a volume to a server.
func (c *VolumeClient) AttachWithOpts(ctx context.Context, volume *Volume, opts VolumeAttachOpts) (*Action, *Response, error) {
	reqBody := schema.VolumeActionAttachVolumeRequest{
		Server:    opts.Server.ID,
		Automount: opts.Automount,
	}

	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/volumes/%d/actions/attach", volume.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	var respBody schema.VolumeActionAttachVolumeResponse
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, nil
}

// Attach attaches a volume to a server.
func (c *VolumeClient) Attach(ctx context.Context, volume *Volume, server *Server) (*Action, *Response, error) {
	return c.AttachWithOpts(ctx, volume, VolumeAttachOpts{Server: server})
}

// Detach detaches a volume from a server.
func (c *VolumeClient) Detach(ctx context.Context, volume *Volume) (*Action, *Response, error) {
	var reqBody schema.VolumeActionDetachVolumeRequest
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/volumes/%d/actions/detach", volume.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	var respBody schema.VolumeActionDetachVolumeResponse
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, nil
}

// VolumeChangeProtectionOpts specifies options for changing the resource protection level of a volume.
type VolumeChangeProtectionOpts struct {
	Delete *bool
}

// ChangeProtection changes the resource protection level of a volume.
func (c *VolumeClient) ChangeProtection(ctx context.Context, volume *Volume, opts VolumeChangeProtectionOpts) (*Action, *Response, error) {
	reqBody := schema.VolumeActionChangeProtectionRequest{
		Delete: opts.Delete,
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/volumes/%d/actions/change_protection", volume.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.VolumeActionChangeProtectionResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, err
}

// Resize changes the size of a volume.
func (c *VolumeClient) Resize(ctx context.Context, volume *Volume, size int) (*Action, *Response, error) {
	reqBody := schema.VolumeActionResizeVolumeRequest{
		Size: size,
	}
	reqBodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, err
	}

	path := fmt.Sprintf("/volumes/%d/actions/resize", volume.ID)
	req, err := c.client.NewRequest(ctx, "POST", path, bytes.NewReader(reqBodyData))
	if err != nil {
		return nil, nil, err
	}

	respBody := schema.VolumeActionResizeVolumeResponse{}
	resp, err := c.client.Do(req, &respBody)
	if err != nil {
		return nil, resp, err
	}
	return ActionFromSchema(respBody.Action), resp, err
}
