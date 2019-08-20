package opensearch

import (
	"net/http"
)

//应用管理类API
const (
	status = "status"
)

//查看应用信息
func (this *Client) GetStatus(appName string, resp interface{}) error {
	return this.InvokeByAnyMethod(http.MethodGet, "", "/index/"+appName, OpenSearchArgs{Action: status}, resp)
}
