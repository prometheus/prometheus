package rds

import (
	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

type DescribeDBInstancePerformanceArgs struct {
	DBInstanceId string
	Key          string
	StartTime    string
	EndTime      string
}

type PerformanceValueType struct {
	Value string
	Date  util.ISO6801Time
}

type PerformanceKeyType struct {
	Key         string
	Unit        string
	ValueFormat string
	Values      struct {
		PerformanceValue []PerformanceValueType
	}
}

type DescribeDBInstancePerformanceResponse struct {
	common.Response
	DBInstanceId    string
	Engine          string
	StartTime       util.ISO6801Time
	EndTime         util.ISO6801Time
	PerformanceKeys struct {
		PerformanceKey []PerformanceKeyType
	}
}

func (client *DescribeDBInstancePerformanceArgs) Setkey(key string) {
	client.Key = key
}

func (client *Client) DescribeDBInstancePerformance(args *DescribeDBInstancePerformanceArgs) (resp DescribeDBInstancePerformanceResponse, err error) {

	response := DescribeDBInstancePerformanceResponse{}
	err = client.Invoke("DescribeDBInstancePerformance", args, &response)
	return response, err

}
