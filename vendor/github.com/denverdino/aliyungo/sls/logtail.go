package sls

import (
	"encoding/json"
	"strconv"
)

type LogtailInput struct {
	LogType       string   `json:"logType,omitempty"`
	LogPath       string   `json:"logPath,omitempty"`
	FilePattern   string   `json:"filePattern,omitempty"`
	LocalStorage  bool     `json:"localStorage"`
	TimeFormat    string   `json:"timeFormat"`
	EnableTag     bool     `json:"enable_tag,omitempty"`
	TimeKey       string   `json:"timeKey"`
	LogBeginRegex string   `json:"logBeginRegex,omitempty"`
	Regex         string   `json:"regex,omitempty"`
	Key           []string `json:"key,omitempty"`
	FilterKey     []string `json:"filterKey,omitempty"`
	FilterRegex   []string `json:"filterRegex,omitempty"`
	TopicFormat   string   `json:"topicFormat,omitempty"`
	Separator     string   `json:"separator,omitempty"`
	Quote         string   `json:"quote,omitempty"`
	AutoExtend    bool     `json:"autoExtend,omitempty"`
}

type LogtailOutput struct {
	LogstoreName string `json:"logstoreName,omitempty"`
}

type LogtailConfig struct {
	Name         string        `json:"configName,omitempty"`
	InputType    string        `json:"inputType,omitempty"`
	InputDetail  LogtailInput  `json:"inputDetail,omitempty"`
	OutputType   string        `json:"outputType,omitempty"`
	Sample       string        `json:"logSample,omitempty"`
	OutputDetail LogtailOutput `json:"outputDetail,omitempty"`
}

func (proj *Project) CreateConfig(config *LogtailConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	req := &request{
		method:      METHOD_POST,
		path:        "/configs",
		payload:     data,
		contentType: "application/json",
	}

	return proj.client.requestWithClose(req)
}

type LogtailConfigList struct {
	Count   int      `json:"count,omitempty"`
	Total   int      `json:"total,omitempty"`
	Configs []string `json:"configs,omitempty"`
}

func (proj *Project) ListConfig(offset, size int) (*LogtailConfigList, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/configs",
		params: map[string]string{
			"size":   strconv.Itoa(size),
			"offset": strconv.Itoa(offset),
		},
	}

	list := &LogtailConfigList{}
	if err := proj.client.requestWithJsonResponse(req, list); err != nil {
		return nil, err
	}
	return list, nil
}

func (proj *Project) GetConfig(name string) (*LogtailConfig, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/configs/" + name,
	}

	config := &LogtailConfig{}
	if err := proj.client.requestWithJsonResponse(req, config); err != nil {
		return nil, err
	}

	return config, nil
}

func (proj *Project) GetAppliedMachineGroups(configName string) ([]string, error) {
	type appliedMachineGroups struct {
		Machinegroups []string `json:"machinegroups,omitempty"`
	}

	req := &request{
		method: METHOD_GET,
		path:   "/configs/" + configName + "/machinegroups",
	}

	group := &appliedMachineGroups{}

	if err := proj.client.requestWithJsonResponse(req, group); err != nil {
		return nil, err
	}

	return group.Machinegroups, nil
}

func (proj *Project) DeleteConfig(configName string) error {
	req := &request{
		method: METHOD_DELETE,
		path:   "/configs/" + configName,
	}

	return proj.client.requestWithClose(req)
}

func (proj *Project) UpdateConfig(config *LogtailConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	req := &request{
		method:      METHOD_PUT,
		path:        "/configs/" + config.Name,
		payload:     data,
		contentType: "application/json",
	}

	return proj.client.requestWithClose(req)
}
