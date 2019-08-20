package cms

import (
	"github.com/denverdino/aliyungo/common"
)

type CommonAlarmResponse struct {
	RequestId string
	Success   bool
	Code      string
	Message   string
}

type CommonAlarmItem struct {
	Name               string //报警规则名称
	Namespace          string //产品名称，参考各产品对应的project,例如acs_ecs_dashboard, acs_rds_dashboard等
	MetricName         string //相应产品对应的监控项名称，参考各产品metric定义
	Dimensions         string //报警规则对应实例列表，为json array对应的string，例如[{“instanceId”:”name1”},{“iinstance”:”name2”}]
	Period             int    //查询指标的周期，必须与定义的metric一致，默认300，单位为秒
	Statistics         string //统计方法，必须与定义的metric一致，例如Average
	ComparisonOperator string //报警比较符，只能为以下几种<=,<,>,>=,==,!=
	Threshold          string //报警阈值，目前只开放数值类型功能
	EvaluationCount    int    //连续探测几次都满足阈值条件时报警，默认3次
	ContactGroups      string //报警规则通知的联系组，必须在控制台上已创建，为json array对应的string，例如 [“联系组1”,”联系组2”]
	StartTime          int    //报警生效时间的开始时间，默认0，代表0点
	EndTime            int    //报警生效时间的结束时间，默认24，代表24点
	SilenceTime        int    //一直处于报警状态的通知沉默周期，默认86400，单位s，最小1小时
	NotifyType         int    //通知类型，为0是旺旺+邮件，为1是旺旺+邮件+短信
}

type AlarmItem struct {
	Id     string //报警规则id
	Enable bool   //该规则是否启用，true为启动
	State  string //报警规则的状态，有一个实例报警就是ALARM，所有都没数据是INSUFFICIENT_DATA，其它情况为OK

	CommonAlarmItem
}

type AlarmHistoryItem struct {
	Id              string //报警规则id
	Name            string //报警规则名称
	Namespace       string //产品名称，参考各产品对应的project,例如acs_ecs_dashboard, acs_rds_dashboard等
	MetricName      string //相应产品对应的监控项名称，参考各产品metric定义
	Dimension       string //报警规则对应实例列表，为json array对应的string，例如[{“instanceId”:”name1”},{“iinstance”:”name2”}]
	EvaluationCount int    //连续探测几次都满足阈值条件时报警，默认3次
	Value           string //报警的当前值
	AlarmTime       int64  //发生报警的时间
	LastTime        int64  //报警持续时间，单位为毫秒
	State           string //报警规则状态，有OK，ALARM，INSUFFICIENT_DATA三种状态
	Status          int    //通知发送状态，0为已通知用户，1为不在生效期未通知，2为处于报警沉默期未通知
	ContactGroups   string //发出的报警通知的通知对象，json array对应的字符串，例如[“联系组1”:”联系组2”]，只有通知状态为0才有该字段
}

type CreateAlarmArgs struct {
	CommonAlarmItem
}

type CreateAlarmResponse struct {
	CommonAlarmResponse
	Data string
}

//see doc at https://help.aliyun.com/document_detail/51910.html?spm=5176.doc51912.6.627.61vKOf
func (client *CMSClient) CreateAlarm(args *CreateAlarmArgs) (*CreateAlarmResponse, error) {
	response := &CreateAlarmResponse{}
	err := client.InvokeByAnyMethod(METHOD_POST, "CreateAlarm", "", args, response)
	if err != nil {
		return nil, err
	}
	return response, err
}

type DeleteAlarmArgs struct {
	Id string
}

type DeleteAlarmResponse struct {
	CommonAlarmResponse
	Data string
}

//see doc at https://help.aliyun.com/document_detail/51912.html?spm=5176.doc51915.6.628.ozEQet
func (client *CMSClient) DeleteAlarm(id string) error {
	args := &DeleteAlarmArgs{Id: id}
	response := &DeleteAlarmResponse{}
	err := client.Invoke("DeleteAlarm", args, response)

	return err
}

type ListAlarmArgs struct {
	Id         string //报警规则的id
	Name       string //报警规则名称，支持模糊查询
	Namespace  string //产品名称，参考各产品对应的project,例如 acs_ecs_dashboard, acs_rds_dashboard等
	Dimensions string //规则关联的实例信息，为json object对应的字符串，例如{“instacnce”:”name1”}。可以查询用于查询关联该实例的所有规则，应用该字段时必须指定Namespace
	State      string //报警规则状态, ALARM, INSUFFICIENT_DATA，OK
	IsEnable   bool   //true为启用，false为禁用
	common.Pagination
}

type ListAlarmResponse struct {
	CommonAlarmResponse
	NextToken int //下一页，为空代表没有下一页
	Total     int //符合条件数据总数
	AlarmList struct {
		Alarm []AlarmItem //报警规则详情列表
	}
}

//see doc at https://help.aliyun.com/document_detail/51915.html?spm=5176.doc51914.6.629.Yz4ZjF
func (client *CMSClient) ListAlarm(args *ListAlarmArgs) (*ListAlarmResponse, error) {
	response := &ListAlarmResponse{}
	err := client.Invoke("ListAlarm", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type DisableAlarmArgs struct {
	Id string
}

type DisableAlarmResponse struct {
	CommonAlarmResponse
}

//see doc at https://help.aliyun.com/document_detail/51914.html?spm=5176.doc51915.6.630.YUybwN
func (client *CMSClient) DisableAlarm(id string) error {
	args := &DisableAlarmArgs{Id: id}
	response := &DisableAlarmResponse{}
	err := client.Invoke("DisableAlarm", args, response)

	return err
}

type EnableAlarmArgss struct {
	Id string
}

type EnableAlarmResponse struct {
	CommonAlarmResponse
}

//see doc at https://help.aliyun.com/document_detail/51913.html?spm=5176.doc51910.6.631.3zgHyK
func (client *CMSClient) EnableAlarm(id string) error {
	args := &EnableAlarmArgss{Id: id}
	response := &EnableAlarmResponse{}
	err := client.Invoke("EnableAlarm", args, response)

	return err
}

type UpdateAlarmArgs struct {
	Id                 string //报警规则的id
	Name               string //报警规则名称
	Period             int    //查询指标的周期，必须与定义的metric一致，默认300，单位为秒
	Statistics         string //统计方法，必须与定义的metric一致，例如Average
	ComparisonOperator string //报警比较符，只能为以下几种<=,<,>,>=,==,!=
	Threshold          string //报警阈值，目前只开放数值类型功能
	EvaluationCount    int    //连续探测几次都满足阈值条件时报警，默认3次
	ContactGroups      string //报警规则通知的联系组，必须在控制台上已创建，为json array对应的string，例如 [“联系组1”,”联系组2”]
	StartTime          int    //报警生效时间的开始时间，默认0，代表0点
	EndTime            int    //报警生效时间的结束时间，默认24，代表24点
	SilenceTime        int    //一直处于报警状态的通知沉默周期，默认86400，单位s，最小1小时
	NotifyType         int    //通知类型，为0是旺旺+邮件，为1是旺旺+邮件+短信
}

type UpdateAlarmResponse struct {
	CommonAlarmResponse
}

//see doc at https://help.aliyun.com/document_detail/51911.html?spm=5176.doc51913.6.632.IgBO9g
func (client *CMSClient) UpdateAlarm(args *UpdateAlarmArgs) error {
	response := &UpdateAlarmResponse{}
	err := client.Invoke("UpdateAlarm", args, response)

	return err
}

type ListAlarmHistoryArgs struct {
	Id        string //报警规则的id
	Size      int    //每页记录数，默认值：100
	StartTime string //查询数据开始时间，默认24小时前，可以输入long型时间，也可以输入yyyy-MM-dd HH:mm:ss类型时间
	EndTime   string //查询数据结束时间，默认当前时间，可以输入long型时间，也可以输入yyyy-MM-dd HH:mm:ss类型时间
	Cursor    string //查询数据的起始位置，为空则按时间查询前100条
}

type ListAlarmHistoryResponse struct {
	CommonAlarmResponse
	Cursor           string //查询数据的起始位置，为空则按时间查询前100条
	AlarmHistoryList struct {
		AlarmHistory []AlarmHistoryItem //报警历史详情列表
	}
}

//see doc at https://help.aliyun.com/document_detail/51916.html?spm=5176.doc51911.6.633.KPOcwc
func (client *CMSClient) ListAlarmHistory(args *ListAlarmHistoryArgs) (*ListAlarmHistoryResponse, error) {
	response := &ListAlarmHistoryResponse{}
	err := client.Invoke("ListAlarmHistory", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

type ListContactGroupArgs struct {
	common.Pagination
}

type ListContactGroupResponse struct {
	CommonAlarmResponse
	NextToken  string   //下一页，为空代表没有下一页
	Datapoints []string //联系组名称列表
	Total      int      //符合条件数据总数
}

//see doc at https://help.aliyun.com/document_detail/52315.html?spm=5176.doc51916.6.634.TDR72G
func (client *CMSClient) ListContactGroup(args *ListContactGroupArgs) (*ListContactGroupResponse, error) {
	response := &ListContactGroupResponse{}
	err := client.Invoke("ListContactGroup", args, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}
