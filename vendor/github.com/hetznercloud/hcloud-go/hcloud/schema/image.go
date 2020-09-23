package schema

import "time"

// Image defines the schema of an image.
type Image struct {
	ID          int               `json:"id"`
	Status      string            `json:"status"`
	Type        string            `json:"type"`
	Name        *string           `json:"name"`
	Description string            `json:"description"`
	ImageSize   *float32          `json:"image_size"`
	DiskSize    float32           `json:"disk_size"`
	Created     time.Time         `json:"created"`
	CreatedFrom *ImageCreatedFrom `json:"created_from"`
	BoundTo     *int              `json:"bound_to"`
	OSFlavor    string            `json:"os_flavor"`
	OSVersion   *string           `json:"os_version"`
	RapidDeploy bool              `json:"rapid_deploy"`
	Protection  ImageProtection   `json:"protection"`
	Deprecated  time.Time         `json:"deprecated"`
	Labels      map[string]string `json:"labels"`
}

// ImageProtection represents the protection level of a image.
type ImageProtection struct {
	Delete bool `json:"delete"`
}

// ImageCreatedFrom defines the schema of the images created from reference.
type ImageCreatedFrom struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ImageGetResponse defines the schema of the response when
// retrieving a single image.
type ImageGetResponse struct {
	Image Image `json:"image"`
}

// ImageListResponse defines the schema of the response when
// listing images.
type ImageListResponse struct {
	Images []Image `json:"images"`
}

// ImageUpdateRequest defines the schema of the request to update an image.
type ImageUpdateRequest struct {
	Description *string            `json:"description,omitempty"`
	Type        *string            `json:"type,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
}

// ImageUpdateResponse defines the schema of the response when updating an image.
type ImageUpdateResponse struct {
	Image Image `json:"image"`
}

// ImageActionChangeProtectionRequest defines the schema of the request to change the resource protection of an image.
type ImageActionChangeProtectionRequest struct {
	Delete *bool `json:"delete,omitempty"`
}

// ImageActionChangeProtectionResponse defines the schema of the response when changing the resource protection of an image.
type ImageActionChangeProtectionResponse struct {
	Action Action `json:"action"`
}
