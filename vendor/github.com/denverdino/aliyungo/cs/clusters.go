package cs

import (
	"net/http"
	"net/url"

	"math"
	"strconv"
	"time"

	"fmt"

	"encoding/json"
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/ecs"
)

type ClusterState string

const (
	Initial      = ClusterState("initial")
	Running      = ClusterState("running")
	Updating     = ClusterState("updating")
	Scaling      = ClusterState("scaling")
	Failed       = ClusterState("failed")
	Deleting     = ClusterState("deleting")
	DeleteFailed = ClusterState("deleteFailed")
	Deleted      = ClusterState("deleted")
	InActive     = ClusterState("inactive")

	ClusterTypeKubernetes        = "Kubernetes"
	ClusterTypeManagedKubernetes = "ManagedKubernetes"
)

var NodeStableClusterState = []ClusterState{Running, Updating, Failed, DeleteFailed, Deleted, InActive}

var NodeUnstableClusterState = []ClusterState{Initial, Scaling, Deleting}

type NodeStatus struct {
	Health   int64 `json:"health"`
	Unhealth int64 `json:"unhealth"`
}

type NetworkModeType string

const (
	ClassicNetwork = NetworkModeType("classic")
	VPCNetwork     = NetworkModeType("vpc")
)

// https://help.aliyun.com/document_detail/26053.html
type ClusterType struct {
	AgentVersion           string          `json:"agent_version"`
	ClusterID              string          `json:"cluster_id"`
	Name                   string          `json:"name"`
	Created                time.Time       `json:"created"`
	ExternalLoadbalancerID string          `json:"external_loadbalancer_id"`
	MasterURL              string          `json:"master_url"`
	NetworkMode            NetworkModeType `json:"network_mode"`
	RegionID               common.Region   `json:"region_id"`
	SecurityGroupID        string          `json:"security_group_id"`
	Size                   int64           `json:"size"`
	State                  ClusterState    `json:"state"`
	Updated                time.Time       `json:"updated"`
	VPCID                  string          `json:"vpc_id"`
	VSwitchID              string          `json:"vswitch_id"`
	NodeStatus             string          `json:"node_status"`
	DockerVersion          string          `json:"docker_version"`
	ClusterType            string          `json:"cluster_type"`
}

func (client *Client) DescribeClusters(nameFilter string) (clusters []ClusterType, err error) {
	query := make(url.Values)

	if nameFilter != "" {
		query.Add("name", nameFilter)
	}

	err = client.Invoke("", http.MethodGet, "/clusters", query, nil, &clusters)
	return
}

func (client *Client) DescribeCluster(id string) (cluster ClusterType, err error) {
	err = client.Invoke("", http.MethodGet, "/clusters/"+id, nil, nil, &cluster)
	return
}

type ClusterCreationArgs struct {
	Name             string           `json:"name"`
	Size             int64            `json:"size"`
	NetworkMode      NetworkModeType  `json:"network_mode"`
	SubnetCIDR       string           `json:"subnet_cidr,omitempty"`
	InstanceType     string           `json:"instance_type"`
	VPCID            string           `json:"vpc_id,omitempty"`
	VSwitchID        string           `json:"vswitch_id,omitempty"`
	Password         string           `json:"password"`
	DataDiskSize     int64            `json:"data_disk_size"`
	DataDiskCategory ecs.DiskCategory `json:"data_disk_category"`
	ECSImageID       string           `json:"ecs_image_id,omitempty"`
	IOOptimized      ecs.IoOptimized  `json:"io_optimized"`
	ReleaseEipFlag   bool             `json:"release_eip_flag"`
	NeedSLB          bool             `json:"need_slb"`
}

type ClusterCreationResponse struct {
	Response
	ClusterID string `json:"cluster_id"`
}

func (client *Client) CreateCluster(region common.Region, args *ClusterCreationArgs) (cluster ClusterCreationResponse, err error) {
	err = client.Invoke(region, http.MethodPost, "/clusters", nil, args, &cluster)
	return
}

// Deprecated
type KubernetesStackArgs struct {
	VPCID                    string           `json:"VpcId,omitempty"`
	VSwitchID                string           `json:"VSwitchId,omitempty"`
	MasterInstanceType       string           `json:"MasterInstanceType,omitempty"`
	WorkerInstanceType       string           `json:"WorkerInstanceType,omitempty"`
	NumOfNodes               int64            `json:"NumOfNodes,omitempty"`
	Password                 string           `json:"LoginPassword,omitempty"`
	DockerVersion            string           `json:"DockerVersion,omitempty"`
	KubernetesVersion        string           `json:"KubernetesVersion,omitempty"`
	ZoneId                   string           `json:"ZoneId,omitempty"`
	ContainerCIDR            string           `json:"ContainerCIDR,omitempty"`
	ServiceCIDR              string           `json:"ServiceCIDR,omitempty"`
	SSHFlags                 bool             `json:"SSHFlags,omitempty"`
	MasterSystemDiskSize     int64            `json:"MasterSystemDiskSize,omitempty"`
	MasterSystemDiskCategory ecs.DiskCategory `json:"MasterSystemDiskCategory,omitempty"`
	WorkerSystemDiskSize     int64            `json:"WorkerSystemDiskSize,omitempty"`
	WorkerSystemDiskCategory ecs.DiskCategory `json:"WorkerSystemDiskCategory,omitempty"`
	ImageID                  string           `json:"ImageId,omitempty"`
	CloudMonitorFlags        bool             `json:"CloudMonitorFlags,omitempty"`
	SNatEntry                bool             `json:"SNatEntry,omitempty"`
}

type KubernetesCreationArgs struct {
	DisableRollback bool   `json:"disable_rollback"`
	Name            string `json:"name"`
	TimeoutMins     int64  `json:"timeout_mins"`
	ZoneId          string `json:"zoneid,omitempty"`
	VPCID           string `json:"vpcid,omitempty"`
	VSwitchId       string `json:"vswitchid,omitempty"`
	ImageId         string `json:"image_id"`
	ContainerCIDR   string `json:"container_cidr,omitempty"`
	ServiceCIDR     string `json:"service_cidr,omitempty"`

	MasterInstanceType       string           `json:"master_instance_type,omitempty"`
	MasterSystemDiskSize     int64            `json:"master_system_disk_size,omitempty"`
	MasterSystemDiskCategory ecs.DiskCategory `json:"master_system_disk_category,omitempty"`

	MasterInstanceChargeType string `json:"master_instance_charge_type"`
	MasterPeriodUnit         string `json:"master_period_unit"`
	MasterPeriod             int    `json:"master_period"`
	MasterAutoRenew          bool   `json:"master_auto_renew"`
	MasterAutoRenewPeriod    int    `json:"master_auto_renew_period"`

	WorkerInstanceType       string           `json:"worker_instance_type,omitempty"`
	WorkerSystemDiskSize     int64            `json:"worker_system_disk_size,omitempty"`
	WorkerSystemDiskCategory ecs.DiskCategory `json:"worker_system_disk_category,omitempty"`
	WorkerDataDisk           bool             `json:"worker_data_disk"`
	WorkerDataDiskCategory   string           `json:"worker_data_disk_category,omitempty"`
	WorkerDataDiskSize       int64            `json:"worker_data_disk_size,omitempty"`

	WorkerInstanceChargeType string `json:"worker_instance_charge_type"`
	WorkerPeriodUnit         string `json:"worker_period_unit"`
	WorkerPeriod             int    `json:"worker_period"`
	WorkerAutoRenew          bool   `json:"worker_auto_renew"`
	WorkerAutoRenewPeriod    int    `json:"worker_auto_renew_period"`

	LoginPassword     string `json:"login_password,omitempty"`
	KeyPair           string `json:"key_pair,omitempty"`
	NumOfNodes        int64  `json:"num_of_nodes,omitempty"`
	SNatEntry         bool   `json:"snat_entry"`
	SSHFlags          bool   `json:"ssh_flags"`
	CloudMonitorFlags bool   `json:"cloud_monitor_flags"`
	NodeCIDRMask      string `json:"node_cidr_mask,omitempty"`
	LoggingType       string `json:"logging_type,omitempty"`
	SLSProjectName    string `json:"sls_project_name,omitempty"`
	PublicSLB         bool   `json:"public_slb"`

	ClusterType string `json:"cluster_type"`
	Network     string `json:"network,omitempty"`

	KubernetesVersion string              `json:"kubernetes_version,omitempty"`
	StackParams       KubernetesStackArgs `json:"stack_params,omitempty"`
}

type KubernetesMultiAZCreationArgs struct {
	DisableRollback bool   `json:"disable_rollback"`
	Name            string `json:"name"`
	TimeoutMins     int64  `json:"timeout_mins"`
	ClusterType     string `json:"cluster_type"`
	MultiAZ         bool   `json:"multi_az"`
	VPCID           string `json:"vpcid,omitempty"`
	ImageId         string `json:"image_id"`
	ContainerCIDR   string `json:"container_cidr"`
	ServiceCIDR     string `json:"service_cidr"`
	VSwitchIdA      string `json:"vswitch_id_a,omitempty"`
	VSwitchIdB      string `json:"vswitch_id_b,omitempty"`
	VSwitchIdC      string `json:"vswitch_id_c,omitempty"`

	MasterInstanceTypeA      string           `json:"master_instance_type_a,omitempty"`
	MasterInstanceTypeB      string           `json:"master_instance_type_b,omitempty"`
	MasterInstanceTypeC      string           `json:"master_instance_type_c,omitempty"`
	MasterSystemDiskCategory ecs.DiskCategory `json:"master_system_disk_category"`
	MasterSystemDiskSize     int64            `json:"master_system_disk_size"`

	MasterInstanceChargeType string `json:"master_instance_charge_type"`
	MasterPeriodUnit         string `json:"master_period_unit"`
	MasterPeriod             int    `json:"master_period"`
	MasterAutoRenew          bool   `json:"master_auto_renew"`
	MasterAutoRenewPeriod    int    `json:"master_auto_renew_period"`

	WorkerInstanceTypeA      string           `json:"worker_instance_type_a,omitempty"`
	WorkerInstanceTypeB      string           `json:"worker_instance_type_b,omitempty"`
	WorkerInstanceTypeC      string           `json:"worker_instance_type_c,omitempty"`
	WorkerSystemDiskCategory ecs.DiskCategory `json:"worker_system_disk_category"`
	WorkerSystemDiskSize     int64            `json:"worker_system_disk_size"`
	WorkerDataDisk           bool             `json:"worker_data_disk"`
	WorkerDataDiskCategory   string           `json:"worker_data_disk_category"`
	WorkerDataDiskSize       int64            `json:"worker_data_disk_size"`

	WorkerInstanceChargeType string `json:"worker_instance_charge_type"`
	WorkerPeriodUnit         string `json:"worker_period_unit"`
	WorkerPeriod             int    `json:"worker_period"`
	WorkerAutoRenew          bool   `json:"worker_auto_renew"`
	WorkerAutoRenewPeriod    int    `json:"worker_auto_renew_period"`

	NumOfNodesA       int64  `json:"num_of_nodes_a"`
	NumOfNodesB       int64  `json:"num_of_nodes_b"`
	NumOfNodesC       int64  `json:"num_of_nodes_c"`
	LoginPassword     string `json:"login_password,omitempty"`
	KeyPair           string `json:"key_pair,omitempty"`
	SSHFlags          bool   `json:"ssh_flags"`
	CloudMonitorFlags bool   `json:"cloud_monitor_flags"`
	NodeCIDRMask      string `json:"node_cidr_mask,omitempty"`
	LoggingType       string `json:"logging_type,omitempty"`
	SLSProjectName    string `json:"sls_project_name,omitempty"`
	PublicSLB         bool   `json:"public_slb"`

	KubernetesVersion string `json:"kubernetes_version,omitempty"`
	Network           string `json:"network,omitempty"`
}

func (client *Client) CreateKubernetesMultiAZCluster(region common.Region, args *KubernetesMultiAZCreationArgs) (cluster ClusterCreationResponse, err error) {
	err = client.Invoke(region, http.MethodPost, "/clusters", nil, args, &cluster)
	return
}

func (client *Client) CreateKubernetesCluster(region common.Region, args *KubernetesCreationArgs) (cluster ClusterCreationResponse, err error) {
	err = client.Invoke(region, http.MethodPost, "/clusters", nil, args, &cluster)
	return
}

type KubernetesClusterMetaData struct {
	DockerVersion     string `json:"DockerVersion"`
	EtcdVersion       string `json:"EtcdVersion"`
	KubernetesVersion string `json:"KubernetesVersion"`
	MultiAZ           bool   `json:"MultiAZ"`
	SubClass          string `json:"SubClass"`
}

type KubernetesClusterParameter struct {
	ServiceCidr       string `json:"ServiceCIDR"`
	ContainerCidr     string `json:"ContainerCIDR"`
	DockerVersion     string `json:"DockerVersion"`
	EtcdVersion       string `json:"EtcdVersion"`
	KubernetesVersion string `json:"KubernetesVersion"`
	VPCID             string `json:"VpcId"`
	ImageId           string `json:"ImageId"`
	KeyPair           string `json:"KeyPair"`

	MasterSystemDiskCategory string `json:"MasterSystemDiskCategory"`
	MasterSystemDiskSize     string `json:"MasterSystemDiskSize"`
	MasterImageId            string `json:"MasterImageId"`

	MasterInstanceChargeType string `json:"MasterInstanceChargeType"`
	MasterPeriodUnit         string `json:"MasterPeriodUnit"`
	MasterPeriod             string `json:"MasterPeriod"`
	MasterAutoRenew          bool
	RawMasterAutoRenew       string `json:"MasterAutoRenew"`
	MasterAutoRenewPeriod    string `json:"MasterAutoRenewPeriod"`

	WorkerSystemDiskCategory string `json:"WorkerSystemDiskCategory"`
	WorkerSystemDiskSize     string `json:"WorkerSystemDiskSize"`
	WorkerImageId            string `json:"WorkerImageId"`
	WorkerDataDisk           bool
	RawWorkerDataDisk        string `json:"WorkerDataDisk"`
	WorkerDataDiskCategory   string `json:"WorkerDataDiskCategory"`
	WorkerDataDiskSize       string `json:"WorkerDataDiskSize"`

	WorkerInstanceChargeType string `json:"WorkerInstanceChargeType"`
	WorkerPeriodUnit         string `json:"WorkerPeriodUnit"`
	WorkerPeriod             string `json:"WorkerPeriod"`
	WorkerAutoRenew          bool
	RawWorkerAutoRenew       string `json:"WorkerAutoRenew"`
	WorkerAutoRenewPeriod    string `json:"WorkerAutoRenewPeriod"`

	ZoneId         string `json:"ZoneId"`
	NodeCIDRMask   string `json:"NodeCIDRMask"`
	LoggingType    string `json:"LoggingType"`
	SLSProjectName string `json:"SLSProjectName"`
	PublicSLB      bool
	RawPublicSLB   string `json:"PublicSLB"`

	// Single AZ
	MasterInstanceType string `json:"MasterInstanceType"`
	WorkerInstanceType string `json:"WorkerInstanceType"`
	NumOfNodes         string `json:"NumOfNodes"`
	VSwitchID          string `json:"VSwitchId"`

	// Multi AZ
	MasterInstanceTypeA string `json:"MasterInstanceTypeA"`
	MasterInstanceTypeB string `json:"MasterInstanceTypeB"`
	MasterInstanceTypeC string `json:"MasterInstanceTypeC"`
	WorkerInstanceTypeA string `json:"WorkerInstanceTypeA"`
	WorkerInstanceTypeB string `json:"WorkerInstanceTypeB"`
	WorkerInstanceTypeC string `json:"WorkerInstanceTypeC"`
	NumOfNodesA         string `json:"NumOfNodesA"`
	NumOfNodesB         string `json:"NumOfNodesB"`
	NumOfNodesC         string `json:"NumOfNodesC"`
	VSwitchIdA          string `json:"VSwitchIdA"`
	VSwitchIdB          string `json:"VSwitchIdB"`
	VSwitchIdC          string `json:"VSwitchIdC"`
}

type KubernetesCluster struct {
	ClusterType

	AgentVersion    string          `json:"agent_version"`
	ClusterID       string          `json:"cluster_id"`
	Name            string          `json:"name"`
	Created         time.Time       `json:"created"`
	MasterURL       string          `json:"master_url"`
	NetworkMode     NetworkModeType `json:"network_mode"`
	RegionID        common.Region   `json:"region_id"`
	SecurityGroupID string          `json:"security_group_id"`
	Size            int64           `json:"size"`
	State           ClusterState    `json:"state"`
	Updated         time.Time       `json:"updated"`
	VPCID           string          `json:"vpc_id"`
	VSwitchID       string          `json:"vswitch_id"`
	NodeStatus      string          `json:"node_status"`
	DockerVersion   string          `json:"docker_version"`

	ZoneId                 string `json:"zone_id"`
	RawMetaData            string `json:"meta_data,omitempty"`
	MetaData               KubernetesClusterMetaData
	InitVersion            string `json:"init_version"`
	CurrentVersion         string `json:"current_version"`
	ExternalLoadbalancerId string `json:"external_loadbalancer_id"`

	Parameters KubernetesClusterParameter `json:"parameters"`
}

func (client *Client) DescribeKubernetesCluster(id string) (cluster KubernetesCluster, err error) {
	err = client.Invoke("", http.MethodGet, "/clusters/"+id, nil, nil, &cluster)
	if err != nil {
		return cluster, err
	}
	var metaData KubernetesClusterMetaData
	err = json.Unmarshal([]byte(cluster.RawMetaData), &metaData)
	cluster.MetaData = metaData
	cluster.RawMetaData = ""
	cluster.Parameters.WorkerDataDisk, err = strconv.ParseBool(cluster.Parameters.RawWorkerDataDisk)
	if err != nil {
		return cluster, err
	}
	cluster.Parameters.PublicSLB, err = strconv.ParseBool(cluster.Parameters.RawPublicSLB)
	if err != nil && cluster.ClusterType.ClusterType != ClusterTypeManagedKubernetes {
		return cluster, err
	}
	cluster.Parameters.MasterAutoRenew, err = strconv.ParseBool(cluster.Parameters.RawMasterAutoRenew)
	if err != nil && cluster.ClusterType.ClusterType != ClusterTypeManagedKubernetes {
		return cluster, err
	}
	cluster.Parameters.WorkerAutoRenew, err = strconv.ParseBool(cluster.Parameters.RawWorkerAutoRenew)
	if err != nil {
		return cluster, err
	}
	return
}

type ClusterResizeArgs struct {
	Size             int64            `json:"size"`
	InstanceType     string           `json:"instance_type"`
	Password         string           `json:"password"`
	DataDiskSize     int64            `json:"data_disk_size"`
	DataDiskCategory ecs.DiskCategory `json:"data_disk_category"`
	ECSImageID       string           `json:"ecs_image_id,omitempty"`
	IOOptimized      ecs.IoOptimized  `json:"io_optimized"`
}

type ModifyClusterNameArgs struct {
	Name string `json:"name"`
}

func (client *Client) ResizeCluster(clusterID string, args *ClusterResizeArgs) error {
	return client.Invoke("", http.MethodPut, "/clusters/"+clusterID, nil, args, nil)
}

// deprecated
// use ResizeKubernetesCluster instead
func (client *Client) ResizeKubernetes(clusterID string, args *KubernetesCreationArgs) error {
	return client.Invoke("", http.MethodPut, "/clusters/"+clusterID, nil, args, nil)
}

type KubernetesClusterResizeArgs struct {
	DisableRollback bool   `json:"disable_rollback"`
	TimeoutMins     int64  `json:"timeout_mins"`
	LoginPassword   string `json:"login_password,omitempty"`

	// Single AZ
	WorkerInstanceType string `json:"worker_instance_type"`
	NumOfNodes         int64  `json:"num_of_nodes"`

	// Multi AZ
	WorkerInstanceTypeA string `json:"worker_instance_type_a"`
	WorkerInstanceTypeB string `json:"worker_instance_type_b"`
	WorkerInstanceTypeC string `json:"worker_instance_type_c"`
	NumOfNodesA         int64  `json:"num_of_nodes_a"`
	NumOfNodesB         int64  `json:"num_of_nodes_b"`
	NumOfNodesC         int64  `json:"num_of_nodes_c"`
}

func (client *Client) ResizeKubernetesCluster(clusterID string, args *KubernetesClusterResizeArgs) error {
	return client.Invoke("", http.MethodPut, "/clusters/"+clusterID, nil, args, nil)
}

func (client *Client) ModifyClusterName(clusterID, clusterName string) error {
	return client.Invoke("", http.MethodPost, "/clusters/"+clusterID+"/name/"+clusterName, nil, nil, nil)
}

func (client *Client) DeleteCluster(clusterID string) error {
	return client.Invoke("", http.MethodDelete, "/clusters/"+clusterID, nil, nil, nil)
}

type ClusterCerts struct {
	CA   string `json:"ca,omitempty"`
	Key  string `json:"key,omitempty"`
	Cert string `json:"cert,omitempty"`
}

func (client *Client) GetClusterCerts(id string) (certs ClusterCerts, err error) {
	err = client.Invoke("", http.MethodGet, "/clusters/"+id+"/certs", nil, nil, &certs)
	return
}

type ClusterConfig struct {
	Config string `json:"config"`
}

func (client *Client) GetClusterConfig(id string) (config ClusterConfig, err error) {
	err = client.Invoke("", http.MethodGet, "/k8s/"+id+"/user_config", nil, nil, &config)
	return
}

type KubernetesNodeType struct {
	InstanceType       string   `json:"instance_type"`
	IpAddress          []string `json:"ip_address"`
	InstanceChargeType string   `json:"instance_charge_type"`
	InstanceRole       string   `json:"instance_role"`
	CreationTime       string   `json:"creation_time"`
	InstanceName       string   `json:"instance_name"`
	InstanceTypeFamily string   `json:"instance_type_family"`
	HostName           string   `json:"host_name"`
	ImageId            string   `json:"image_id"`
	InstanceId         string   `json:"instance_id"`
}

type GetKubernetesClusterNodesResponse struct {
	Response
	Page  PaginationResult     `json:"page"`
	Nodes []KubernetesNodeType `json:"nodes"`
}

func (client *Client) GetKubernetesClusterNodes(id string, pagination common.Pagination) (nodes []KubernetesNodeType, paginationResult *PaginationResult, err error) {
	response := &GetKubernetesClusterNodesResponse{}
	err = client.Invoke("", http.MethodGet, "/clusters/"+id+"/nodes?pageNumber="+strconv.Itoa(pagination.PageNumber)+"&pageSize="+strconv.Itoa(pagination.PageSize), nil, nil, &response)
	if err != nil {
		return nil, nil, err
	}

	return response.Nodes, &response.Page, nil
}

const ClusterDefaultTimeout = 300
const DefaultWaitForInterval = 10
const DefaultPreCheckSleepTime = 20
const DefaultPreSleepTime = 220

// WaitForCluster waits for instance to given status
// when instance.NotFound wait until timeout
func (client *Client) WaitForClusterAsyn(clusterId string, status ClusterState, timeout int) error {
	if timeout <= 0 {
		timeout = ClusterDefaultTimeout
	}

	// Sleep 20 second to check cluster creating or failed
	sleep := math.Min(float64(timeout), float64(DefaultPreCheckSleepTime))
	time.Sleep(time.Duration(sleep) * time.Second)

	cluster, err := client.DescribeCluster(clusterId)
	if err != nil {
		return err
	} else if cluster.State == Failed {
		return fmt.Errorf("Waitting for cluster %s %s failed. Looking the specified reason in the web console.", clusterId, status)
	} else if cluster.State == status {
		//TODO
		return nil
	}

	// Create or Reset cluster usually cost at least 4 min, so there will sleep a long time before polling
	sleep = math.Min(float64(timeout), float64(DefaultPreSleepTime))
	time.Sleep(time.Duration(sleep) * time.Second)

	for {
		cluster, err := client.DescribeCluster(clusterId)
		if err != nil {
			return err
		} else if cluster.State == Failed {
			return fmt.Errorf("Waitting for cluster %s %s failed. Looking the specified reason in the web console.", clusterId, status)
		} else if cluster.State == status {
			//TODO
			break
		}
		timeout = timeout - DefaultWaitForInterval
		if timeout <= 0 {
			return common.GetClientErrorFromString("Timeout")
		}
		time.Sleep(DefaultWaitForInterval * time.Second)
	}
	return nil
}

func (client *Client) GetProjectClient(clusterId string) (projectClient *ProjectClient, err error) {
	cluster, err := client.DescribeCluster(clusterId)
	if err != nil {
		return
	}

	certs, err := client.GetClusterCerts(clusterId)
	if err != nil {
		return
	}

	projectClient, err = NewProjectClient(clusterId, cluster.MasterURL, certs)

	if err != nil {
		return
	}

	projectClient.SetDebug(client.debug)

	return
}
