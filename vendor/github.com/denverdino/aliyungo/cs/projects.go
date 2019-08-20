package cs

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Project struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Template     string            `json:"template"`
	Version      string            `json:"version"`
	Created      string            `json:"created"`
	Updated      string            `json:"updated"`
	DesiredState string            `json:"desired_state"`
	CurrentState string            `json:"current_state"`
	Environment  map[string]string `json:"environment"`
	Services     []Service         `json:"services"`
}

type GetProjectsResponse []Project

type GetProjectResponse Project

type Port struct {
	HostIP   string `json:"host_ip"`
	HostPort string `json:"host_port"`
}

type Labels map[string]string

type Definition struct {
	Environment        []string `json:"environment"`
	Image              string   `json:"image"`
	KernelMemory       int      `json:"kernel_memory"`
	Labels             Labels   `json:"labels"`
	MemLimit           int      `json:"mem_limit"`
	MemswapLimit       int      `json:"memswap_limit"`
	MemswapReservation int      `json:"memswap_reservation"`
	OomKillDisable     bool     `json:"oom_kill_disable"`
	Restart            string   `json:"restart"`
	ShmSize            int      `json:"shm_size"`
	Volumes            []string `json:"volumes"`
}

type ProjectCreationArgs struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Template    string            `json:"template"`
	Version     string            `json:"version"`
	Environment map[string]string `json:"environment"`
	LatestImage bool              `json:"latest_image"`
}

type ProjectUpdationArgs struct {
	Name         string            `json:"-"`
	Description  string            `json:"description"`
	Template     string            `json:"template"`
	Version      string            `json:"version"`
	Environment  map[string]string `json:"environment"`
	LatestImage  bool              `json:"latest_image"`
	UpdateMethod string            `json:"update_method"`
}

func (client *ProjectClient) GetProjects(q string, services, containers bool) (projects GetProjectsResponse, err error) {
	query := make(url.Values)

	if len(q) != 0 {
		query.Add("q", q)
	}

	query.Add("services", strconv.FormatBool(services))
	query.Add("containers", strconv.FormatBool(containers))

	err = client.Invoke(http.MethodGet, "/projects/", query, nil, &projects)

	return
}

func (client *ProjectClient) GetProject(name string) (project GetProjectResponse, err error) {

	if len(name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	err = client.Invoke(http.MethodGet, "/projects/"+name, nil, nil, &project)

	return
}

func (client *ProjectClient) StartProject(name string) (err error) {

	if len(name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	err = client.Invoke(http.MethodPost, "/projects/"+name+"/start", nil, nil, nil)

	return
}

func (client *ProjectClient) StopProject(name string, timeout ...time.Duration) (err error) {

	if len(name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	query := make(url.Values)

	if len(timeout) > 0 {
		if timeout[0] > 0 {
			query.Add("t", strconv.Itoa(int(timeout[0].Seconds())))
		}
	}

	err = client.Invoke(http.MethodPost, "/projects/"+name+"/stop", query, nil, nil)

	return
}

func (client *ProjectClient) KillProject(name string, signal ...string) (err error) {

	if len(name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	query := make(url.Values)

	if len(signal) > 0 {
		if len(signal[0]) > 0 {
			query.Add("signal", signal[0])
		}
	}

	err = client.Invoke(http.MethodPost, "/projects/"+name+"/kill", query, nil, nil)

	return
}

func (client *ProjectClient) CreateProject(args *ProjectCreationArgs) (err error) {

	args.Template = strings.TrimSpace(args.Template)

	err = client.Invoke(http.MethodPost, "/projects/", nil, args, nil)

	return
}

func (client *ProjectClient) UpdateProject(args *ProjectUpdationArgs) (err error) {

	if len(args.Name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	args.Template = strings.TrimSpace(args.Template)

	err = client.Invoke(http.MethodPost, "/projects/"+args.Name+"/update", nil, args, nil)

	return
}

func (client *ProjectClient) DeleteProject(name string, forceDelete, deleteVolume bool) (err error) {

	if len(name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	query := make(url.Values)

	query.Add("name", name)
	query.Add("force", strconv.FormatBool(forceDelete))
	query.Add("volume", strconv.FormatBool(deleteVolume))

	err = client.Invoke(http.MethodDelete, "/projects/"+name, query, nil, nil)

	return
}

func (client *ProjectClient) ConfirmBlueGreenProject(name string, force bool) (err error) {

	if len(name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	err = client.Invoke(http.MethodPost, "/projects/"+name+"/confirm-update?force="+strconv.FormatBool(force), nil, nil, nil)

	return
}

func (client *ProjectClient) RollBackBlueGreenProject(name string, force bool) (err error) {

	if len(name) == 0 {
		err = errors.New("project name is empty")
		return
	}

	err = client.Invoke(http.MethodPost, "/projects/"+name+"/rollback-update?force="+strconv.FormatBool(force), nil, nil, nil)

	return
}

type SwarmNodeType struct {
	IP                string
	Healthy           bool
	InstanceId        string
	ZoneId            string
	RegionId          string
	Status            string
	CreationTime      string `json:"created"`
	UpdatedTime       string `json:"updated"`
	Containers        int
	ContainersRunning int
	ContainersPaused  int
	ContainersStopped int
	AgentVersion      string
	Name              string
}
type GetSwarmClusterNodesResponse []SwarmNodeType

func (client *ProjectClient) GetSwarmClusterNodes() (project GetSwarmClusterNodesResponse, err error) {
	err = client.Invoke(http.MethodGet, "/hosts/", nil, nil, &project)
	return
}
