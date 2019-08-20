package sls

import "encoding/json"

type Logstore struct {
	TTL   int    `json:"ttl,omitempty"`
	Shard int    `json:"shardCount,omitempty"`
	Name  string `json:"logstoreName,omitempty"`
}

func (proj *Project) CreateLogstore(logstore *Logstore) error {

	data, err := json.Marshal(logstore)
	if err != nil {
		return err
	}

	req := &request{
		method:      METHOD_POST,
		path:        "/logstores",
		contentType: "application/json",
		payload:     data,
	}

	return proj.client.requestWithClose(req)
}

func (proj *Project) GetLogstore(name string) (*Logstore, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/logstores/" + name,
	}

	ls := &Logstore{}
	if err := proj.client.requestWithJsonResponse(req, ls); err != nil {
		return nil, err
	}
	return ls, nil
}

func (proj *Project) DeleteLogstore(name string) error {
	req := &request{
		method: METHOD_DELETE,
		path:   "/logstores/" + name,
	}
	return proj.client.requestWithClose(req)
}

func (proj *Project) UpdateLogstore(logstore *Logstore) error {
	data, err := json.Marshal(logstore)
	if err != nil {
		return err
	}
	req := &request{
		method:      METHOD_PUT,
		path:        "/logstores/" + logstore.Name,
		contentType: "application/json",
		payload:     data,
	}

	return proj.client.requestWithClose(req)
}

type LogstoreList struct {
	Count     int      `json:"count,omitempty"`
	Total     int      `json:"total,omitempty"`
	Logstores []string `json:"logstores,omitempty"`
}

func (proj *Project) ListLogstore() (*LogstoreList, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/logstores",
	}
	list := &LogstoreList{}
	if err := proj.client.requestWithJsonResponse(req, list); err != nil {
		return nil, err
	}
	return list, nil
}

type shard struct {
	Id int `json:"shardID,omitempty"`
}

func (proj *Project) ListShards(logstoreName string) ([]int, error) {
	req := &request{
		method: METHOD_GET,
		path:   "/logstores/" + logstoreName + "/shards",
	}

	var resp []*shard
	if err := proj.client.requestWithJsonResponse(req, &resp); err != nil {
		return nil, err
	}

	var shards []int
	for _, shard := range resp {
		shards = append(shards, shard.Id)
	}
	return shards, nil
}
