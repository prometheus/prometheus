package sls

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type GroupAttribute struct {
	Topic        string `json:"groupTopic,omitempty"`
	ExternalName string `json:"externalName,omitempty"`
}

type MachineGroup struct {
	Name                string          `json:"groupName,omitempty"`
	Type                string          `json:"groupType,omitempty"`
	MachineIdentifyType string          `json:"machineIdentifyType,omitempty"`
	Attribute           *GroupAttribute `json:"attribute,omitempty"`
	MachineList         []string        `json:"machineList,omitempty"`
	CreateTime          uint32          `json:"createTime,omitempty"`
	LastModifyTime      uint32          `json:"lastModifyTime,omitempty"`
}

func (proj *Project) CreateMachineGroup(machineGroup *MachineGroup) error {

	data, err := json.Marshal(machineGroup)
	if err != nil {
		return err
	}
	req := &request{
		method:      METHOD_POST,
		path:        "/machinegroups",
		payload:     data,
		contentType: "application/json",
	}

	return proj.client.requestWithClose(req)
}

type MachineGroupList struct {
	Groups []string `json:"machinegroups"`
	Count  int      `json:"count"`
	Total  int      `json:"total"`
}

func (proj *Project) ListMachineGroup(offset, size int) (*MachineGroupList, error) {
	req := &request{
		path:   "/machinegroups",
		method: METHOD_GET,
		params: map[string]string{
			"size":   strconv.Itoa(size),
			"offset": strconv.Itoa(offset),
		},
	}

	groups := &MachineGroupList{}

	if err := proj.client.requestWithJsonResponse(req, groups); err != nil {
		return nil, err
	}

	return groups, nil
}

func (proj *Project) MachineGroup(name string) (*MachineGroup, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/machinegroups/" + name,
	}

	group := &MachineGroup{}

	if err := proj.client.requestWithJsonResponse(req, group); err != nil {
		return nil, err
	}

	return group, nil
}

func (proj *Project) DeleteMachineGroup(name string) error {
	req := &request{
		method: METHOD_DELETE,
		path:   "/machinegroups/" + name,
	}

	return proj.client.requestWithClose(req)
}

func (proj *Project) UpdateMachineGroup(machineGroup *MachineGroup) error {
	data, err := json.Marshal(machineGroup)
	if err != nil {
		return err
	}
	req := &request{
		method:      METHOD_PUT,
		path:        "/machinegroups/" + machineGroup.Name,
		payload:     data,
		contentType: "application/json",
	}

	return proj.client.requestWithClose(req)
}

func (proj *Project) ApplyConfigToMachineGroup(machineGroup string, config string) error {
	req := &request{
		method: METHOD_PUT,
		path:   "/machinegroups/" + machineGroup + "/configs/" + config,
	}
	return proj.client.requestWithClose(req)
}

type Machine struct {
	Ip            string `json:"ip,omitempty"`
	Uniqueid      string `json:"machine-uniqueid,omitempty"`
	UserdefinedId string `json:"userdefined-id,omitempty"`
}

type MachineList struct {
	Count    int       `json:"count,omitempty"`
	Total    int       `json:"total,omitempty"`
	Machines []Machine `json:"machines,omitempty"`
}

func (proj *Project) ListMachines(machineGroup string, offset, size int) (*MachineList, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/machinegroups/" + machineGroup + "/machines",
		params: map[string]string{
			"size":   strconv.Itoa(size),
			"offset": strconv.Itoa(offset),
		},
	}
	//list := &MachineList{ machines:[]Machine{} }
	list := &MachineList{}
	if err := proj.client.requestWithJsonResponse(req, list); err != nil {
		return nil, err
	}
	return list, nil
}

func (proj *Project) GetAppliedConfigs(machineGroup string) ([]string, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/machinegroups/" + machineGroup + "/configs",
	}

	configs := make(map[string]interface{})
	if err := proj.client.requestWithJsonResponse(req, configs); err != nil {
		return nil, err
	}

	if v, ok := configs["config"].([]string); ok {
		return v, nil
	}

	return nil, errors.New(fmt.Sprintf("%v is not a string array", configs["config"]))
}
