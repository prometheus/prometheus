package eureka

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"
	"strings"
)

const (
	defaultBufferSize = 10
	UP = "UP"
	DOWN = "DOWN"
	STARTING = "STARTING"
)

type Config struct {
	CertFile    string        `json:"certFile"`
	KeyFile     string        `json:"keyFile"`
	CaCertFile  []string      `json:"caCertFiles"`
	DialTimeout time.Duration `json:"timeout"`
	Consistency string        `json:"consistency"`
}

type Client struct {
	Config      Config   `json:"config"`
	Cluster     *Cluster `json:"cluster"`
	httpClient  *http.Client
	persistence io.Writer
	cURLch      chan string
	// CheckRetry can be used to control the policy for failed requests
	// and modify the cluster if needed.
	// The client calls it before sending requests again, and
	// stops retrying if CheckRetry returns some error. The cases that
	// this function needs to handle include no response and unexpected
	// http status code of response.
	// If CheckRetry is nil, client will call the default one
	// `DefaultCheckRetry`.
	// Argument cluster is the eureka.Cluster object that these requests have been made on.
	// Argument numReqs is the number of http.Requests that have been made so far.
	// Argument lastResp is the http.Responses from the last request.
	// Argument err is the reason of the failure.
	CheckRetry  func(cluster *Cluster, numReqs int,
	lastResp http.Response, err error) error
}

// NewClient create a basic client that is configured to be used
// with the given machine list.
func NewClient(machines []string) *Client {
	config := Config{
		// default timeout is one second
		DialTimeout: time.Second,
	}

	client := &Client{
		Cluster: NewCluster(machines),
		Config:  config,
	}

	client.initHTTPClient()
	return client
}

// NewTLSClient create a basic client with TLS configuration
func NewTLSClient(machines []string, cert string, key string, caCerts []string) (*Client, error) {
	// overwrite the default machine to use https
	if len(machines) == 0 {
		machines = []string{"https://127.0.0.1:4001"}
	}

	config := Config{
		// default timeout is one second
		DialTimeout: time.Second,
		CertFile:    cert,
		KeyFile:     key,
		CaCertFile:  make([]string, 0),
	}

	client := &Client{
		Cluster: NewCluster(machines),
		Config:  config,
	}

	err := client.initHTTPSClient(cert, key)
	if err != nil {
		return nil, err
	}

	for _, caCert := range caCerts {
		if err := client.AddRootCA(caCert); err != nil {
			return nil, err
		}
	}
	return client, nil
}

// NewClientFromFile creates a client from a given file path.
// The given file is expected to use the JSON format.
func NewClientFromFile(fpath string) (*Client, error) {
	fi, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := fi.Close(); err != nil {
			panic(err)
		}
	}()

	return NewClientFromReader(fi)
}

// NewClientFromReader creates a Client configured from a given reader.
// The configuration is expected to use the JSON format.
func NewClientFromReader(reader io.Reader) (*Client, error) {
	c := new(Client)

	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}
	if c.Config.CertFile == "" {
		c.initHTTPClient()
	} else {
		err = c.initHTTPSClient(c.Config.CertFile, c.Config.KeyFile)
	}

	if err != nil {
		return nil, err
	}

	for _, caCert := range c.Config.CaCertFile {
		if err := c.AddRootCA(caCert); err != nil {
			return nil, err
		}
	}

	return c, nil
}

// Override the Client's HTTP Transport object
func (c *Client) SetTransport(tr *http.Transport) {
	c.httpClient.Transport = tr
}

// initHTTPClient initializes a HTTP client for eureka client
func (c *Client) initHTTPClient() {
	tr := &http.Transport{
		Dial: c.dial,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	c.httpClient = &http.Client{Transport: tr}
}

// initHTTPClient initializes a HTTPS client for eureka client
func (c *Client) initHTTPSClient(cert, key string) error {
	if cert == "" || key == "" {
		return errors.New("Require both cert and key path")
	}

	tlsCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return err
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		InsecureSkipVerify: true,
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
		Dial:            c.dial,
	}

	c.httpClient = &http.Client{Transport: tr}
	return nil
}

// Sets the DialTimeout value
func (c *Client) SetDialTimeout(d time.Duration) {
	c.Config.DialTimeout = d
}

// AddRootCA adds a root CA cert for the eureka client
func (c *Client) AddRootCA(caCert string) error {
	if c.httpClient == nil {
		return errors.New("Client has not been initialized yet!")
	}

	certBytes, err := ioutil.ReadFile(caCert)
	if err != nil {
		return err
	}

	tr, ok := c.httpClient.Transport.(*http.Transport)

	if !ok {
		panic("AddRootCA(): Transport type assert should not fail")
	}

	if tr.TLSClientConfig.RootCAs == nil {
		caCertPool := x509.NewCertPool()
		ok = caCertPool.AppendCertsFromPEM(certBytes)
		if ok {
			tr.TLSClientConfig.RootCAs = caCertPool
		}
		tr.TLSClientConfig.InsecureSkipVerify = false
	} else {
		ok = tr.TLSClientConfig.RootCAs.AppendCertsFromPEM(certBytes)
	}

	if !ok {
		err = errors.New("Unable to load caCert")
	}

	c.Config.CaCertFile = append(c.Config.CaCertFile, caCert)
	return err
}

// SetCluster updates cluster information using the given machine list.
func (c *Client) SetCluster(machines []string) bool {
	success := c.internalSyncCluster(machines)
	return success
}

func (c *Client) GetCluster() []string {
	return c.Cluster.Machines
}

// SyncCluster updates the cluster information using the internal machine list.
func (c *Client) SyncCluster() bool {
	return c.internalSyncCluster(c.Cluster.Machines)
}

// internalSyncCluster syncs cluster information using the given machine list.
func (c *Client) internalSyncCluster(machines []string) bool {
	for _, machine := range machines {
		httpPath := c.createHttpPath(machine, "machines")
		resp, err := c.httpClient.Get(httpPath)
		if err != nil {
			// try another machine in the cluster
			continue
		} else {
			b, err := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				// try another machine in the cluster
				continue
			}

			// update Machines List
			c.Cluster.updateFromStr(string(b))

			// update leader
			// the first one in the machine list is the leader
			c.Cluster.switchLeader(0)

			logger.Debug("sync.machines " + strings.Join(c.Cluster.Machines, ", "))
			return true
		}
	}
	return false
}

// createHttpPath creates a complete HTTP URL.
// serverName should contain both the host name and a port number, if any.
func (c *Client) createHttpPath(serverName string, _path string) string {
	u, err := url.Parse(serverName)
	if err != nil {
		panic(err)
	}

	u.Path = path.Join(u.Path, _path)

	if u.Scheme == "" {
		u.Scheme = "http"
	}
	return u.String()
}

// dial attempts to open a TCP connection to the provided address, explicitly
// enabling keep-alives with a one-second interval.
func (c *Client) dial(network, addr string) (net.Conn, error) {
	conn, err := net.DialTimeout(network, addr, c.Config.DialTimeout)
	if err != nil {
		return nil, err
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, errors.New("Failed type-assertion of net.Conn as *net.TCPConn")
	}

	// Keep TCP alive to check whether or not the remote machine is down
	if err = tcpConn.SetKeepAlive(true); err != nil {
		return nil, err
	}

	if err = tcpConn.SetKeepAlivePeriod(time.Second); err != nil {
		return nil, err
	}

	return tcpConn, nil
}

// MarshalJSON implements the Marshaller interface
// as defined by the standard JSON package.
func (c *Client) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(struct {
		Config  Config   `json:"config"`
		Cluster *Cluster `json:"cluster"`
	}{
		Config:  c.Config,
		Cluster: c.Cluster,
	})

	if err != nil {
		return nil, err
	}

	return b, nil
}

// UnmarshalJSON implements the Unmarshaller interface
// as defined by the standard JSON package.
func (c *Client) UnmarshalJSON(b []byte) error {
	temp := struct {
		Config  Config   `json:"config"`
		Cluster *Cluster `json:"cluster"`
	}{}
	err := json.Unmarshal(b, &temp)
	if err != nil {
		return err
	}

	c.Cluster = temp.Cluster
	c.Config = temp.Config
	return nil
}
