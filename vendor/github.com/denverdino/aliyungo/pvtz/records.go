package pvtz

import (
	"log"

	"github.com/denverdino/aliyungo/common"
)

type RecordStatus string

const EnableStatus = RecordStatus("ENABLE")
const DisableStatus = RecordStatus("DISABLE")

type DescribeZoneRecordsArgs struct {
	ZoneId       string
	Keyword      string
	Lang         string
	UserClientIp string
	common.Pagination
}

//
type ZoneRecordType struct {
	RecordId int64
	Rr       string
	Type     string
	Ttl      int
	Priority int
	Value    string
	Status   RecordStatus
}

type DescribeZoneRecordsResponse struct {
	common.Response
	common.PaginationResult
	Records struct {
		Record []ZoneRecordType
	}
}

// DescribeZoneRecords describes zones
//
// You can read doc at https://help.aliyun.com/document_detail/66252.html
func (client *Client) DescribeZoneRecords(args *DescribeZoneRecordsArgs) (records []ZoneRecordType, err error) {

	result := []ZoneRecordType{}

	for {
		response := DescribeZoneRecordsResponse{}
		err = client.Invoke("DescribeZoneRecords", args, &response)

		if err != nil {
			return result, err
		}

		result = append(result, response.Records.Record...)

		nextPage := response.PaginationResult.NextPage()
		if nextPage == nil {
			break
		}
		args.Pagination = *nextPage
	}

	return result, nil
}

func (client *Client) DescribeZoneRecordsByRR(zoneId string, rr string) (records []ZoneRecordType, err error) {
	records, err = client.DescribeZoneRecords(&DescribeZoneRecordsArgs{
		ZoneId:  zoneId,
		Keyword: rr,
	})

	if err != nil {
		return records, err
	}

	result := make([]ZoneRecordType, 0, 0)
	for _, record := range records {
		if record.Rr == rr {
			result = append(result, record)
		}
	}
	return result, err
}

func (client *Client) DeleteZoneRecordsByRR(zoneId string, rr string) error {
	records, err := client.DescribeZoneRecordsByRR(zoneId, rr)

	if err != nil {
		return err
	}

	for _, record := range records {
		if record.Rr == rr {
			err := client.DeleteZoneRecord(&DeleteZoneRecordArgs{
				RecordId: record.RecordId,
			})
			if err != nil {
				log.Printf("failed to delete zone record %d: %v\n", record.RecordId, err)
			}
		}
	}
	return nil
}

type AddZoneRecordArgs struct {
	ZoneName     string
	Rr           string
	Type         string
	Value        string
	ZoneId       string
	Lang         string
	Priority     int
	Ttl          int
	UserClientIp string
}

type AddZoneRecordResponse struct {
	common.Response
	Success  bool
	RecordId int64
}

// AddZoneRecord add zone record
//
// You can read doc at https://help.aliyun.com/document_detail/66248.html
func (client *Client) AddZoneRecord(args *AddZoneRecordArgs) (response *AddZoneRecordResponse, err error) {
	response = &AddZoneRecordResponse{}

	err = client.Invoke("AddZoneRecord", args, &response)

	return response, err
}

type UpdateZoneRecordArgs struct {
	RecordId     int64
	Rr           string
	Type         string
	Value        string
	Lang         string
	Priority     int
	Ttl          int
	UserClientIp string
}

type UpdateZoneRecordResponse struct {
	common.Response
	RecordId int64
}

// UpdateZoneRecord update zone record
//
// You can read doc at https://help.aliyun.com/document_detail/66250.html
func (client *Client) UpdateZoneRecord(args *AddZoneRecordArgs) (err error) {
	response := &UpdateZoneRecordResponse{}

	err = client.Invoke("UpdateZoneRecord", args, &response)

	return err
}

type DeleteZoneRecordArgs struct {
	RecordId     int64
	Lang         string
	UserClientIp string
}

type DeleteZoneRecordResponse struct {
	common.Response
	RecordId int64
}

// DeleteZone delete zone
//
// You can read doc at https://help.aliyun.com/document_detail/66249.html
func (client *Client) DeleteZoneRecord(args *DeleteZoneRecordArgs) (err error) {
	response := &DeleteZoneRecordResponse{}
	err = client.Invoke("DeleteZoneRecord", args, &response)

	return err
}

type SetZoneRecordStatusArgs struct {
	RecordId     int64
	Lang         string
	UserClientIp string
	Status       RecordStatus
}

type SetZoneRecordStatusResponse struct {
	common.Response
	RecordId string
	Status   RecordStatus
}

// SetZoneRecordStatus set zone record status
//
// You can read doc at https://help.aliyun.com/document_detail/66251.html
func (client *Client) SetZoneRecordStatus(args *SetZoneRecordStatusArgs) (err error) {
	response := &SetZoneRecordStatusResponse{}
	err = client.Invoke("SetZoneRecordStatus", args, &response)

	return err
}
