package rds

import (
	"strconv"
	"strings"
	"testing"

	"github.com/denverdino/aliyungo/common"
)

func TestDBPostpaidClassicInstanceCreationAndDeletion(t *testing.T) {

	if TestIAmRich == false {
		// Avoid payment
		return
	}

	client := NewTestClient()

	args := CreateOrderArgs{
		RegionId:          TestRegionID,
		CommodityCode:     Bards,
		Engine:            MySQL,
		EngineVersion:     EngineVersion,
		DBInstanceClass:   DBInstanceClass,
		DBInstanceStorage: 10,
		Quantity:          1,
		PayType:           Postpaid,
		DBInstanceNetType: common.Intranet,
		Resource:          DefaultResource,
	}

	resp, err := client.CreateOrder(&args)
	if err != nil {
		t.Errorf("Failed to create db instance %v", err)
	}
	instanceId := resp.DBInstanceId
	t.Logf("Instance %s is created successfully.", instanceId)

	arrtArgs := DescribeDBInstanceAttributeArgs{
		DBInstanceId: instanceId,
	}
	attrResp, err := client.DescribeDBInstanceAttribute(&arrtArgs)
	t.Logf("Instance: %++v  %v", attrResp, err)

	err = client.WaitForInstance(instanceId, Running, 500)
	if err != nil {
		t.Errorf("Failed to create instance %s: %v", instanceId, err)
	}

	err = client.DeleteInstance(instanceId)

	if err != nil {
		t.Errorf("Failed to delete instance %s: %v", instanceId, err)
	}
	t.Logf("Instance %s is deleted successfully.", instanceId)
}

func TestDBPrepaidInstanceCreation(t *testing.T) {

	if TestIAmRich == false {
		// Avoid payment
		return
	}

	client := NewTestClient()

	args := CreateOrderArgs{
		RegionId:          TestRegionID,
		CommodityCode:     Rds,
		Engine:            MySQL,
		EngineVersion:     EngineVersion,
		DBInstanceClass:   DBInstanceClass,
		DBInstanceStorage: 10,
		Quantity:          1,
		PayType:           Prepaid,
		DBInstanceNetType: common.Intranet,
		Resource:          DefaultResource,
		TimeType:          common.Month,
		UsedTime:          1,
		AutoPay:           strconv.FormatBool(false),
	}

	resp, err := client.CreateOrder(&args)
	if err != nil {
		t.Errorf("Failed to create db instance %v", err)
	}

	instanceId := resp.DBInstanceId
	t.Logf("Instance %s is created successfully.", instanceId)

	arrtArgs := DescribeDBInstanceAttributeArgs{
		DBInstanceId: instanceId,
	}
	attrResp, err := client.DescribeDBInstanceAttribute(&arrtArgs)
	t.Logf("Instance: %++v  %v", attrResp, err)

	err = client.WaitForInstance(instanceId, Running, 500)
	if err != nil {
		t.Errorf("Failed to create instance %s: %v", instanceId, err)
	}
	t.Logf("Instance %s is running successfully.", instanceId)
}

func TestDBPostpaidVpcInstanceCreationAndDeletion(t *testing.T) {

	if TestIAmRich == false {
		// Avoid payment
		return
	}

	client := NewTestClient()

	args := CreateOrderArgs{
		RegionId:            TestRegionID,
		ZoneId:              ZoneId,
		CommodityCode:       Bards,
		Engine:              MySQL,
		EngineVersion:       EngineVersion,
		DBInstanceClass:     DBInstanceClass,
		DBInstanceStorage:   10,
		Quantity:            1,
		PayType:             Postpaid,
		DBInstanceNetType:   common.Intranet,
		InstanceNetworkType: common.VPC,
		VPCId:               VPCId,
		VSwitchId:           VSwitchId,
		SecurityIPList:      "127.0.0.1",
		Resource:            DefaultResource,
	}

	resp, err := client.CreateOrder(&args)
	if err != nil {
		t.Errorf("Failed to create db instance %v", err)
	}
	instanceId := resp.DBInstanceId
	t.Logf("Instance %s is created successfully.", instanceId)

	arrtArgs := DescribeDBInstanceAttributeArgs{
		DBInstanceId: instanceId,
	}
	attrResp, err := client.DescribeDBInstanceAttribute(&arrtArgs)
	t.Logf("Instance: %++v  %v", attrResp, err)

	err = client.WaitForInstance(instanceId, Running, 600)
	if err != nil {
		t.Errorf("Failed to create instance %s: %v", instanceId, err)
	}

	err = client.DeleteInstance(instanceId)

	if err != nil {
		t.Errorf("Failed to delete instance %s: %v", instanceId, err)
	}
	t.Logf("Instance %s is deleted successfully.", instanceId)
}

func TestGetZonesByRegionId(t *testing.T) {

	client := NewTestClient()
	resp, err := client.DescribeRegions()
	if err != nil {
		t.Errorf("Failed to describe rds regions %v", err)
	}

	regions := resp.Regions.RDSRegion
	t.Logf("all regions %++v.", regions)

	zoneIds := []string{}
	for _, r := range regions {
		if strings.Contains(r.RegionId, string(TestRegionID)) {
			zoneIds = append(zoneIds, r.ZoneId)
		}
	}
	t.Logf("all zones %++v of current region.", zoneIds)
}

func TestDatabaseCreationAndDeletion(t *testing.T) {
	client := NewTestClient()

	args := CreateDatabaseArgs{
		DBInstanceId:     DBInstanceId,
		DBName:           DBName,
		CharacterSetName: "utf8",
		DBDescription:    "test",
	}

	_, err := client.CreateDatabase(&args)
	if err != nil {
		t.Errorf("Failed to create db instance %v", err)
	}
	t.Logf("Database %s is created successfully.", DBName)

	q := [1]string{DBName}
	err = client.WaitForAllDatabase(DBInstanceId, q[:], Running, 600)
	if err != nil {
		t.Errorf("Failed to create database %s: %v", DBName, err)
	}

	err = client.DeleteDatabase(DBInstanceId, DBName)

	if err != nil {
		t.Errorf("Failed to delete database %s: %v", DBName, err)
	}
	t.Logf("Database %s is deleted successfully.", DBName)
}

func TestAccountCreationAndDeletion(t *testing.T) {
	client := NewTestClient()

	args := CreateAccountArgs{
		DBInstanceId:       DBInstanceId,
		AccountName:        AccountName,
		AccountPassword:    AccountPassword,
		AccountDescription: "test",
	}

	_, err := client.CreateAccount(&args)
	err = client.WaitForAccount(DBInstanceId, AccountName, Available, 600)
	if err != nil {
		t.Errorf("Failed to create account %s: %v", AccountName, err)
	}
	t.Logf("Account %s is created successfully.", AccountName)

	pargs := GrantAccountPrivilegeArgs{
		DBInstanceId:     DBInstanceId,
		AccountName:      AccountName,
		DBName:           DBName,
		AccountPrivilege: ReadWrite,
	}
	_, err = client.GrantAccountPrivilege(&pargs)
	if err != nil {
		t.Errorf("Failed to grant privilege to account %v", err)
	}
	t.Logf("Grant privilege to account %s successfully.", AccountName)

	err = client.WaitForAccountPrivilege(DBInstanceId, AccountName, DBName, ReadWrite, 200)
	if err != nil {
		t.Errorf("Failed to grant privilege to account %s: %v", AccountName, err)
	}
	t.Logf("Grant privilege to account %s successfully.", AccountName)

	_, err = client.DeleteAccount(DBInstanceId, AccountName)

	if err != nil {
		t.Errorf("Failed to delete account %s: %v", AccountName, err)
	}
	t.Logf("Account %s is deleted successfully.", AccountName)
}

func TestAccountAllocatePublicConnection(t *testing.T) {
	client := NewTestClient()

	args := AllocateInstancePublicConnectionArgs{
		DBInstanceId:           DBInstanceId,
		ConnectionStringPrefix: DBInstanceId + "o",
		Port:                   "3306",
	}

	_, err := client.AllocateInstancePublicConnection(&args)
	err = client.WaitForPublicConnection(DBInstanceId, 600)
	if err != nil {
		t.Errorf("Failed to allocate public connection: %v", err)
	}
	t.Logf("Allocate public connection successfully.")

}

func TestModifyBackupPolicy(t *testing.T) {
	client := NewTestClient()

	bargs := BackupPolicy{
		PreferredBackupTime:   "00:00Z-01:00Z",
		PreferredBackupPeriod: "Wednesday",
		BackupRetentionPeriod: 9,
	}
	args := ModifyBackupPolicyArgs{
		DBInstanceId: DBInstanceId,
		BackupPolicy: bargs,
	}

	_, err := client.ModifyBackupPolicy(&args)
	if err != nil {
		t.Errorf("Failed to modify backup policy: %v", err)
	}
	t.Logf("Modify backup policy successfully.")

}

func TestModifySecurityIps(t *testing.T) {
	client := NewTestClient()

	securityIps := "127.0.0.1"

	args := ModifySecurityIpsArgs{
		DBInstanceId: DBInstanceId,
		SecurityIps:  securityIps,
	}

	_, err := client.ModifySecurityIps(&args)
	if err != nil {
		t.Errorf("Failed to modify security ips: %v", err)
	}
	t.Logf("Modify security ips successfully.")

}

func TestModifyDBInstanceSpec(t *testing.T) {
	client := NewTestClient()

	args := ModifyDBInstanceSpecArgs{
		DBInstanceId:    DBInstanceUpgradeId,
		PayType:         Postpaid,
		DBInstanceClass: DBInstanceUpgradeClass,
	}

	_, err := client.ModifyDBInstanceSpec(&args)
	if err != nil {
		t.Errorf("Failed to modify db instance spec: %v", err)
	}

	err = client.WaitForInstance(DBInstanceUpgradeId, Running, 600)
	if err != nil {
		t.Errorf("Failed to modify db instance %s: %v", DBInstanceUpgradeId, err)
	}

	arrtArgs := DescribeDBInstanceAttributeArgs{
		DBInstanceId: DBInstanceUpgradeId,
	}
	attrResp, err := client.DescribeDBInstanceAttribute(&arrtArgs)
	t.Logf("Instance: %++v  %v", attrResp, err)

	if attrResp.Items.DBInstanceAttribute[0].DBInstanceClass != DBInstanceUpgradeClass {
		t.Errorf("Failed to modify db instance spec: %v", err)
	}

	t.Logf("Modify db instance spec successfully.")
}

func TestClient_DescribeRegions(t *testing.T) {
	client := NewTestClientForDebug()
	client.SetSecurityToken(TestSecurityToken)

	regions, err := client.DescribeRegions()
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("Result = %++v", regions)
	}
}
