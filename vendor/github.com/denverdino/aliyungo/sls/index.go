package sls

import "encoding/json"

type IndexLineConfig struct {
	TokenList     []string `json:"token,omitempty"`
	CaseSensitive bool     `json:"caseSensitive"`
	IncludeKeys   []string `json:"include_keys,omitempty"`
	Exclude_keys  []string `json:"exclude_keys,omitempty"`
}

type IndexKeyConfig struct {
	TokenList     []string `json:"token,omitempty"`
	CaseSensitive bool     `json:"caseSensitive,omitempty"`
}

type IndexConfig struct {
	TTL           int                       `json:"ttl,omitempty"`
	LineConfig    IndexLineConfig           `json:"line,omitempty"`
	KeyConfigList map[string]IndexKeyConfig `json:"keys,omitempty"`
}

func (proj *Project) CreateIndex(logstore string, indexConfig *IndexConfig) error {
	data, err := json.Marshal(indexConfig)
	if err != nil {
		return err
	}

	req := &request{
		method:      METHOD_POST,
		path:        "/logstores/" + logstore + "/index",
		payload:     data,
		contentType: "application/json",
	}
	return proj.client.requestWithClose(req)
}

func (proj *Project) DeleteIndex(logstore string) error {
	req := &request{
		method: METHOD_DELETE,
		path:   "/logstores/" + logstore + "/index",
	}

	return proj.client.requestWithClose(req)
}

func (proj *Project) GetIndex(logstore string) (*IndexConfig, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/logstores/" + logstore + "/index",
	}
	indexConfig := &IndexConfig{}
	if err := proj.client.requestWithJsonResponse(req, indexConfig); err != nil {
		return nil, err
	}
	return indexConfig, nil
}

func (proj *Project) UpdateIndex(logstore string, indexConfig *IndexConfig) error {
	data, err := json.Marshal(indexConfig)
	if err != nil {
		return err
	}
	req := &request{
		method:      METHOD_PUT,
		path:        "/logstores/" + logstore + "/index",
		payload:     data,
		contentType: "application/json",
	}

	return proj.client.requestWithClose(req)
}
