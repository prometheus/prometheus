package godo

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	kubernetesBasePath     = "/v2/kubernetes"
	kubernetesClustersPath = kubernetesBasePath + "/clusters"
	kubernetesOptionsPath  = kubernetesBasePath + "/options"
)

// KubernetesService is an interface for interfacing with the Kubernetes endpoints
// of the DigitalOcean API.
// See: https://developers.digitalocean.com/documentation/v2#kubernetes
type KubernetesService interface {
	Create(context.Context, *KubernetesClusterCreateRequest) (*KubernetesCluster, *Response, error)
	Get(context.Context, string) (*KubernetesCluster, *Response, error)
	GetUser(context.Context, string) (*KubernetesClusterUser, *Response, error)
	GetUpgrades(context.Context, string) ([]*KubernetesVersion, *Response, error)
	GetKubeConfig(context.Context, string) (*KubernetesClusterConfig, *Response, error)
	GetCredentials(context.Context, string, *KubernetesClusterCredentialsGetRequest) (*KubernetesClusterCredentials, *Response, error)
	List(context.Context, *ListOptions) ([]*KubernetesCluster, *Response, error)
	Update(context.Context, string, *KubernetesClusterUpdateRequest) (*KubernetesCluster, *Response, error)
	Upgrade(context.Context, string, *KubernetesClusterUpgradeRequest) (*Response, error)
	Delete(context.Context, string) (*Response, error)

	CreateNodePool(ctx context.Context, clusterID string, req *KubernetesNodePoolCreateRequest) (*KubernetesNodePool, *Response, error)
	GetNodePool(ctx context.Context, clusterID, poolID string) (*KubernetesNodePool, *Response, error)
	ListNodePools(ctx context.Context, clusterID string, opts *ListOptions) ([]*KubernetesNodePool, *Response, error)
	UpdateNodePool(ctx context.Context, clusterID, poolID string, req *KubernetesNodePoolUpdateRequest) (*KubernetesNodePool, *Response, error)
	// RecycleNodePoolNodes is DEPRECATED please use DeleteNode
	// The method will be removed in godo 2.0.
	RecycleNodePoolNodes(ctx context.Context, clusterID, poolID string, req *KubernetesNodePoolRecycleNodesRequest) (*Response, error)
	DeleteNodePool(ctx context.Context, clusterID, poolID string) (*Response, error)
	DeleteNode(ctx context.Context, clusterID, poolID, nodeID string, req *KubernetesNodeDeleteRequest) (*Response, error)

	GetOptions(context.Context) (*KubernetesOptions, *Response, error)
}

var _ KubernetesService = &KubernetesServiceOp{}

// KubernetesServiceOp handles communication with Kubernetes methods of the DigitalOcean API.
type KubernetesServiceOp struct {
	client *Client
}

// KubernetesClusterCreateRequest represents a request to create a Kubernetes cluster.
type KubernetesClusterCreateRequest struct {
	Name        string   `json:"name,omitempty"`
	RegionSlug  string   `json:"region,omitempty"`
	VersionSlug string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	VPCUUID     string   `json:"vpc_uuid,omitempty"`

	NodePools []*KubernetesNodePoolCreateRequest `json:"node_pools,omitempty"`

	MaintenancePolicy *KubernetesMaintenancePolicy `json:"maintenance_policy"`
	AutoUpgrade       bool                         `json:"auto_upgrade"`
}

// KubernetesClusterUpdateRequest represents a request to update a Kubernetes cluster.
type KubernetesClusterUpdateRequest struct {
	Name              string                       `json:"name,omitempty"`
	Tags              []string                     `json:"tags,omitempty"`
	MaintenancePolicy *KubernetesMaintenancePolicy `json:"maintenance_policy,omitempty"`
	AutoUpgrade       *bool                        `json:"auto_upgrade,omitempty"`
}

// KubernetesClusterUpgradeRequest represents a request to upgrade a Kubernetes cluster.
type KubernetesClusterUpgradeRequest struct {
	VersionSlug string `json:"version,omitempty"`
}

// KubernetesNodePoolCreateRequest represents a request to create a node pool for a
// Kubernetes cluster.
type KubernetesNodePoolCreateRequest struct {
	Name      string            `json:"name,omitempty"`
	Size      string            `json:"size,omitempty"`
	Count     int               `json:"count,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	AutoScale bool              `json:"auto_scale,omitempty"`
	MinNodes  int               `json:"min_nodes,omitempty"`
	MaxNodes  int               `json:"max_nodes,omitempty"`
}

// KubernetesNodePoolUpdateRequest represents a request to update a node pool in a
// Kubernetes cluster.
type KubernetesNodePoolUpdateRequest struct {
	Name      string            `json:"name,omitempty"`
	Count     *int              `json:"count,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	AutoScale *bool             `json:"auto_scale,omitempty"`
	MinNodes  *int              `json:"min_nodes,omitempty"`
	MaxNodes  *int              `json:"max_nodes,omitempty"`
}

// KubernetesNodePoolRecycleNodesRequest is DEPRECATED please use DeleteNode
// The type will be removed in godo 2.0.
type KubernetesNodePoolRecycleNodesRequest struct {
	Nodes []string `json:"nodes,omitempty"`
}

// KubernetesNodeDeleteRequest is a request to delete a specific node in a node pool.
type KubernetesNodeDeleteRequest struct {
	// Replace will cause a new node to be created to replace the deleted node.
	Replace bool `json:"replace,omitempty"`

	// SkipDrain skips draining the node before deleting it.
	SkipDrain bool `json:"skip_drain,omitempty"`
}

// KubernetesClusterCredentialsGetRequest is a request to get cluster credentials.
type KubernetesClusterCredentialsGetRequest struct {
	ExpirySeconds *int `json:"expiry_seconds,omitempty"`
}

// KubernetesCluster represents a Kubernetes cluster.
type KubernetesCluster struct {
	ID            string   `json:"id,omitempty"`
	Name          string   `json:"name,omitempty"`
	RegionSlug    string   `json:"region,omitempty"`
	VersionSlug   string   `json:"version,omitempty"`
	ClusterSubnet string   `json:"cluster_subnet,omitempty"`
	ServiceSubnet string   `json:"service_subnet,omitempty"`
	IPv4          string   `json:"ipv4,omitempty"`
	Endpoint      string   `json:"endpoint,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	VPCUUID       string   `json:"vpc_uuid,omitempty"`

	NodePools []*KubernetesNodePool `json:"node_pools,omitempty"`

	MaintenancePolicy *KubernetesMaintenancePolicy `json:"maintenance_policy,omitempty"`
	AutoUpgrade       bool                         `json:"auto_upgrade,omitempty"`

	Status    *KubernetesClusterStatus `json:"status,omitempty"`
	CreatedAt time.Time                `json:"created_at,omitempty"`
	UpdatedAt time.Time                `json:"updated_at,omitempty"`
}

// KubernetesClusterUser represents a Kubernetes cluster user.
type KubernetesClusterUser struct {
	Username string   `json:"username,omitempty"`
	Groups   []string `json:"groups,omitempty"`
}

// KubernetesClusterCredentials represents Kubernetes cluster credentials.
type KubernetesClusterCredentials struct {
	Server                   string    `json:"server"`
	CertificateAuthorityData []byte    `json:"certificate_authority_data"`
	ClientCertificateData    []byte    `json:"client_certificate_data"`
	ClientKeyData            []byte    `json:"client_key_data"`
	Token                    string    `json:"token"`
	ExpiresAt                time.Time `json:"expires_at"`
}

// KubernetesMaintenancePolicy is a configuration to set the maintenance window
// of a cluster
type KubernetesMaintenancePolicy struct {
	StartTime string                         `json:"start_time"`
	Duration  string                         `json:"duration"`
	Day       KubernetesMaintenancePolicyDay `json:"day"`
}

// KubernetesMaintenancePolicyDay represents the possible days of a maintenance
// window
type KubernetesMaintenancePolicyDay int

const (
	KubernetesMaintenanceDayAny KubernetesMaintenancePolicyDay = iota
	KubernetesMaintenanceDayMonday
	KubernetesMaintenanceDayTuesday
	KubernetesMaintenanceDayWednesday
	KubernetesMaintenanceDayThursday
	KubernetesMaintenanceDayFriday
	KubernetesMaintenanceDaySaturday
	KubernetesMaintenanceDaySunday
)

var (
	days = [...]string{
		"any",
		"monday",
		"tuesday",
		"wednesday",
		"thursday",
		"friday",
		"saturday",
		"sunday",
	}

	toDay = map[string]KubernetesMaintenancePolicyDay{
		"any":       KubernetesMaintenanceDayAny,
		"monday":    KubernetesMaintenanceDayMonday,
		"tuesday":   KubernetesMaintenanceDayTuesday,
		"wednesday": KubernetesMaintenanceDayWednesday,
		"thursday":  KubernetesMaintenanceDayThursday,
		"friday":    KubernetesMaintenanceDayFriday,
		"saturday":  KubernetesMaintenanceDaySaturday,
		"sunday":    KubernetesMaintenanceDaySunday,
	}
)

// KubernetesMaintenanceToDay returns the appropriate KubernetesMaintenancePolicyDay for the given string.
func KubernetesMaintenanceToDay(day string) (KubernetesMaintenancePolicyDay, error) {
	d, ok := toDay[day]
	if !ok {
		return 0, fmt.Errorf("unknown day: %q", day)
	}

	return d, nil
}

func (k KubernetesMaintenancePolicyDay) String() string {
	if KubernetesMaintenanceDayAny <= k && k <= KubernetesMaintenanceDaySunday {
		return days[k]
	}
	return fmt.Sprintf("%d !Weekday", k)

}

func (k *KubernetesMaintenancePolicyDay) UnmarshalJSON(data []byte) error {
	var val string
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}

	parsed, err := KubernetesMaintenanceToDay(val)
	if err != nil {
		return err
	}
	*k = parsed
	return nil
}

func (k KubernetesMaintenancePolicyDay) MarshalJSON() ([]byte, error) {
	if KubernetesMaintenanceDayAny <= k && k <= KubernetesMaintenanceDaySunday {
		return json.Marshal(days[k])
	}

	return nil, fmt.Errorf("invalid day: %d", k)
}

// Possible states for a cluster.
const (
	KubernetesClusterStatusProvisioning = KubernetesClusterStatusState("provisioning")
	KubernetesClusterStatusRunning      = KubernetesClusterStatusState("running")
	KubernetesClusterStatusDegraded     = KubernetesClusterStatusState("degraded")
	KubernetesClusterStatusError        = KubernetesClusterStatusState("error")
	KubernetesClusterStatusDeleted      = KubernetesClusterStatusState("deleted")
	KubernetesClusterStatusUpgrading    = KubernetesClusterStatusState("upgrading")
	KubernetesClusterStatusInvalid      = KubernetesClusterStatusState("invalid")
)

// KubernetesClusterStatusState represents states for a cluster.
type KubernetesClusterStatusState string

var _ encoding.TextUnmarshaler = (*KubernetesClusterStatusState)(nil)

// UnmarshalText unmarshals the state.
func (s *KubernetesClusterStatusState) UnmarshalText(text []byte) error {
	switch KubernetesClusterStatusState(strings.ToLower(string(text))) {
	case KubernetesClusterStatusProvisioning:
		*s = KubernetesClusterStatusProvisioning
	case KubernetesClusterStatusRunning:
		*s = KubernetesClusterStatusRunning
	case KubernetesClusterStatusDegraded:
		*s = KubernetesClusterStatusDegraded
	case KubernetesClusterStatusError:
		*s = KubernetesClusterStatusError
	case KubernetesClusterStatusDeleted:
		*s = KubernetesClusterStatusDeleted
	case KubernetesClusterStatusUpgrading:
		*s = KubernetesClusterStatusUpgrading
	case "", KubernetesClusterStatusInvalid:
		*s = KubernetesClusterStatusInvalid
	default:
		return fmt.Errorf("unknown cluster state %q", string(text))
	}
	return nil
}

// KubernetesClusterStatus describes the status of a cluster.
type KubernetesClusterStatus struct {
	State   KubernetesClusterStatusState `json:"state,omitempty"`
	Message string                       `json:"message,omitempty"`
}

// KubernetesNodePool represents a node pool in a Kubernetes cluster.
type KubernetesNodePool struct {
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Size      string            `json:"size,omitempty"`
	Count     int               `json:"count,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	AutoScale bool              `json:"auto_scale,omitempty"`
	MinNodes  int               `json:"min_nodes,omitempty"`
	MaxNodes  int               `json:"max_nodes,omitempty"`

	Nodes []*KubernetesNode `json:"nodes,omitempty"`
}

// KubernetesNode represents a Node in a node pool in a Kubernetes cluster.
type KubernetesNode struct {
	ID        string                `json:"id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Status    *KubernetesNodeStatus `json:"status,omitempty"`
	DropletID string                `json:"droplet_id,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// KubernetesNodeStatus represents the status of a particular Node in a Kubernetes cluster.
type KubernetesNodeStatus struct {
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// KubernetesOptions represents options available for creating Kubernetes clusters.
type KubernetesOptions struct {
	Versions []*KubernetesVersion  `json:"versions,omitempty"`
	Regions  []*KubernetesRegion   `json:"regions,omitempty"`
	Sizes    []*KubernetesNodeSize `json:"sizes,omitempty"`
}

// KubernetesVersion is a DigitalOcean Kubernetes release.
type KubernetesVersion struct {
	Slug              string `json:"slug,omitempty"`
	KubernetesVersion string `json:"kubernetes_version,omitempty"`
}

// KubernetesNodeSize is a node sizes supported for Kubernetes clusters.
type KubernetesNodeSize struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// KubernetesRegion is a region usable by Kubernetes clusters.
type KubernetesRegion struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type kubernetesClustersRoot struct {
	Clusters []*KubernetesCluster `json:"kubernetes_clusters,omitempty"`
	Links    *Links               `json:"links,omitempty"`
	Meta     *Meta                `json:"meta"`
}

type kubernetesClusterRoot struct {
	Cluster *KubernetesCluster `json:"kubernetes_cluster,omitempty"`
}

type kubernetesClusterUserRoot struct {
	User *KubernetesClusterUser `json:"kubernetes_cluster_user,omitempty"`
}

type kubernetesNodePoolRoot struct {
	NodePool *KubernetesNodePool `json:"node_pool,omitempty"`
}

type kubernetesNodePoolsRoot struct {
	NodePools []*KubernetesNodePool `json:"node_pools,omitempty"`
	Links     *Links                `json:"links,omitempty"`
}

type kubernetesUpgradesRoot struct {
	AvailableUpgradeVersions []*KubernetesVersion `json:"available_upgrade_versions,omitempty"`
}

// Get retrieves the details of a Kubernetes cluster.
func (svc *KubernetesServiceOp) Get(ctx context.Context, clusterID string) (*KubernetesCluster, *Response, error) {
	path := fmt.Sprintf("%s/%s", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesClusterRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Cluster, resp, nil
}

// GetUser retrieves the details of a Kubernetes cluster user.
func (svc *KubernetesServiceOp) GetUser(ctx context.Context, clusterID string) (*KubernetesClusterUser, *Response, error) {
	path := fmt.Sprintf("%s/%s/user", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesClusterUserRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.User, resp, nil
}

// GetUpgrades retrieves versions a Kubernetes cluster can be upgraded to. An
// upgrade can be requested using `Upgrade`.
func (svc *KubernetesServiceOp) GetUpgrades(ctx context.Context, clusterID string) ([]*KubernetesVersion, *Response, error) {
	path := fmt.Sprintf("%s/%s/upgrades", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesUpgradesRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, nil, err
	}
	return root.AvailableUpgradeVersions, resp, nil
}

// Create creates a Kubernetes cluster.
func (svc *KubernetesServiceOp) Create(ctx context.Context, create *KubernetesClusterCreateRequest) (*KubernetesCluster, *Response, error) {
	path := kubernetesClustersPath
	req, err := svc.client.NewRequest(ctx, http.MethodPost, path, create)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesClusterRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Cluster, resp, nil
}

// Delete deletes a Kubernetes cluster. There is no way to recover a cluster
// once it has been destroyed.
func (svc *KubernetesServiceOp) Delete(ctx context.Context, clusterID string) (*Response, error) {
	path := fmt.Sprintf("%s/%s", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := svc.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// List returns a list of the Kubernetes clusters visible with the caller's API token.
func (svc *KubernetesServiceOp) List(ctx context.Context, opts *ListOptions) ([]*KubernetesCluster, *Response, error) {
	path := kubernetesClustersPath
	path, err := addOptions(path, opts)
	if err != nil {
		return nil, nil, err
	}
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesClustersRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}

	if l := root.Links; l != nil {
		resp.Links = l
	}
	if m := root.Meta; m != nil {
		resp.Meta = m
	}

	return root.Clusters, resp, nil
}

// KubernetesClusterConfig is the content of a Kubernetes config file, which can be
// used to interact with your Kubernetes cluster using `kubectl`.
// See: https://kubernetes.io/docs/tasks/tools/install-kubectl/
type KubernetesClusterConfig struct {
	KubeconfigYAML []byte
}

// GetKubeConfig returns a Kubernetes config file for the specified cluster.
func (svc *KubernetesServiceOp) GetKubeConfig(ctx context.Context, clusterID string) (*KubernetesClusterConfig, *Response, error) {
	path := fmt.Sprintf("%s/%s/kubeconfig", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	configBytes := bytes.NewBuffer(nil)
	resp, err := svc.client.Do(ctx, req, configBytes)
	if err != nil {
		return nil, resp, err
	}
	res := &KubernetesClusterConfig{
		KubeconfigYAML: configBytes.Bytes(),
	}
	return res, resp, nil
}

// GetCredentials returns a Kubernetes API server credentials for the specified cluster.
func (svc *KubernetesServiceOp) GetCredentials(ctx context.Context, clusterID string, get *KubernetesClusterCredentialsGetRequest) (*KubernetesClusterCredentials, *Response, error) {
	path := fmt.Sprintf("%s/%s/credentials", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	q := req.URL.Query()
	if get.ExpirySeconds != nil {
		q.Add("expiry_seconds", strconv.Itoa(*get.ExpirySeconds))
	}
	req.URL.RawQuery = q.Encode()
	credentials := new(KubernetesClusterCredentials)
	resp, err := svc.client.Do(ctx, req, credentials)
	if err != nil {
		return nil, nil, err
	}
	return credentials, resp, nil
}

// Update updates a Kubernetes cluster's properties.
func (svc *KubernetesServiceOp) Update(ctx context.Context, clusterID string, update *KubernetesClusterUpdateRequest) (*KubernetesCluster, *Response, error) {
	path := fmt.Sprintf("%s/%s", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodPut, path, update)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesClusterRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Cluster, resp, nil
}

// Upgrade upgrades a Kubernetes cluster to a new version. Valid upgrade
// versions for a given cluster can be retrieved with `GetUpgrades`.
func (svc *KubernetesServiceOp) Upgrade(ctx context.Context, clusterID string, upgrade *KubernetesClusterUpgradeRequest) (*Response, error) {
	path := fmt.Sprintf("%s/%s/upgrade", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodPost, path, upgrade)
	if err != nil {
		return nil, err
	}
	return svc.client.Do(ctx, req, nil)
}

// CreateNodePool creates a new node pool in an existing Kubernetes cluster.
func (svc *KubernetesServiceOp) CreateNodePool(ctx context.Context, clusterID string, create *KubernetesNodePoolCreateRequest) (*KubernetesNodePool, *Response, error) {
	path := fmt.Sprintf("%s/%s/node_pools", kubernetesClustersPath, clusterID)
	req, err := svc.client.NewRequest(ctx, http.MethodPost, path, create)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesNodePoolRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.NodePool, resp, nil
}

// GetNodePool retrieves an existing node pool in a Kubernetes cluster.
func (svc *KubernetesServiceOp) GetNodePool(ctx context.Context, clusterID, poolID string) (*KubernetesNodePool, *Response, error) {
	path := fmt.Sprintf("%s/%s/node_pools/%s", kubernetesClustersPath, clusterID, poolID)
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesNodePoolRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.NodePool, resp, nil
}

// ListNodePools lists all the node pools found in a Kubernetes cluster.
func (svc *KubernetesServiceOp) ListNodePools(ctx context.Context, clusterID string, opts *ListOptions) ([]*KubernetesNodePool, *Response, error) {
	path := fmt.Sprintf("%s/%s/node_pools", kubernetesClustersPath, clusterID)
	path, err := addOptions(path, opts)
	if err != nil {
		return nil, nil, err
	}
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesNodePoolsRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.NodePools, resp, nil
}

// UpdateNodePool updates the details of an existing node pool.
func (svc *KubernetesServiceOp) UpdateNodePool(ctx context.Context, clusterID, poolID string, update *KubernetesNodePoolUpdateRequest) (*KubernetesNodePool, *Response, error) {
	path := fmt.Sprintf("%s/%s/node_pools/%s", kubernetesClustersPath, clusterID, poolID)
	req, err := svc.client.NewRequest(ctx, http.MethodPut, path, update)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesNodePoolRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.NodePool, resp, nil
}

// RecycleNodePoolNodes is DEPRECATED please use DeleteNode
// The method will be removed in godo 2.0.
func (svc *KubernetesServiceOp) RecycleNodePoolNodes(ctx context.Context, clusterID, poolID string, recycle *KubernetesNodePoolRecycleNodesRequest) (*Response, error) {
	path := fmt.Sprintf("%s/%s/node_pools/%s/recycle", kubernetesClustersPath, clusterID, poolID)
	req, err := svc.client.NewRequest(ctx, http.MethodPost, path, recycle)
	if err != nil {
		return nil, err
	}
	resp, err := svc.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// DeleteNodePool deletes a node pool, and subsequently all the nodes in that pool.
func (svc *KubernetesServiceOp) DeleteNodePool(ctx context.Context, clusterID, poolID string) (*Response, error) {
	path := fmt.Sprintf("%s/%s/node_pools/%s", kubernetesClustersPath, clusterID, poolID)
	req, err := svc.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := svc.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// DeleteNode deletes a specific node in a node pool.
func (svc *KubernetesServiceOp) DeleteNode(ctx context.Context, clusterID, poolID, nodeID string, deleteReq *KubernetesNodeDeleteRequest) (*Response, error) {
	path := fmt.Sprintf("%s/%s/node_pools/%s/nodes/%s", kubernetesClustersPath, clusterID, poolID, nodeID)
	if deleteReq != nil {
		v := make(url.Values)
		if deleteReq.SkipDrain {
			v.Set("skip_drain", "1")
		}
		if deleteReq.Replace {
			v.Set("replace", "1")
		}
		if query := v.Encode(); query != "" {
			path = path + "?" + query
		}
	}

	req, err := svc.client.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := svc.client.Do(ctx, req, nil)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

type kubernetesOptionsRoot struct {
	Options *KubernetesOptions `json:"options,omitempty"`
	Links   *Links             `json:"links,omitempty"`
}

// GetOptions returns options about the Kubernetes service, such as the versions available for
// cluster creation.
func (svc *KubernetesServiceOp) GetOptions(ctx context.Context) (*KubernetesOptions, *Response, error) {
	path := kubernetesOptionsPath
	req, err := svc.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	root := new(kubernetesOptionsRoot)
	resp, err := svc.client.Do(ctx, req, root)
	if err != nil {
		return nil, resp, err
	}
	return root.Options, resp, nil
}
