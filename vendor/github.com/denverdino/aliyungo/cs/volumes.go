package cs

import (
	"net/http"
)

type VolumeDriverType string

const (
	OSSFSDriver VolumeDriverType = "ossfs"
	NASDriver   VolumeDriverType = "nas"
)

type DriverOptions interface {
	driverOptions()
}

type OSSOpts struct {
	Bucket          string `json:"bucket"`
	AccessKeyId     string `json:"ak_id"`
	AccessKeySecret string `json:"ak_secret"`
	URL             string `json:"url"`
	NoStatCache     string `json:"no_stat_cache"`
	OtherOpts       string `json:"other_opts"`
}

func (*OSSOpts) driverOptions() {}

type NASOpts struct {
	DiskId string `json:"disk_id"`
	Host   string `json:"host"`
	Path   string `json:"path"`
	Mode   string `json:"mode"`
}

func (*NASOpts) driverOptions() {}

type VolumeRef struct {
	Name string `json:"Name"`
	ID   string `json:"ID"`
}

type VolumeCreationArgs struct {
	Name       string           `json:"name"`
	Driver     VolumeDriverType `json:"driver"`
	DriverOpts DriverOptions    `json:"driverOpts"`
}

type VolumeCreationResponse struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
}

type GetVolumeResponse struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	Node       string            `json:"Node"`
	Refs       []VolumeRef       `json:"Refs"`
}

type GetVolumesResponse struct {
	Volumes []GetVolumeResponse `json:"Volumes"`
}

func (client *ProjectClient) CreateVolume(args *VolumeCreationArgs) (err error) {
	err = client.Invoke(http.MethodPost, "/volumes/create", nil, args, nil)
	return
}

func (client *ProjectClient) GetVolume(name string) (volume GetVolumeResponse, err error) {
	err = client.Invoke(http.MethodGet, "/volumes/"+name, nil, nil, &volume)
	return
}

func (client *ProjectClient) GetVolumes() (volumes GetVolumesResponse, err error) {
	err = client.Invoke(http.MethodGet, "/volumes", nil, nil, &volumes)
	return
}

func (client *ProjectClient) DeleteVolume(name string) (err error) {
	err = client.Invoke(http.MethodDelete, "/volumes/"+name, nil, nil, nil)
	return
}
