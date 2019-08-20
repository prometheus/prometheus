package sls

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/denverdino/aliyungo/common"
	"github.com/golang/protobuf/proto"
	//"time"
	"os"
	"strconv"
)

type Client struct {
	accessKeyId     string //Access Key Id
	accessKeySecret string //Access Key Secret
	securityToken   string //sts token
	debug           bool
	httpClient      *http.Client
	version         string
	internal        bool
	region          common.Region
	endpoint        string
}

func (client *Client) SetDebug(debug bool) {
	client.debug = debug
}

type Project struct {
	client      *Client
	Name        string `json:"projectName,omitempty"`
	Description string `json:"description,omitempty"`
}

//type LogContent struct {
//	Key   string
//	Value string
//}
//
//type LogItem struct {
//	Time     time.Time
//	Contents []*LogContent
//}
//
//type LogGroupItem struct {
//	Logs   []*LogItem
//	Topic  string
//	Source string
//}

type PutLogsRequest struct {
	Project  string
	LogStore string
	LogItems LogGroup
	HashKey  string
}

const (
	SLSDefaultEndpoint = "log.aliyuncs.com"
	SLSAPIVersion      = "0.6.0"
	METHOD_GET         = "GET"
	METHOD_POST        = "POST"
	METHOD_PUT         = "PUT"
	METHOD_DELETE      = "DELETE"
)

// NewClient creates a new instance of ECS client
func NewClient(region common.Region, internal bool, accessKeyId, accessKeySecret string) *Client {
	endpoint := os.Getenv("SLS_ENDPOINT")
	if endpoint == "" {
		endpoint = SLSDefaultEndpoint
	}
	return NewClientWithEndpoint(endpoint, region, internal, accessKeyId, accessKeySecret)
}

func NewClientForAssumeRole(region common.Region, internal bool, accessKeyId, accessKeySecret, securityToken string) *Client {
	endpoint := os.Getenv("SLS_ENDPOINT")
	if endpoint == "" {
		endpoint = SLSDefaultEndpoint
	}

	return &Client{
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
		securityToken:   securityToken,
		internal:        internal,
		region:          region,
		version:         SLSAPIVersion,
		endpoint:        endpoint,
		httpClient:      &http.Client{},
	}
}

func NewClientWithEndpoint(endpoint string, region common.Region, internal bool, accessKeyId, accessKeySecret string) *Client {
	return &Client{
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
		internal:        internal,
		region:          region,
		version:         SLSAPIVersion,
		endpoint:        endpoint,
		httpClient:      &http.Client{},
	}
}

func (client *Client) Project(name string) (*Project, error) {

	//	newClient := client.forProject(name)
	//
	//	req := &request{
	//		method: METHOD_GET,
	//		path: "/",
	//	}
	//
	//	project := &Project{}
	//
	//	if err := newClient.requestWithJsonResponse(req, project); err != nil {
	//		return nil, err
	//	}
	//	project.client = newClient
	//	return project, nil
	return &Project{
		Name:   name,
		client: client.forProject(name),
	}, nil
}

func (client *Client) forProject(name string) *Client {
	newclient := *client

	region := string(client.region)
	if client.internal {
		region = fmt.Sprintf("%s-intranet", region)
	}
	newclient.endpoint = fmt.Sprintf("%s.%s.%s", name, region, client.endpoint)
	return &newclient
}

func (client *Client) DeleteProject(name string) error {
	req := &request{
		method: METHOD_DELETE,
		path:   "/",
	}

	newClient := client.forProject(name)
	return newClient.requestWithClose(req)
}

func (client *Client) CreateProject(name string, description string) error {
	project := &Project{
		Name:        name,
		Description: description,
	}
	data, err := json.Marshal(project)
	if err != nil {
		return err
	}

	req := &request{
		method:      METHOD_POST,
		path:        "/",
		payload:     data,
		contentType: "application/json",
	}

	newClient := client.forProject(name)
	return newClient.requestWithClose(req)
}

//
//func marshal() ([]byte, error) {
//
//	logGroups := []*LogGroup{}
//	tmp := []*LogGroupItem
//	for _, logGroupItem := range tmp {
//
//		logs := []*Log{}
//		for _, logItem := range logGroupItem.Logs {
//			contents := []*Log_Content{}
//			for key, value := range logItem.Content {
//				contents = append(contents, &Log_Content{
//					Key: proto.String(key),
//					Value: proto.String(value),
//				})
//			}
//
//			logs = append(logs, &Log{
//				Time: proto.Uint32(uint32(LogItem.Time.Unix())),
//				Contents: contents,
//			})
//		}
//
//		logGroup := &LogGroup{
//			Topic: proto.String(LogGroupItem.Topic),
//			Source: proto.String(LogGroupItem.Source),
//			Logs: logs,
//		}
//		logGroups = append(logGroups, logGroup)
//	}
//
//	return proto.Marshal(&LogGroupList{
//		LogGroupList: logGroups,
//	})
//}

func (client *Client) PutLogs(putLogRequest *PutLogsRequest) error {
	if putLogRequest == nil {
		return nil
	}

	data, err := proto.Marshal(&putLogRequest.LogItems)
	if err != nil {
		return err
	}

	req := &request{
		method:      METHOD_POST,
		path:        "/logstores/" + putLogRequest.LogStore + "/shards/lb",
		payload:     data,
		contentType: "application/x-protobuf",
		headers:     map[string]string{"x-log-bodyrawsize": strconv.Itoa(len(data))},
	}

	newClient := client.forProject(putLogRequest.Project)
	return newClient.requestWithClose(req)

}
