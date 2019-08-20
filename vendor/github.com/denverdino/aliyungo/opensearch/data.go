package opensearch

import "net/http"

type PushArgs struct {
	OpenSearchArgs        // 这个参数不用填
	Table_name     string `ArgName:"table_name"` //要上传数据的表名
	Items          string `ArgName:"items"`      //规定JSON格式
}

//上传文档
//支持新增、更新、删除的批量操作
func (this *Client) Push(appName string, args PushArgs, resp interface{}) error {
	args.OpenSearchArgs.Action = "push"
	return this.InvokeByAnyMethod(http.MethodPost, "", "/index/doc/"+appName, args, resp)
}
