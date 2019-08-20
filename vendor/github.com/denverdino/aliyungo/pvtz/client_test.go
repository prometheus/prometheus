package pvtz

import (
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestDescribeRegions(t *testing.T) {
	client := NewTestClient()

	regions, err := client.DescribeRegions()

	t.Logf("regions: %v, %v", regions, err)
}

func TestAddZone(t *testing.T) {
	client := NewTestClient()

	checkResult, err := client.CheckZoneName(&CheckZoneNameArgs{
		ZoneName: "demo.com",
	})

	t.Logf("CheckZoneName: %v, %v", checkResult, err)

	response, err := client.AddZone(&AddZoneArgs{
		ZoneName: "demo.com",
	})

	t.Logf("AddZone: %++v, %v", response, err)
	testDescribeZones(t)

	zoneId := response.ZoneId

	err = client.UpdateZoneRemark(&UpdateZoneRemarkArgs{
		ZoneId: zoneId,
		Remark: "specialZone",
	})
	t.Logf("UpdateZoneRemark: %v", err)

	testDescribeZoneRecords(t, zoneId)

	err = client.BindZoneVpc(&BindZoneVpcArgs{
		ZoneId: zoneId,
		Vpcs: []VPCType{
			VPCType{
				RegionId: common.Beijing,
				VpcId:    TestVPCId,
			},
		},
	})
	t.Logf("BindZoneVpc:  %v", err)

	zoneInfo, err := client.DescribeZoneInfo(&DescribeZoneInfoArgs{
		ZoneId: zoneId,
	})

	t.Logf("DescribeZoneInfo:  %v %v", zoneInfo, err)

	err = client.BindZoneVpc(&BindZoneVpcArgs{
		ZoneId: zoneId,
		Vpcs:   []VPCType{},
	})
	t.Logf("unbind ZoneVpc:  %v", err)

	testDeleteZone(t, zoneId)

}

func testDeleteZone(t *testing.T, zoneId string) {
	client := NewTestClient()

	err := client.DeleteZone(&DeleteZoneArgs{
		ZoneId: zoneId,
	})
	t.Logf("DeleteZone: %v", err)
}

func testDescribeZones(t *testing.T) {
	client := NewTestClient()

	zones, err := client.DescribeZones(&DescribeZonesArgs{})

	t.Logf("zones: %v, %v", zones, err)
}

func testDescribeZoneRecords(t *testing.T, zoneId string) {
	client := NewTestClient()

	response, err := client.AddZoneRecord(&AddZoneRecordArgs{
		ZoneId: zoneId,
		Rr:     "www",
		Type:   "A",
		Ttl:    60,
		Value:  "1.1.1.1",
	})

	t.Logf("AddZoneRecord: %v, %v", response, err)

	if err != nil {
		return
	}

	recordId := response.RecordId
	records, err := client.DescribeZoneRecords(&DescribeZoneRecordsArgs{
		ZoneId: zoneId,
	})

	t.Logf("records: %v, %v", records, err)

	err = client.DeleteZoneRecord(&DeleteZoneRecordArgs{
		RecordId: recordId,
	})
	t.Logf("DeleteZoneRecord: %v", err)
}

func TestDescribeChangeLogs(t *testing.T) {
	client := NewTestClient()

	changeLogs, err := client.DescribeChangeLogs(&DescribeChangeLogsArgs{})

	t.Logf("Change logs: %v, %v", changeLogs, err)
}
