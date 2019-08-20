package ess

import "github.com/denverdino/aliyungo/common"

type RecurrenceType string

const (
	Daily   = RecurrenceType("Daily")
	Weekly  = RecurrenceType("Weekly")
	Monthly = RecurrenceType("Monthly")
)

type CreateScheduledTaskArgs struct {
	RegionId             common.Region
	ScheduledAction      string
	LaunchTime           string
	ScheduledTaskName    string
	Description          string
	LaunchExpirationTime int
	RecurrenceType       RecurrenceType
	RecurrenceValue      string
	RecurrenceEndTime    string
	TaskEnabled          bool
}

type CreateScheduledTaskResponse struct {
	common.Response
	ScheduledTaskId string
}

// CreateScheduledTask create schedule task
//
// You can read doc at https://help.aliyun.com/document_detail/25957.html?spm=5176.doc25950.6.638.FfQ0BR
func (client *Client) CreateScheduledTask(args *CreateScheduledTaskArgs) (resp *CreateScheduledTaskResponse, err error) {
	response := CreateScheduledTaskResponse{}
	err = client.Invoke("CreateScheduledTask", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type ModifyScheduledTaskArgs struct {
	RegionId             common.Region
	ScheduledTaskId      string
	ScheduledAction      string
	LaunchTime           string
	ScheduledTaskName    string
	Description          string
	LaunchExpirationTime int
	RecurrenceType       RecurrenceType
	RecurrenceValue      string
	RecurrenceEndTime    string
	TaskEnabled          bool
}

type ModifyScheduledTaskResponse struct {
	common.Response
}

// ModifyScheduledTask modify schedule task
//
// You can read doc at https://help.aliyun.com/document_detail/25958.html?spm=5176.doc25957.6.639.rgxQ1c
func (client *Client) ModifyScheduledTask(args *ModifyScheduledTaskArgs) (resp *ModifyScheduledTaskResponse, err error) {
	response := ModifyScheduledTaskResponse{}
	err = client.Invoke("ModifyScheduledTask", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}

type DescribeScheduledTasksArgs struct {
	RegionId          common.Region
	ScheduledTaskId   common.FlattenArray
	ScheduledTaskName common.FlattenArray
	ScheduledAction   common.FlattenArray
	common.Pagination
}

type DescribeScheduledTasksResponse struct {
	common.Response
	common.PaginationResult
	ScheduledTasks struct {
		ScheduledTask []ScheduledTaskItemType
	}
}

type ScheduledTaskItemType struct {
	ScheduledTaskId      string
	ScheduledTaskName    string
	Description          string
	ScheduledAction      string
	LaunchTime           string
	RecurrenceType       string
	RecurrenceValue      string
	RecurrenceEndTime    string
	LaunchExpirationTime int
	TaskEnabled          bool
}

// DescribeScheduledTasks describes scaling tasks
//
// You can read doc at https://help.aliyun.com/document_detail/25959.html?spm=5176.doc25958.6.640.cLccdR
func (client *Client) DescribeScheduledTasks(args *DescribeScheduledTasksArgs) (tasks []ScheduledTaskItemType, pagination *common.PaginationResult, err error) {
	args.Validate()
	response := DescribeScheduledTasksResponse{}

	err = client.InvokeByFlattenMethod("DescribeScheduledTasks", args, &response)

	if err == nil {
		return response.ScheduledTasks.ScheduledTask, &response.PaginationResult, nil
	}

	return nil, nil, err
}

type DeleteScheduledTaskArgs struct {
	RegionId        common.Region
	ScheduledTaskId string
}

type DeleteScheduledTaskResponse struct {
	common.Response
}

// DeleteScheduledTask delete schedule task
//
// You can read doc at https://help.aliyun.com/document_detail/25960.html?spm=5176.doc25959.6.641.aGdNuW
func (client *Client) DeleteScheduledTask(args *DeleteScheduledTaskArgs) (resp *DeleteScheduledTaskResponse, err error) {
	response := DeleteScheduledTaskResponse{}
	err = client.Invoke("DeleteScheduledTask", args, &response)

	if err != nil {
		return nil, err
	}
	return &response, nil
}
