package cdn

import (
	"time"

	"github.com/denverdino/aliyungo/common"
)

type RefreshRequest struct {
	ObjectPath string
	// optional
	ObjectType string
}

type RefreshResponse struct {
	CdnCommonResponse
	RefreshTaskId string
}

type PushResponse struct {
	CdnCommonResponse
	PushTaskId string
}

type DescribeRequest struct {
	// optional
	TaskId     string
	ObjectPath string
	common.Pagination
}

type Task struct {
	TaskId       string
	ObjectPath   string
	Status       string
	Process      string
	CreationTime time.Time
	Description  string
}

type DescribeResponse struct {
	CdnCommonResponse
	common.PaginationResult
	Tasks struct {
		CDNTask []Task
	}
}

type QuotaResponse struct {
	CdnCommonResponse
	UrlQuota  string
	DirQuota  string
	UrlRemain string
	DirRemain string
}

func (client *CdnClient) RefreshObjectCaches(req RefreshRequest) (RefreshResponse, error) {
	var resp RefreshResponse
	err := client.Invoke("RefreshObjectCaches", req, &resp)
	if err != nil {
		return RefreshResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) PushObjectCache(req RefreshRequest) (PushResponse, error) {
	var resp PushResponse
	err := client.Invoke("PushObjectCache", req, &resp)
	if err != nil {
		return PushResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DescribeRefreshTasks(req DescribeRequest) (DescribeResponse, error) {
	var resp DescribeResponse
	err := client.Invoke("DescribeRefreshTasks", req, &resp)
	if err != nil {
		return DescribeResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DescribeRefreshQuota() (QuotaResponse, error) {
	var resp QuotaResponse
	err := client.Invoke("DescribeRefreshQuota", struct{}{}, &resp)
	if err != nil {
		return QuotaResponse{}, err
	}
	return resp, nil
}
