package cdn

import "time"

type ServiceRequest struct {
	InternetChargeType string
}

type Service struct {
	InternetChargeType string
	OpeningTime        string
	ChangingChargeType string
	ChangingAffectTime time.Time
	OperationLocks     struct {
		LockReason []string
	}
}

type ServiceResponse struct {
	CdnCommonResponse
	Service
}

func (client *CdnClient) OpenCdnService(req ServiceRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("OpenCdnService", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) DescribeCdnService() (ServiceResponse, error) {
	var resp ServiceResponse
	err := client.Invoke("DescribeCdnService", struct{}{}, &resp)
	if err != nil {
		return ServiceResponse{}, err
	}
	return resp, nil
}

func (client *CdnClient) ModifyCdnService(req ServiceRequest) (CdnCommonResponse, error) {
	var resp CdnCommonResponse
	err := client.Invoke("ModifyCdnService", req, &resp)
	if err != nil {
		return CdnCommonResponse{}, err
	}
	return resp, nil
}
