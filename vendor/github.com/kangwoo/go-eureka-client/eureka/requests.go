package eureka

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
	"strconv"
)

// Errors introduced by handling requests
var (
	ErrRequestCancelled = errors.New("sending request is cancelled")
)

type RawRequest struct {
	method       string
	relativePath string
	body         []byte
	cancel       <-chan bool
}
type Applications struct {
	VersionsDelta int           `xml:"versions__delta"`
	AppsHashcode  string        `xml:"apps__hashcode"`
	Applications  []Application `xml:"application,omitempty"`
}
type Application struct {
	Name      string         `xml:"name"`
	Instances []InstanceInfo `xml:"instance"`
}
type Instance struct {
	Instance *InstanceInfo `xml:"instance" json:"instance"`
}
type Port struct {
	Port    int  `xml:",chardata" json:"$"`
	Enabled bool `xml:"enabled,attr" json:"@enabled"`
}
type InstanceInfo struct {
	InstanceId                    string            `xml:"instanceId" json:"instanceId"`
	HostName                      string            `xml:"hostName" json:"hostName"`
	HomePageUrl                   string            `xml:"homePageUrl,omitempty" json:"homePageUrl,omitempty"`
	StatusPageUrl                 string            `xml:"statusPageUrl" json:"statusPageUrl"`
	HealthCheckUrl                string            `xml:"healthCheckUrl,omitempty" json:"healthCheckUrl,omitempty"`
	App                           string            `xml:"app" json:"app"`
	IpAddr                        string            `xml:"ipAddr" json:"ipAddr"`
	VipAddress                    string            `xml:"vipAddress" json:"vipAddress"`
	secureVipAddress              string            `xml:"secureVipAddress,omitempty" json:"secureVipAddress,omitempty"`
	Status                        string            `xml:"status" json:"status"`
	Port                          *Port             `xml:"port,omitempty" json:"port,omitempty"`
	SecurePort                    *Port             `xml:"securePort,omitempty" json:"securePort,omitempty"`
	DataCenterInfo                *DataCenterInfo   `xml:"dataCenterInfo" json:"dataCenterInfo"`
	LeaseInfo                     *LeaseInfo        `xml:"leaseInfo,omitempty" json:"leaseInfo,omitempty"`
	Metadata                      *MetaData         `xml:"metadata,omitempty" json:"metadata,omitempty"`
	IsCoordinatingDiscoveryServer bool              `xml:"isCoordinatingDiscoveryServer,omitempty" json:"isCoordinatingDiscoveryServer,omitempty"`
	LastUpdatedTimestamp          int               `xml:"lastUpdatedTimestamp,omitempty" json:"lastUpdatedTimestamp,omitempty"`
	LastDirtyTimestamp            int               `xml:"lastDirtyTimestamp,omitempty" json:"lastDirtyTimestamp,omitempty"`
	ActionType                    string            `xml:"actionType,omitempty" json:"actionType,omitempty"`
	Overriddenstatus              string            `xml:"overriddenstatus,omitempty" json:"overriddenstatus,omitempty"`
	CountryId                     int               `xml:"countryId,omitempty" json:"countryId,omitempty"`

}
type DataCenterInfo struct {
	Name     string             `xml:"name" json:"name"`
	Class    string             `xml:"class,attr" json:"@class"`
	Metadata *DataCenterMetadata `xml:"metadata,omitempty" json:"metadata,omitempty"`
}

type DataCenterMetadata struct {
	AmiLaunchIndex   string `xml:"ami-launch-index,omitempty" json:"ami-launch-index,omitempty"`
	LocalHostname    string `xml:"local-hostname,omitempty" json:"local-hostname,omitempty"`
	AvailabilityZone string `xml:"availability-zone,omitempty" json:"availability-zone,omitempty"`
	InstanceId       string `xml:"instance-id,omitempty" json:"instance-id,omitempty"`
	PublicIpv4       string `xml:"public-ipv4,omitempty" json:"public-ipv4,omitempty"`
	PublicHostname   string `xml:"public-hostname,omitempty" json:"public-hostname,omitempty"`
	AmiManifestPath  string `xml:"ami-manifest-path,omitempty" json:"ami-manifest-path,omitempty"`
	LocalIpv4        string `xml:"local-ipv4,omitempty" json:"local-ipv4,omitempty"`
	Hostname         string `xml:"hostname,omitempty" json:"hostname,omitempty"`
	AmiId            string `xml:"ami-id,omitempty" json:"ami-id,omitempty"`
	InstanceType     string `xml:"instance-type,omitempty" json:"instance-type,omitempty"`
}

type LeaseInfo struct {
	EvictionDurationInSecs uint `xml:"evictionDurationInSecs,omitempty" json:"evictionDurationInSecs,omitempty"`
	RenewalIntervalInSecs  int `xml:"renewalIntervalInSecs,omitempty" json:"renewalIntervalInSecs,omitempty"`
	DurationInSecs         int `xml:"durationInSecs,omitempty" json:"durationInSecs,omitempty"`
	RegistrationTimestamp  int `xml:"registrationTimestamp,omitempty" json:"registrationTimestamp,omitempty"`
	LastRenewalTimestamp   int `xml:"lastRenewalTimestamp,omitempty" json:"lastRenewalTimestamp,omitempty"`
	EvictionTimestamp      int `xml:"evictionTimestamp,omitempty" json:"evictionTimestamp,omitempty"`
	ServiceUpTimestamp     int `xml:"serviceUpTimestamp,omitempty" json:"serviceUpTimestamp,omitempty"`
}

func NewRawRequest(method, relativePath string, body []byte, cancel <-chan bool) *RawRequest {
	return &RawRequest{
		method:       method,
		relativePath: relativePath,
		body:         body,
		cancel:       cancel,
	}
}

func NewInstanceInfo(hostName, app, ip string, port int, ttl uint, isSsl bool) *InstanceInfo {
	dataCenterInfo := &DataCenterInfo{
		Name: "MyOwn",
		Class:    "com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo",
		Metadata: nil,
	}
	leaseInfo := &LeaseInfo{
		EvictionDurationInSecs: ttl,
	}
	instanceInfo := &InstanceInfo{
		HostName:       hostName,
		App:            app,
		IpAddr:         ip,
		Status:         UP,
		DataCenterInfo: dataCenterInfo,
		LeaseInfo:      leaseInfo,
		Metadata:       nil,
	}
	stringPort := ""
	if (port != 80 && port != 443) {
		stringPort = ":" + strconv.Itoa(port)
	}
	var protocol string = "http"
	if (isSsl) {
		protocol = "https"
		instanceInfo.secureVipAddress = protocol + "://" + hostName + stringPort
		instanceInfo.SecurePort = &Port{
			Port: port,
			Enabled: true,
		}
	}else {
		instanceInfo.VipAddress = protocol + "://" + hostName + stringPort
		instanceInfo.Port = &Port{
			Port: port,
			Enabled: true,
		}
	}
	instanceInfo.StatusPageUrl = protocol + "://" + hostName + stringPort + "/info"
	return instanceInfo
}

// getCancelable issues a cancelable GET request
func (c *Client) getCancelable(endpoint string,
cancel <-chan bool) (*RawResponse, error) {
	logger.Debug("get %s [%s]", endpoint, c.Cluster.Leader)
	p := endpoint

	req := NewRawRequest("GET", p, nil, cancel)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// get issues a GET request
func (c *Client) Get(endpoint string) (*RawResponse, error) {
	return c.getCancelable(endpoint, nil)
}

// put issues a PUT request
func (c *Client) Put(endpoint string, body []byte) (*RawResponse, error) {

	logger.Debug("put %s, %s, [%s]", endpoint, body, c.Cluster.Leader)
	p := endpoint

	req := NewRawRequest("PUT", p, body, nil)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// post issues a POST request
func (c *Client) Post(endpoint string, body []byte) (*RawResponse, error) {
	logger.Debug("post %s, %s, [%s]", endpoint, body, c.Cluster.Leader)
	p := endpoint

	req := NewRawRequest("POST", p, body, nil)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// delete issues a DELETE request
func (c *Client) Delete(endpoint string) (*RawResponse, error) {
	logger.Debug("delete %s [%s]", endpoint, c.Cluster.Leader)
	p := endpoint

	req := NewRawRequest("DELETE", p, nil, nil)
	resp, err := c.SendRequest(req)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) SendRequest(rr *RawRequest) (*RawResponse, error) {

	var req *http.Request
	var resp *http.Response
	var httpPath string
	var err error
	var respBody []byte

	var numReqs = 1

	checkRetry := c.CheckRetry
	if checkRetry == nil {
		checkRetry = DefaultCheckRetry
	}

	cancelled := make(chan bool, 1)
	reqLock := new(sync.Mutex)

	if rr.cancel != nil {
		cancelRoutine := make(chan bool)
		defer close(cancelRoutine)

		go func() {
			select {
			case <-rr.cancel:
				cancelled <- true
				logger.Debug("send.request is cancelled")
			case <-cancelRoutine:
				return
			}

			// Repeat canceling request until this thread is stopped
			// because we have no idea about whether it succeeds.
			for {
				reqLock.Lock()
				c.httpClient.Transport.(*http.Transport).CancelRequest(req)
				reqLock.Unlock()

				select {
				case <-time.After(100 * time.Millisecond):
				case <-cancelRoutine:
					return
				}
			}
		}()
	}

	// If we connect to a follower and consistency is required, retry until
	// we connect to a leader
	sleep := 25 * time.Millisecond
	maxSleep := time.Second

	for attempt := 0;; attempt++ {
		if attempt > 0 {
			select {
			case <-cancelled:
				return nil, ErrRequestCancelled
			case <-time.After(sleep):
				sleep = sleep * 2
				if sleep > maxSleep {
					sleep = maxSleep
				}
			}
		}

		logger.Debug("Connecting to eureka: attempt %d for %s", attempt + 1, rr.relativePath)

		httpPath = c.getHttpPath(false, rr.relativePath)

		logger.Debug("send.request.to %s | method %s", httpPath, rr.method)

		req, err := func() (*http.Request, error) {
			reqLock.Lock()
			defer reqLock.Unlock()

			if req, err = http.NewRequest(rr.method, httpPath, bytes.NewReader(rr.body)); err != nil {
				return nil, err
			}

			req.Header.Set("Content-Type",
				"application/json")
			return req, nil
		}()

		if err != nil {
			return nil, err
		}

		resp, err = c.httpClient.Do(req)
		defer func() {
			if resp != nil {
				resp.Body.Close()
			}
		}()

		// If the request was cancelled, return ErrRequestCancelled directly
		select {
		case <-cancelled:
			return nil, ErrRequestCancelled
		default:
		}

		numReqs++

		// network error, change a machine!
		if err != nil {
			logger.Error("network error: %v", err.Error())
			lastResp := http.Response{}
			if checkErr := checkRetry(c.Cluster, numReqs, lastResp, err); checkErr != nil {
				return nil, checkErr
			}

			c.Cluster.switchLeader(attempt % len(c.Cluster.Machines))
			continue
		}

		// if there is no error, it should receive response
		logger.Debug("recv.response.from "+httpPath)

		if validHttpStatusCode[resp.StatusCode] {
			// try to read byte code and break the loop
			respBody, err = ioutil.ReadAll(resp.Body)
			if err == nil {
				logger.Debug("recv.success "+ httpPath)
				break
			}
			// ReadAll error may be caused due to cancel request
			select {
			case <-cancelled:
				return nil, ErrRequestCancelled
			default:
			}

			if err == io.ErrUnexpectedEOF {
				// underlying connection was closed prematurely, probably by timeout
				// TODO: empty body or unexpectedEOF can cause http.Transport to get hosed;
				// this allows the client to detect that and take evasive action. Need
				// to revisit once code.google.com/p/go/issues/detail?id=8648 gets fixed.
				respBody = []byte{}
				break
			}
		}

		// if resp is TemporaryRedirect, set the new leader and retry
		if resp.StatusCode == http.StatusTemporaryRedirect {
			u, err := resp.Location()

			if err != nil {
				logger.Warning("%v", err)
			} else {
				// Update cluster leader based on redirect location
				// because it should point to the leader address
				c.Cluster.updateLeaderFromURL(u)
				logger.Debug("recv.response.relocate "+ u.String())
			}
			resp.Body.Close()
			continue
		}

		if checkErr := checkRetry(c.Cluster, numReqs, *resp,
			errors.New("Unexpected HTTP status code")); checkErr != nil {
			return nil, checkErr
		}
		resp.Body.Close()
	}

	r := &RawResponse{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Header:     resp.Header,
	}

	return r, nil
}

// DefaultCheckRetry defines the retrying behaviour for bad HTTP requests
// If we have retried 2 * machine number, stop retrying.
// If status code is InternalServerError, sleep for 200ms.
func DefaultCheckRetry(cluster *Cluster, numReqs int, lastResp http.Response,
err error) error {

	if numReqs >= 2 * len(cluster.Machines) {
		return newError(ErrCodeEurekaNotReachable,
			"Tried to connect to each peer twice and failed", 0)
	}

	code := lastResp.StatusCode
	if code == http.StatusInternalServerError {
		time.Sleep(time.Millisecond * 200)

	}

	logger.Warning("bad response status code %d", code)
	return nil
}

func (c *Client) getHttpPath(random bool, s ...string) string {
	var machine string
	if random {
		machine = c.Cluster.Machines[rand.Intn(len(c.Cluster.Machines))]
	} else {
		machine = c.Cluster.Leader
	}

	fullPath := machine
	for _, seg := range s {
		fullPath += "/" + seg
	}

	return fullPath
}

// buildValues builds a url.Values map according to the given value and ttl
func buildValues(value string, ttl uint64) url.Values {
	v := url.Values{}

	if value != "" {
		v.Set("value", value)
	}

	if ttl > 0 {
		v.Set("ttl", fmt.Sprintf("%v", ttl))
	}

	return v
}
