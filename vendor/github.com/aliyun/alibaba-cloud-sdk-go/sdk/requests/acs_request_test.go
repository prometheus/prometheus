package requests

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_AcsRequest(t *testing.T) {
	r := defaultBaseRequest()
	assert.NotNil(t, r)

	// query params
	query := r.GetQueryParams()
	assert.Equal(t, 0, len(query))
	r.addQueryParam("key", "value")
	assert.Equal(t, 1, len(query))
	assert.Equal(t, "value", query["key"])

	// form params
	form := r.GetFormParams()
	assert.Equal(t, 0, len(form))
	r.addFormParam("key", "value")
	assert.Equal(t, 1, len(form))
	assert.Equal(t, "value", form["key"])

	// getter/setter for stringtosign
	assert.Equal(t, "", r.GetStringToSign())
	r.SetStringToSign("s2s")
	assert.Equal(t, "s2s", r.GetStringToSign())

	// content type
	_, contains := r.GetContentType()
	assert.False(t, contains)
	r.SetContentType("application/json")
	ct, contains := r.GetContentType()
	assert.Equal(t, "application/json", ct)
	assert.True(t, contains)

	// default 3 headers & content-type
	headers := r.GetHeaders()
	assert.Equal(t, 4, len(headers))
	r.addHeaderParam("x-key", "x-key-value")
	assert.Equal(t, 5, len(headers))
	assert.Equal(t, "x-key-value", headers["x-key"])

	// Version
	r.SetVersion("2017-06-06")
	assert.Equal(t, "2017-06-06", r.GetVersion())
	// GetActionName
	assert.Equal(t, "", r.GetActionName())

	// GetMethod
	assert.Equal(t, "GET", r.GetMethod())
	r.Method = "POST"
	assert.Equal(t, "POST", r.GetMethod())

	// Domain
	assert.Equal(t, "", r.GetDomain())
	r.SetDomain("ecs.aliyuncs.com")
	assert.Equal(t, "ecs.aliyuncs.com", r.GetDomain())

	// Region
	assert.Equal(t, "", r.GetRegionId())
	r.RegionId = "cn-hangzhou"
	assert.Equal(t, "cn-hangzhou", r.GetRegionId())

	// AcceptFormat
	assert.Equal(t, "JSON", r.GetAcceptFormat())
	r.AcceptFormat = "XML"
	assert.Equal(t, "XML", r.GetAcceptFormat())

	// GetLocationServiceCode
	assert.Equal(t, "", r.GetLocationServiceCode())

	// GetLocationEndpointType
	assert.Equal(t, "", r.GetLocationEndpointType())

	// GetProduct
	assert.Equal(t, "", r.GetProduct())

	// GetScheme
	assert.Equal(t, "", r.GetScheme())
	r.SetScheme("HTTPS")
	assert.Equal(t, "HTTPS", r.GetScheme())

	// GetReadTimeout
	assert.Equal(t, 0*time.Second, r.GetReadTimeout())
	r.SetReadTimeout(5 * time.Second)
	assert.Equal(t, 5*time.Second, r.GetReadTimeout())

	// GetConnectTimeout
	assert.Equal(t, 0*time.Second, r.GetConnectTimeout())
	r.SetConnectTimeout(5 * time.Second)
	assert.Equal(t, 5*time.Second, r.GetConnectTimeout())

	// GetHTTPSInsecure
	assert.True(t, r.GetHTTPSInsecure() == nil)
	r.SetHTTPSInsecure(true)
	assert.Equal(t, true, *r.GetHTTPSInsecure())

	// GetPort
	assert.Equal(t, "", r.GetPort())

	// GetUserAgent
	r.AppendUserAgent("cli", "1.01")
	assert.Equal(t, "1.01", r.GetUserAgent()["cli"])
	// GetUserAgent
	r.AppendUserAgent("cli", "2.02")
	assert.Equal(t, "2.02", r.GetUserAgent()["cli"])
	// Content
	assert.Equal(t, []byte(nil), r.GetContent())
	r.SetContent([]byte("The Content"))
	assert.True(t, bytes.Equal([]byte("The Content"), r.GetContent()))
}

type AcsRequestTest struct {
	*baseRequest
	Ontology AcsRequest
	Query    string                 `position:"Query" name:"Query"`
	Header   string                 `position:"Header" name:"Header"`
	Path     string                 `position:"Path" name:"Path"`
	Body     string                 `position:"Body" name:"Body"`
	Target   map[string]interface{} `position:"Query" name:"Target"`
	TypeAcs  *[]string              `position:"type" name:"type" type:"Repeated"`
}

func (r AcsRequestTest) BuildQueries() string {
	return ""
}

func (r AcsRequestTest) BuildUrl() string {
	return ""
}

func (r AcsRequestTest) GetBodyReader() io.Reader {
	return nil
}

func (r AcsRequestTest) GetStyle() string {
	return ""
}

func (r AcsRequestTest) addPathParam(key, value string) {
	return
}

func Test_AcsRequest_InitParams(t *testing.T) {
	r := &AcsRequestTest{
		baseRequest: defaultBaseRequest(),
		Query:       "query value",
		Header:      "header value",
		Path:        "path value",
		Body:        "body value",
		Target: map[string]interface{}{
			"key":   "test",
			"value": 1234,
		},
	}
	tmp := []string{r.Query, r.Header}
	r.TypeAcs = &tmp
	r.addQueryParam("qkey", "qvalue")
	InitParams(r)

	queries := r.GetQueryParams()
	assert.Equal(t, "query value", queries["Query"])
	assert.Equal(t, "{\"key\":\"test\",\"value\":1234}", queries["Target"])
	headers := r.GetHeaders()
	assert.Equal(t, "header value", headers["Header"])
	// TODO: check the body & path
}

// CreateContainerGroupRequest is the request struct for api CreateContainerGroup
type CreateContainerGroupRequest struct {
	*RpcRequest
	OwnerId                 Integer                                         `position:"Query" name:"OwnerId"`
	ResourceOwnerAccount    string                                          `position:"Query" name:"ResourceOwnerAccount"`
	ResourceOwnerId         Integer                                         `position:"Query" name:"ResourceOwnerId"`
	OwnerAccount            string                                          `position:"Query" name:"OwnerAccount"`
	RegionId                string                                          `position:"Query" name:"RegionId"`
	ZoneId                  string                                          `position:"Query" name:"ZoneId"`
	SecurityGroupId         string                                          `position:"Query" name:"SecurityGroupId"`
	VSwitchId               string                                          `position:"Query" name:"VSwitchId"`
	ContainerGroupName      string                                          `position:"Query" name:"ContainerGroupName"`
	RestartPolicy           string                                          `position:"Query" name:"RestartPolicy"`
	Tag                     *[]CreateContainerGroup_Tag                     `position:"Query" name:"Tag" type:"Repeated"`
	ImageRegistryCredential *[]CreateContainerGroup_ImageRegistryCredential `position:"Query" name:"ImageRegistryCredential" type:"Repeated"`
	Container               *[]CreateContainerGroup_Container               `position:"Query" name:"Container" type:"Repeated"`
	Volume                  *[]CreateContainerGroup_Volume                  `position:"Query" name:"Volume" type:"Repeated"`
	EipInstanceId           string                                          `position:"Query" name:"EipInstanceId"`
	InitContainer           *[]CreateContainerGroup_InitContainer           `position:"Query" name:"InitContainer" type:"Repeated"`
	Cpu                     Float                                           `position:"Query" name:"Cpu"`
	Memory                  Float                                           `position:"Query" name:"Memory"`
	DnsConfig               CreateContainerGroup_DnsConfig                  `position:"Query" name:"DnsConfig" type:"Struct"`
}

type CreateContainerGroup_Tag struct {
	Key   string `name:"Key"`
	Value string `name:"Value"`
}

type CreateContainerGroup_ImageRegistryCredential struct {
	Server   string `name:"Server"`
	UserName string `name:"UserName"`
	Password string `name:"Password"`
}

type CreateContainerGroup_Container struct {
	Image           string                                 `name:"Image"`
	Name            string                                 `name:"Name"`
	Cpu             Float                                  `name:"Cpu"`
	Memory          Float                                  `name:"Memory"`
	WorkingDir      string                                 `name:"WorkingDir"`
	ImagePullPolicy string                                 `name:"ImagePullPolicy"`
	Command         []string                               `name:"Command" type:"Repeated"`
	Arg             []string                               `name:"Arg" type:"Repeated"`
	VolumeMount     *[]CreateContainerGroup_VolumeMount    `name:"VolumeMount" type:"Repeated"`
	Port            *[]CreateContainerGroup_Port           `name:"Port" type:"Repeated"`
	EnvironmentVar  *[]CreateContainerGroup_EnvironmentVar `name:"EnvironmentVar" type:"Repeated"`
	ReadinessProbe  CreateContainerGroup_ReadinessProbe    `name:"ReadinessProbe" type:"Struct"`
	LivenessProbe   CreateContainerGroup_LivenessProbe     `name:"LivenessProbe" type:"Struct"`
	SecurityContext CreateContainerGroup_SecurityContext   `name:"SecurityContext" type:"Struct"`
}

type CreateContainerGroup_Volume struct {
	Name             string                                `name:"Name"`
	Type             string                                `name:"Type"`
	NFSVolume        CreateContainerGroup_NFSVolume        `name:"NFSVolume"`
	ConfigFileVolume CreateContainerGroup_ConfigFileVolume `name:"ConfigFileVolume"`
}

type CreateContainerGroup_InitContainer struct {
	Name            string                                 `name:"Name"`
	Image           string                                 `name:"Image"`
	Cpu             Float                                  `name:"Cpu"`
	Memory          Float                                  `name:"Memory"`
	WorkingDir      string                                 `name:"WorkingDir"`
	ImagePullPolicy string                                 `name:"ImagePullPolicy"`
	Command         []string                               `name:"Command"`
	Arg             []string                               `name:"Arg"`
	VolumeMount     *[]CreateContainerGroup_VolumeMount    `name:"VolumeMount"`
	Port            *[]CreateContainerGroup_Port           `name:"Port"`
	EnvironmentVar  *[]CreateContainerGroup_EnvironmentVar `name:"EnvironmentVar"`
	SecurityContext CreateContainerGroup_SecurityContext   `name:"SecurityContext"`
}

type CreateContainerGroup_DnsConfig struct {
	NameServer []string                       `name:"NameServer"`
	Search     []string                       `name:"Search"`
	Option     *[]CreateContainerGroup_Option `name:"Option"`
}

type CreateContainerGroup_VolumeMount struct {
	MountPath string  `name:"MountPath"`
	ReadOnly  Boolean `name:"ReadOnly"`
	Name      string  `name:"Name"`
}

type CreateContainerGroup_Port struct {
	Protocol string  `name:"Protocol"`
	Port     Integer `name:"Port"`
}

type CreateContainerGroup_EnvironmentVar struct {
	Key   string `name:"Key"`
	Value string `name:"Value"`
}

type CreateContainerGroup_ReadinessProbe struct {
	InitialDelaySeconds Integer                        `name:"InitialDelaySeconds"`
	PeriodSeconds       Integer                        `name:"PeriodSeconds"`
	SuccessThreshold    Integer                        `name:"SuccessThreshold"`
	FailureThreshold    Integer                        `name:"FailureThreshold"`
	TimeoutSeconds      Integer                        `name:"TimeoutSeconds"`
	HttpGet             CreateContainerGroup_HttpGet   `name:"HttpGet"`
	Exec                CreateContainerGroup_Exec      `name:"Exec"`
	TcpSocket           CreateContainerGroup_TcpSocket `name:"TcpSocket"`
}

type CreateContainerGroup_HttpGet struct {
	Path   string  `name:"Path"`
	Port   Integer `name:"Port"`
	Scheme string  `name:"Scheme"`
}

type CreateContainerGroup_Exec struct {
	Command []string `name:"Command"`
}

type CreateContainerGroup_TcpSocket struct {
	Port Integer `name:"Port"`
}

type CreateContainerGroup_LivenessProbe struct {
	InitialDelaySeconds Integer                        `name:"InitialDelaySeconds"`
	PeriodSeconds       Integer                        `name:"PeriodSeconds"`
	SuccessThreshold    Integer                        `name:"SuccessThreshold"`
	FailureThreshold    Integer                        `name:"FailureThreshold"`
	TimeoutSeconds      Integer                        `name:"TimeoutSeconds"`
	HttpGet             CreateContainerGroup_HttpGet   `name:"HttpGet"`
	Exec                CreateContainerGroup_Exec      `name:"Exec"`
	TcpSocket           CreateContainerGroup_TcpSocket `name:"TcpSocket"`
}

type CreateContainerGroup_SecurityContext struct {
	ReadOnlyRootFilesystem Boolean                         `name:"ReadOnlyRootFilesystem"`
	RunAsUser              Integer                         `name:"RunAsUser"`
	Capability             CreateContainerGroup_Capability `name:"Capability"`
}

type CreateContainerGroup_Capability struct {
	Add []string `name:"Add"`
}

type CreateContainerGroup_NFSVolume struct {
	Server   string  `name:"Server"`
	Path     string  `name:"Path"`
	ReadOnly Boolean `name:"ReadOnly"`
}

type CreateContainerGroup_ConfigFileVolume struct {
	ConfigFileToPath *[]CreateContainerGroup_ConfigFileToPath `name:"ConfigFileToPath"`
}

type CreateContainerGroup_ConfigFileToPath struct {
	Content string `name:"Content"`
	Path    string `name:"Path"`
}

type CreateContainerGroup_Option struct {
	Name  string `name:"Name"`
	Value string `name:"Value"`
}

func GetQueryString(r *CreateContainerGroupRequest) string {
	queries := r.GetQueryParams()
	// To store the keys in slice in sorted order
	sortedKeys := make([]string, 0)
	for k := range queries {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	// To perform the opertion you want
	resultBuilder := bytes.Buffer{}
	for _, key := range sortedKeys {
		resultBuilder.WriteString(key + "=" + queries[key] + "&")
	}
	return resultBuilder.String()
}

func InitRequest() (r *CreateContainerGroupRequest) {
	r = &CreateContainerGroupRequest{
		RpcRequest: &RpcRequest{},
	}
	r.InitWithApiInfo("Eci", "2018-08-08", "CreateContainerGroup", "eci", "openAPI")
	return
}

func Test_AcsRequest_InitParams2(t *testing.T) {
	r := InitRequest()
	InitParams(r)
	assert.Equal(t, "", GetQueryString(r))
}

func Test_AcsRequest_InitParams3(t *testing.T) {
	r := InitRequest()
	r.RegionId = "regionid"
	InitParams(r)
	assert.Equal(t, "RegionId=regionid&", GetQueryString(r))
}

func Test_AcsRequest_InitParams4(t *testing.T) {
	r := InitRequest()
	r.RegionId = "regionid"
	r.DnsConfig = CreateContainerGroup_DnsConfig{
		NameServer: []string{"nameserver1", "nameserver2"},
	}
	InitParams(r)
	assert.Equal(t, "DnsConfig.NameServer.1=nameserver1&DnsConfig.NameServer.2=nameserver2&RegionId=regionid&",
		GetQueryString(r))
}

func Test_AcsRequest_InitParams5(t *testing.T) {
	r := InitRequest()
	r.Container = &[]CreateContainerGroup_Container{
		{
			Image:      "nginx",
			Name:       "nginx",
			Cpu:        "1",
			Memory:     "2",
			WorkingDir: "ddd",
		},
	}
	InitParams(r)
	assert.Equal(t, "Container.1.Cpu=1&Container.1.Image=nginx&Container.1.Memory=2&Container.1.Name=nginx&Container.1.WorkingDir=ddd&",
		GetQueryString(r))
}

type StartMPUTaskRequest struct {
	*RpcRequest
	OwnerId         Integer   `position:"Query" name:"OwnerId"`
	AppId           string    `position:"Query" name:"AppId"`
	ChannelId       string    `position:"Query" name:"ChannelId"`
	TaskId          string    `position:"Query" name:"TaskId"`
	MediaEncode     Integer   `position:"Query" name:"MediaEncode"`
	BackgroundColor Integer   `position:"Query" name:"BackgroundColor"`
	LayoutIds       []Integer `position:"Query" name:"LayoutIds" type:"Repeated"`
	StreamURL       string    `position:"Query" name:"StreamURL"`
}

func Test_RPCRequest_InitParams(t *testing.T) {
	channelID := "id"
	r := &StartMPUTaskRequest{
		RpcRequest: &RpcRequest{},
	}
	r.init()
	r.Domain = "rtc.aliyuncs.com"
	r.AppId = "app ID"
	r.ChannelId = channelID
	r.TaskId = channelID
	r.MediaEncode = NewInteger(2)
	r.BackgroundColor = NewInteger(0)
	r.StreamURL = fmt.Sprintf("rtmp://video-center.alivecdn.com/galaxy/%s_%s?vhost=fast-live.chinalivestream.top", channelID, channelID)
	var out []Integer
	out = append(out, NewInteger(2))
	r.LayoutIds = out

	InitParams(r)

	queries := r.GetQueryParams()

	assert.Equal(t, "2", queries["LayoutIds.1"])
	assert.Len(t, queries, 7)
}
