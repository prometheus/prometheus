package cms

import (
	"encoding/json"
	//	"errors"
	//	"fmt"
	//	"io/ioutil"
	//	"net/http"
	"net/url"
	//	"strings"
)

// 调用接口返回的实体
// 新增加的结构首字母一定要大写，否则json编码或者解析编码会出现问题
type ResultModel struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Success bool   `json:"success"`
}

//project的实体类，主要用于发送请求,更新或者添加Project
type ProjectModel struct {
	ProjectName  string `json:"projectName"`
	ProjectDesc  string `json:"projectDesc"`
	ProjectOwner string `json:"projectOwner"`
}

//project返回实体,继承于ProjectModel
type ProjectResultModel struct {
	ProjectModel
	Id          int64  `json:"id"`
	GmtModified int64  `json:"gmtModified"`
	GmtCreate   int64  `json:"gmtCreate"`
	Status      int    `json:"status"`
	Creator     string `json:"creator"`
}

//取得多个project的结构
type ProjectListResultModel struct {
	Total      int                  `json:"total"`
	Datapoints []ProjectResultModel `json:"datapoints"`
}

//返回的实体类型
type GetProjectResult struct {
	Result  ProjectResultModel `json:"result"`
	Code    string             `json:"code"`
	Success bool               `json:"success"`
}

//列表批量取得Project的请求参数
type ListProjectRequestModel struct {
	Page     string `json:"page"`
	PageSize string `json:"pageSize"`
}

/**
 * 创建一个Project
 * @params
 *   ProjectModel 创建模块的模型
 * @return
 *   result 创建返回的模型
 *   error  错误，如果一切Ok，error为 nil
 */
func (c *Client) CreateProject(projectName string, projectDesc string, projectOwner string) (result ResultModel, err error) {

	project := &ProjectModel{
		projectName,
		projectDesc,
		projectOwner,
	}

	postObjContent, _ := json.Marshal(project)
	postJson := string(postObjContent)

	requestUrl := c.GetUrl("projects", "", "")
	requestPath := GetRequestPath("projects", "", "")

	responseResult, err := c.GetResponseJson("POST", requestUrl, requestPath, postJson)

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

/**
 * 删除一个Project
 * @params
 *   projectName 创建模块的模型
 * @return
 *   result 返回的模型
 *   error  错误，如果一切Ok，error为 nil
 */
func (c *Client) DeleteProject(projectName string) (result ResultModel, err error) {

	requestUrl := c.GetUrl("projects", "", projectName)
	requestPath := GetRequestPath("projects", "", projectName)

	responseResult, err := c.GetResponseJson("DELETE", requestUrl, requestPath, "")

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err

}

/**
 * 更新一个Project
 * @params
 *   projectName 创建模块的模型
 *    project 修改的模型，这个只能修改projectDesc和ProjectOwer
 * @return
 *   result 返回的模型
 *   error  错误，如果一切Ok，error为 nil
 */
func (c *Client) UpdateProject(projectName string, project *ProjectModel) (result ResultModel, err error) {

	postObjContent, _ := json.Marshal(project)
	postJson := string(postObjContent)

	requestUrl := c.GetUrl("projects", "", projectName)
	requestPath := GetRequestPath("projects", "", projectName)

	responseResult, err := c.GetResponseJson("PUT", requestUrl, requestPath, postJson)

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

/**
 * 取得一个Project
 * @params
 *   projectName projectName不能为空
 * @return
 *   result 返回的模型
 *   error  错误，如果一切Ok，error为 nil
 */
func (c *Client) GetProject(projectName string) (result GetProjectResult, err error) {

	requestUrl := c.GetUrl("projects", "", projectName)
	requestPath := GetRequestPath("projects", "", projectName)

	responseResult, err := c.GetResponseJson("GET", requestUrl, requestPath, "")

	if err != nil {
		return result, err
	}

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}

/**
 * 取得一个多个Project
 * @params
 *   page: 页码
 *   pageSize: 每夜记录条数
 *   projectOwner 所属用户
 * @return
 *   result 返回的模型
 *   error  错误，如果一切Ok，error为 nil
 */
func (c *Client) GetProjectList(page string, pageSize string, projectOwner string) (result ProjectListResultModel, err error) {

	params := url.Values{}
	params.Add("Page", page)
	params.Add("PageSize", pageSize)

	if projectOwner != "" {
		params.Add("ProjectOwner", projectOwner)
	}

	paramsString := params.Encode()

	requestUrl := c.GetUrl("projects", "", "")
	requestPath := GetRequestPath("projects", "", "")

	responseResult, _ := c.GetResponseJson("GET", requestUrl+"?"+paramsString+"&resource", requestPath+"?"+paramsString+"&resource", "")

	err = json.Unmarshal([]byte(responseResult), &result)

	return result, err
}
