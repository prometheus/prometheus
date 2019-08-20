package cms

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

//alert请求结构
type AlertRequest struct {
	Name        string             `json:"name"`
	Status      int                `json:"status"`
	Actions     ActionsModel       `json:"actions"`
	Condition   ConditionModel     `json:"condition"`
	Enable      bool               `json:"enable"`
	Escalations []EscalationsModel `json:"escalations"`
}

type ActionsModel struct {
	AlertActions []AlertActionsModel `json:"alertActions"`
	Effective    string              `json:"effective"`
	Failure      ActionGroup         `json:"failure"`
}

type Failure struct {
	ContactGroups []string `json:"contactGroups"`
	Id            string   `json:"id"`
}

type AlertActionsModel struct {
	ContactGroups []string `json:"contactGroups"`
	Id            string   `json:"id"`
	Level         int      `json:"level"`
}

type ConditionModel struct {
	DimensionKeys    []string `json:"dimensionKeys"`
	Interval         int      `json:"interval"`
	MetricColumnName string   `json:"metricColumnName"`
	MetricName       string   `json:"metricName"`
	SourceType       string   `json:"sourceType"`
}

type EscalationsModel struct {
	Expression string `json:"expression"`
	Level      int    `json:"level"`
	Times      int    `json:"times"`
}

type ActionGroup struct {
	ContactGroups []string `json:"contactGroups"`
	Id            string   `json:"id"`
}

type DimensionRequest struct {
	//ProjectName string `json:"projectName"`
	AlertName  string `json:"alertName"`
	Dimensions string `json:"dimensions"`
	UserId     string `json:"userId"`
}

type DimensionDataPoint struct {
	Id         string `json:"id"`
	Project    string `json:"project,omitempty"`
	AlertUuid  string `json:"alertUuid,omitempty"`
	Status     int    `json:"status,omitempty"`
	Dimensions string `json:"dimensions,omitempty"`
	AlertName  string `json:"alertName,omitempty"`
}
type GetDimenstionResult struct {
	Code       string                `json:"code"`
	Success    bool                  `json:"success"`
	Message    string                `json:"comessagede,omitempty"`
	TraceId    string                `json:"traceId"`
	DataPoints []*DimensionDataPoint `json:"datapoints,omitempty"`
}

const (
	entity = "alerts"
)

/**
 * 创建一个Alert
 * @params
 *   projectName
 *   AlertRequest 创建模块的模型
 * @return
 *   result 创建返回的模型
 *   error  错误，如果一切Ok，error为 nil
 */
func (c *Client) CreateAlert(projectName string, alertRequest AlertRequest) (result ResultModel, err error) {
	postObjContent, _ := json.Marshal(alertRequest)
	postJson := string(postObjContent)

	requestUrl := c.GetUrl(entity, projectName, "")
	requestPath := GetRequestPath(entity, projectName, "")

	responseResult, err := c.GetResponseJson("POST", requestUrl, requestPath, postJson)

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

func (c *Client) CreateAlert4Json(projectName string, request string) (result ResultModel, err error) {

	requestUrl := c.GetUrl(entity, projectName, "")
	requestPath := GetRequestPath(entity, projectName, "")

	responseResult, err := c.GetResponseJson("POST", requestUrl, requestPath, request)

	if err != nil {
		return result, err
	}

	fmt.Printf("response: %s \n", responseResult)
	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

func (c *Client) PutMetrics(projectName string, request string) (result ResultModel, err error) {

	requestUrl := c.GetUrl(entity, projectName, "")
	requestPath := GetRequestPath(entity, projectName, "")

	responseResult, err := c.GetResponseJson("POST", requestUrl, requestPath, request)

	if err != nil {
		return result, err
	}

	fmt.Printf("response: %s \n", responseResult)
	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

/**
 * 更新Alert
 */
func (c *Client) UpdateAlert(projectName string, alertName string, alertRequest AlertRequest) (result ResultModel, err error) {

	postObjContent, _ := json.Marshal(alertRequest)
	postJson := string(postObjContent)

	requestUrl := c.GetUrl(entity, projectName, alertName)
	requestPath := GetRequestPath(entity, projectName, alertName)

	responseResult, err := c.GetResponseJson("PUT", requestUrl, requestPath, postJson)

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

/**
 *
 */
func (c *Client) DeleteAlert(projectName string, alertName string) (result ResultModel, err error) {

	requestUrl := c.GetUrl("alert", projectName, "?alertName="+alertName)
	requestPath := GetRequestPath("alert", projectName, "?alertName="+alertName)

	responseResult, err := c.GetResponseJson("DELETE", requestUrl, requestPath, "")

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err

}

/*
 * 取得一个alert
 */
func (c *Client) GetAlert(projectName string, alertName string) (result GetProjectResult, err error) {

	requestUrl := c.GetUrl(entity, projectName, "")
	requestPath := GetRequestPath(entity, projectName, "")
	requestUrl = requestUrl + "?alertName=" + alertName
	requestPath = requestPath + "?alertName=" + alertName

	fmt.Printf("url: %s \n", requestUrl)
	responseResult, err := c.GetResponseJson("GET", requestUrl, requestPath, "")

	if err != nil {
		return result, err
	}

	fmt.Printf("GetAlert response: %s \n", responseResult)
	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

/**
 * 取得alertl列表
 */
func (c *Client) GetAlertList(page string, pageSize string, projectName string, alertName string) (result GetProjectResult, err error) {

	params := url.Values{}
	params.Add("Page", page)
	params.Add("PageSize", pageSize)

	requestUrl := c.GetUrl(entity, projectName, alertName)
	//requestUrl := "http://alert.aliyuncs.com/projects/acs_custom_1047840662545713/alerts?alertName=test_alert_dimensions"
	requestPath := GetRequestPath(entity, projectName, alertName)

	paramsString := params.Encode()

	responseResult, err := c.GetResponseJson("GET", requestUrl+"?"+paramsString+"&resource", requestPath+"?"+paramsString+"&resource", "")

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

func (c *Client) CreateAlertDimension(projectName string, request DimensionRequest) (result ResultModel, err error) {
	postObjContent, _ := json.Marshal(request)
	postJson := string(postObjContent)

	// ?alertName=test_alert_dimensions_2
	requestUrl := c.GetUrl("alert", projectName, "dimensions?alertName="+request.AlertName)
	requestPath := GetRequestPath("alert", projectName, "dimensions?alertName="+request.AlertName)

	fmt.Printf("requestUrl: %s \n", requestUrl)
	responseResult, err := c.GetResponseJson("POST", requestUrl, requestPath, postJson)

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err

}

func (c *Client) GetDimensions(projectName string, alertName string) (result GetDimenstionResult, err error) {
	//  this.setUriPattern("/projects/[ProjectName]/alert/dimensions");

	requestUrl := c.GetUrl("alert", projectName, "dimensions?alertName="+alertName)
	requestPath := GetRequestPath("alert", projectName, "dimensions?alertName="+alertName)

	fmt.Printf("requestUrl: %s \n", requestUrl)
	responseResult, err := c.GetResponseJson(http.MethodGet, requestUrl, requestPath, "")

	if err != nil {
		return result, err
	}

	fmt.Printf("response: %s \n", responseResult)
	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

func (c *Client) DeleteAlertDimension(projectName string, alertName string, dimensionId string) (result ResultModel, err error) {

	requestUrl := c.GetUrl("alert", projectName, "dimensions/"+dimensionId+"?alertName="+alertName)
	requestPath := GetRequestPath("alert", projectName, "dimensions/"+dimensionId+"?alertName="+alertName)

	fmt.Printf("the requesturl %s", requestUrl)
	responseResult, err := c.GetResponseJson(http.MethodDelete, requestUrl, requestPath, "")

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err

}
