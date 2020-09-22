package schema

import "time"

// Volume defines the schema of a volume.
type Volume struct {
	ID          int               `json:"id"`
	Name        string            `json:"name"`
	Server      *int              `json:"server"`
	Status      string            `json:"status"`
	Location    Location          `json:"location"`
	Size        int               `json:"size"`
	Protection  VolumeProtection  `json:"protection"`
	Labels      map[string]string `json:"labels"`
	LinuxDevice string            `json:"linux_device"`
	Created     time.Time         `json:"created"`
}

// VolumeCreateRequest defines the schema of the request
// to create a volume.
type VolumeCreateRequest struct {
	Name      string             `json:"name"`
	Size      int                `json:"size"`
	Server    *int               `json:"server,omitempty"`
	Location  interface{}        `json:"location,omitempty"` // int, string, or nil
	Labels    *map[string]string `json:"labels,omitempty"`
	Automount *bool              `json:"automount,omitempty"`
	Format    *string            `json:"format,omitempty"`
}

// VolumeCreateResponse defines the schema of the response
// when creating a volume.
type VolumeCreateResponse struct {
	Volume      Volume   `json:"volume"`
	Action      *Action  `json:"action"`
	NextActions []Action `json:"next_actions"`
}

// VolumeListResponse defines the schema of the response
// when listing volumes.
type VolumeListResponse struct {
	Volumes []Volume `json:"volumes"`
}

// VolumeGetResponse defines the schema of the response
// when retrieving a single volume.
type VolumeGetResponse struct {
	Volume Volume `json:"volume"`
}

// VolumeUpdateRequest defines the schema of the request to update a volume.
type VolumeUpdateRequest struct {
	Name   string             `json:"name,omitempty"`
	Labels *map[string]string `json:"labels,omitempty"`
}

// VolumeUpdateResponse defines the schema of the response when updating a volume.
type VolumeUpdateResponse struct {
	Volume Volume `json:"volume"`
}

// VolumeProtection defines the schema of a volume's resource protection.
type VolumeProtection struct {
	Delete bool `json:"delete"`
}

// VolumeActionChangeProtectionRequest defines the schema of the request to
// change the resource protection of a volume.
type VolumeActionChangeProtectionRequest struct {
	Delete *bool `json:"delete,omitempty"`
}

// VolumeActionChangeProtectionResponse defines the schema of the response when
// changing the resource protection of a volume.
type VolumeActionChangeProtectionResponse struct {
	Action Action `json:"action"`
}

// VolumeActionAttachVolumeRequest defines the schema of the request to
// attach a volume to a server.
type VolumeActionAttachVolumeRequest struct {
	Server    int   `json:"server"`
	Automount *bool `json:"automount,omitempty"`
}

// VolumeActionAttachVolumeResponse defines the schema of the response when
// attaching a volume to a server.
type VolumeActionAttachVolumeResponse struct {
	Action Action `json:"action"`
}

// VolumeActionDetachVolumeRequest defines the schema of the request to
// create an detach volume action.
type VolumeActionDetachVolumeRequest struct{}

// VolumeActionDetachVolumeResponse defines the schema of the response when
// creating an detach volume action.
type VolumeActionDetachVolumeResponse struct {
	Action Action `json:"action"`
}

// VolumeActionResizeVolumeRequest defines the schema of the request to resize a volume.
type VolumeActionResizeVolumeRequest struct {
	Size int `json:"size"`
}

// VolumeActionResizeVolumeResponse defines the schema of the response when resizing a volume.
type VolumeActionResizeVolumeResponse struct {
	Action Action `json:"action"`
}
