package opensearch

import "net/http"

type SearchArgs struct {
	//搜索主体
	Query string `ArgName:"query"`
	//要查询的应用名
	Index_name string `ArgName:"index_name"`
	//[可以通过此参数获取本次查询需要的字段内容]
	Fetch_fields string `ArgName:"fetch_fields"`
	//[指定要使用的查询分析规则]
	Qp string `ArgName:"qp"`
	//[关闭已生效的查询分析功能]
	Disable string `ArgName:"disable"`
	//[设置粗排表达式名字]
	First_formula_name string `ArgName:"first_formula_name"`
	//[设置精排表达式名字]
	Formula_name string `ArgName:"formula_name"`
	//[动态摘要的配置]
	Summary string `ArgName:"summary"`
}

//搜索
//系统提供了丰富的搜索语法以满足用户各种场景下的搜索需求
func (this *Client) Search(args SearchArgs, resp interface{}) error {
	return this.InvokeByAnyMethod(http.MethodGet, "", "/search", args, resp)
}
