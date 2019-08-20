package cs

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Container struct {
	Name      string            `json:"name"`
	Node      string            `json:"node"`
	VMID      string            `json:"vm_id"`
	IP        string            `json:"ip"`
	Running   bool              `json:"running"`
	Status    string            `json:"status"`
	Health    string            `json:"health"`
	StatusExt string            `json:"status_ext"`
	FailCount int               `json:"fail_count"`
	Ports     map[string][]Port `json:"ports"`
}

type Service struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Project        string                 `json:"project"`
	ComposeVersion string                 `json:"compose_version"`
	Containers     map[string]Container   `json:"containers"`
	Created        time.Time              `json:"created"`
	CurrentState   string                 `json:"current_state"`
	Definition     Definition             `json:"definition"`
	DesiredState   string                 `json:"desired_state"`
	Extensions     map[string]interface{} `json:"extensions"`
	Hash           string                 `json:"hash"`
	Updated        time.Time              `json:"updated"`
	Version        string                 `json:"version"`
}

type ScaleType string

const (
	ScaleTo ScaleType = "scale_to"
)

type ScaleServiceArgs struct {
	ServiceId string    `json:"-"`
	Type      ScaleType `json:"type"`
	Value     int       `json:"value"`
}

type GetServiceResponse Service
type GetServicesResponse []Service

func (client *ProjectClient) GetServices(q string, containers bool) (services GetServicesResponse, err error) {
	query := make(url.Values)

	if len(q) != 0 {
		query.Add("q", q)
	}

	query.Add("containers", strconv.FormatBool(containers))

	err = client.Invoke(http.MethodGet, "/services/", query, nil, &services)

	return
}

func (client *ProjectClient) GetService(serviceId string) (service GetServiceResponse, err error) {

	if len(serviceId) == 0 {
		err = errors.New("service id is empty")
		return
	}

	err = client.Invoke(http.MethodGet, "/services/"+serviceId, nil, nil, &service)

	return
}

func (client *ProjectClient) StartService(serviceId string) (err error) {

	if len(serviceId) == 0 {
		err = errors.New("service id is empty")
		return
	}

	err = client.Invoke(http.MethodPost, "/services/"+serviceId+"/start", nil, nil, nil)

	return
}

func (client *ProjectClient) StopService(serviceId string, timeout ...time.Duration) (err error) {

	if len(serviceId) == 0 {
		err = errors.New("service id is empty")
		return
	}

	query := make(url.Values)

	if len(timeout) > 0 {
		if timeout[0] > 0 {
			query.Add("t", strconv.Itoa(int(timeout[0].Seconds())))
		}
	}

	err = client.Invoke(http.MethodPost, "/services/"+serviceId+"/stop", query, nil, nil)

	return
}

func (client *ProjectClient) KillService(serviceId string, signal ...string) (err error) {

	if len(serviceId) == 0 {
		err = errors.New("service id is empty")
		return
	}

	query := make(url.Values)

	if len(signal) > 0 {
		if len(signal[0]) > 0 {
			query.Add("signal", signal[0])
		}
	}

	err = client.Invoke(http.MethodPost, "/services/"+serviceId+"/kill", query, nil, nil)

	return
}

func (client *ProjectClient) ScaleService(args *ScaleServiceArgs) (err error) {

	if len(args.ServiceId) == 0 {
		err = errors.New("service id is empty")
		return
	}

	err = client.Invoke(http.MethodPost, "/services/"+args.ServiceId+"/scale", nil, args, nil)

	return
}
