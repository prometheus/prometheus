package hypervisors

import (
	"encoding/json"
	"fmt"

	"github.com/gophercloud/gophercloud/pagination"
)

type Topology struct {
	Sockets int `json:"sockets"`
	Cores   int `json:"cores"`
	Threads int `json:"threads"`
}

type CPUInfo struct {
	Vendor   string   `json:"vendor"`
	Arch     string   `json:"arch"`
	Model    string   `json:"model"`
	Features []string `json:"features"`
	Topology Topology `json:"topology"`
}

type Service struct {
	Host           string `json:"host"`
	ID             int    `json:"id"`
	DisabledReason string `json:"disabled_reason"`
}

type Hypervisor struct {
	// A structure that contains cpu information like arch, model, vendor, features and topology
	CPUInfo CPUInfo `json:"-"`
	// The current_workload is the number of tasks the hypervisor is responsible for.
	// This will be equal or greater than the number of active VMs on the system
	// (it can be greater when VMs are being deleted and the hypervisor is still cleaning up).
	CurrentWorkload int `json:"current_workload"`
	// Status of the hypervisor, either "enabled" or "disabled"
	Status string `json:"status"`
	// State of the hypervisor, either "up" or "down"
	State string `json:"state"`
	// Actual free disk on this hypervisor in GB
	DiskAvailableLeast int `json:"disk_available_least"`
	// The hypervisor's IP address
	HostIP string `json:"host_ip"`
	// The free disk remaining on this hypervisor in GB
	FreeDiskGB int `json:"-"`
	// The free RAM in this hypervisor in MB
	FreeRamMB int `json:"free_ram_mb"`
	// The hypervisor host name
	HypervisorHostname string `json:"hypervisor_hostname"`
	// The hypervisor type
	HypervisorType string `json:"hypervisor_type"`
	// The hypervisor version
	HypervisorVersion int `json:"-"`
	// Unique ID of the hypervisor
	ID int `json:"id"`
	// The disk in this hypervisor in GB
	LocalGB int `json:"-"`
	// The disk used in this hypervisor in GB
	LocalGBUsed int `json:"local_gb_used"`
	// The memory of this hypervisor in MB
	MemoryMB int `json:"memory_mb"`
	// The memory used in this hypervisor in MB
	MemoryMBUsed int `json:"memory_mb_used"`
	// The number of running vms on this hypervisor
	RunningVMs int `json:"running_vms"`
	// The hypervisor service object
	Service Service `json:"service"`
	// The number of vcpu in this hypervisor
	VCPUs int `json:"vcpus"`
	// The number of vcpu used in this hypervisor
	VCPUsUsed int `json:"vcpus_used"`
}

func (r *Hypervisor) UnmarshalJSON(b []byte) error {

	type tmp Hypervisor
	var s struct {
		tmp
		CPUInfo           interface{} `json:"cpu_info"`
		HypervisorVersion interface{} `json:"hypervisor_version"`
		FreeDiskGB        interface{} `json:"free_disk_gb"`
		LocalGB           interface{} `json:"local_gb"`
	}

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	*r = Hypervisor(s.tmp)

	// Newer versions pass the CPU into around as the correct types, this just needs
	// converting and copying into place. Older versions pass CPU info around as a string
	// and can simply be unmarshalled by the json parser
	var tmpb []byte

	switch t := s.CPUInfo.(type) {
	case string:
		tmpb = []byte(t)
	case map[string]interface{}:
		tmpb, err = json.Marshal(t)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("CPUInfo has unexpected type: %T", t)
	}

	err = json.Unmarshal(tmpb, &r.CPUInfo)
	if err != nil {
		return err
	}

	// These fields may be passed in in scientific notation
	switch t := s.HypervisorVersion.(type) {
	case int:
		r.HypervisorVersion = t
	case float64:
		r.HypervisorVersion = int(t)
	default:
		return fmt.Errorf("Hypervisor version of unexpected type")
	}

	switch t := s.FreeDiskGB.(type) {
	case int:
		r.FreeDiskGB = t
	case float64:
		r.FreeDiskGB = int(t)
	default:
		return fmt.Errorf("Free disk GB of unexpected type")
	}

	switch t := s.LocalGB.(type) {
	case int:
		r.LocalGB = t
	case float64:
		r.LocalGB = int(t)
	default:
		return fmt.Errorf("Local GB of unexpected type")
	}

	return nil
}

type HypervisorPage struct {
	pagination.SinglePageBase
}

func (page HypervisorPage) IsEmpty() (bool, error) {
	va, err := ExtractHypervisors(page)
	return len(va) == 0, err
}

func ExtractHypervisors(p pagination.Page) ([]Hypervisor, error) {
	var h struct {
		Hypervisors []Hypervisor `json:"hypervisors"`
	}
	err := (p.(HypervisorPage)).ExtractInto(&h)
	return h.Hypervisors, err
}
