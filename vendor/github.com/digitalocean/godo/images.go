package godo

import (
	"context"
	"fmt"
	"net/http"
)

const imageBasePath = "v2/images"

// ImagesService is an interface for interfacing with the images
// endpoints of the DigitalOcean API
// See: https://developers.digitalocean.com/documentation/v2#images
type ImagesService interface {
	List(context.Context, *ListOptions) ([]Image, *Response, error)
	ListDistribution(ctx context.Context, opt *ListOptions) ([]Image, *Response, error)
	ListApplication(ctx context.Context, opt *ListOptions) ([]Image, *Response, error)
	ListUser(ctx context.Context, opt *ListOptions) ([]Image, *Response, error)
	ListByTag(ctx context.Context, tag string, opt *ListOptions) ([]Image, *Response, error)
	GetByID(context.Context, int) (*Image, *Response, error)
	GetBySlug(context.Context, string) (*Image, *Response, error)
	Create(context.Context, *CustomImageCreateRequest) (*Image, *Response, error)
	Update(context.Context, int, *ImageUpdateRequest) (*Image, *Response, error)
	Delete(context.Context, int) (*Response, error)
}

// ImagesServiceOp handles communication with the image related methods of the
// DigitalOcean API.
type ImagesServiceOp struct {
	client *Client
}

var _ ImagesService = &ImagesServiceOp{}

// Image represents a DigitalOcean Image
type Image struct {
	ID            int      `json:"id,float64,omitempty"`
	Name          string   `json:"name,omitempty"`
	Type          string   `json:"type,omitempty"`
	Distribution  string   `json:"distribution,omitempty"`
	Slug          string   `json:"slug,omitempty"`
	Public        bool     `json:"public,omitempty"`
	Regions       []string `json:"regions,omitempty"`
	MinDiskSize   int      `json:"min_disk_size,omitempty"`
	SizeGigaBytes float64  `json:"size_gigabytes,omitempty"`
	Created       string   `json:"created_at,omitempty"`
	Description   string   `json:"description,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Status        string   `json:"status,omitempty"`
	ErrorMessage  string   `json:"error_message,omitempty"`
}

// ImageUpdateRequest represents a request to update an image.
type ImageUpdateRequest struct {
	Name string `json:"name"`
}

// CustomImageCreateRequest represents a request to create a custom image.
type CustomImageCreateRequest struct {
	Name         string   `json:"name"`
	Url          string   `json:"url"`
	Region       string   `json:"region"`
	Distribution string   `json:"distribution,omitempty"`
	Description  string   `json:"description,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type imageRoot struct {
	Image *Image
}

type imagesRoot struct {
	Images []Image
	Links  *Links `json:"links"`
	Meta   *Meta  `json:"meta"`
}

type listImageOptions struct {
	Private bool   `url:"private,omitempty"`
	Type    string `url:"type,omitempty"`
	Tag     string `url:"tag_name,omitempty"`
}

func (i Image) String() string {
	return Stringify(i)
}

// List lists all the images available.
func (s *ImagesServiceOp) List(ctx context.Context, opt *ListOptions) ([]Image, *Response, error) {
	return s.list(ctx, opt, nil)
}

// ListDistribution lists all the distribution images.
func (s *ImagesServiceOp) ListDistribution(ctx context.Context, opt *ListOptions) ([]Image, *Response, error) {
	listOpt := listImageOptions{Type: "distribution"}
	return s.list(ctx, opt, &listOpt)
}

// ListApplication lists all the application images.
func (s *ImagesServiceOp) ListApplication(ctx context.Context, opt *ListOptions) ([]Image, *Response, error) {
	listOpt := listImageOptions{Type: "application"}
	return s.list(ctx, opt, &listOpt)
}

// ListUser lists all the user images.
func (s *ImagesServiceOp) ListUser(ctx context.Context, opt *ListOptions) ([]Image, *Response, error) {
	listOpt := listImageOptions{Private: true}
	return s.list(ctx, opt, &listOpt)
}

// ListByTag lists all images with a specific tag applied.
func (s *ImagesServiceOp) ListByTag(ctx context.Context, tag string, opt *ListOptions) ([]Image, *Response, error) {
	listOpt := listImageOptions{Tag: tag}
	return s.list(ctx, opt, &listOpt)
}

// GetByID retrieves an image by id.
func (s *ImagesServiceOp) GetByID(ctx context.Context, imageID int) (*Image, *Response, error) {
	if imageID < 1 {
		return nil, nil, NewArgError("imageID", "cannot be less than 1")
	}

	return s.get(ctx, interface{}(imageID))
}

// GetBySlug retrieves an image by slug.
func (s *ImagesServiceOp) GetBySlug(ctx context.Context, slug string) (*Image, *Response, error) {
	if len(slug) < 1 {
		return nil, nil, NewArgError("slug", "cannot be blank")
	}

	return s.get(ctx, interface{}(slug))
}

func (s *ImagesServiceOp) Create(ctx context.Context, createRequest *CustomImageCreateRequest) (*Image, *Response, error) {
	if createRequest == nil {
		return nil, nil, NewArgError("createRequest", "cannot be nil")
	}

	req, err := s.client.NewRequest(ctx, http.MethodPost, imageBasePath, createRequest)
	if err != nil {
		return nil, nil, err
	}

	root := new(imageRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Image, resp, err
}

// Update an image name.
func (s *ImagesServiceOp) Update(ctx context.Context, imageID int, updateRequest *ImageUpdateRequest) (*Image, *Response, error) {
	if imageID < 1 {
		return nil, nil, NewArgError("imageID", "cannot be less than 1")
	}

	if updateRequest == nil {
		return nil, nil, NewArgError("updateRequest", "cannot be nil")
	}

	path := fmt.Sprintf("%s/%d", imageBasePath, imageID)
	req, err := s.client.NewRequest(ctx, http.MethodPut, path, updateRequest)
	if err != nil {
		return nil, nil, err
	}

	root := new(imageRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Image, resp, err
}

// Delete an image.
func (s *ImagesServiceOp) Delete(ctx context.Context, imageID int) (*Response, error) {
	if imageID < 1 {
		return nil, NewArgError("imageID", "cannot be less than 1")
	}

	path := fmt.Sprintf("%s/%d", imageBasePath, imageID)

	req, err := s.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(ctx, req, nil)

	return resp, err
}

// Helper method for getting an individual image
func (s *ImagesServiceOp) get(ctx context.Context, ID interface{}) (*Image, *Response, error) {
	path := fmt.Sprintf("%s/%v", imageBasePath, ID)

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(imageRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	return root.Image, resp, err
}

// Helper method for listing images
func (s *ImagesServiceOp) list(ctx context.Context, opt *ListOptions, listOpt *listImageOptions) ([]Image, *Response, error) {
	path := imageBasePath
	path, err := addOptions(path, opt)
	if err != nil {
		return nil, nil, err
	}
	path, err = addOptions(path, listOpt)
	if err != nil {
		return nil, nil, err
	}

	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}

	root := new(imagesRoot)
	resp, err := s.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Images, resp, err
}
