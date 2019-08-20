package pvtz

import (
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

type DescribeChangeLogsArgs struct {
	StartTime    int64
	EndTime      int64
	EntityType   string
	Keyword      string
	Lang         string
	UserClientIp string
	common.Pagination
}

//
type ChangeLogType struct {
	OperTimestamp int64
	OperAction    string
	OperObject    string
	EntityId      string
	EntityName    string
	OperIp        string
	OperTime      util.ISO6801Time
	Id            int64
	Content       string
	IsPtr         bool
	RecordCount   int
}

type DescribeChangeLogsResponse struct {
	common.Response
	common.PaginationResult
	ChangeLogs struct {
		ChangeLog []ChangeLogType
	}
}

// DescribeChangeLogs describes change logs
//
// You can read doc at https://help.aliyun.com/document_detail/66253.html
func (client *Client) DescribeChangeLogs(args *DescribeChangeLogsArgs) (logs []ChangeLogType, err error) {

	result := []ChangeLogType{}

	for {
		response := DescribeChangeLogsResponse{}
		err = client.Invoke("DescribeChangeLogs", args, &response)

		if err != nil {
			return result, err
		}

		result = append(result, response.ChangeLogs.ChangeLog...)

		nextPage := response.PaginationResult.NextPage()
		if nextPage == nil {
			break
		}
		args.Pagination = *nextPage
	}

	return result, nil
}
