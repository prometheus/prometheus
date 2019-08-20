package util

import (
	"testing"
)

func TestCreateSignature(t *testing.T) {

	str := "GET&%2F&AccessKeyId%3Dtestid%26Action%3DDescribeRegions%26Format%3DXML%26RegionId%3Dregion1%26SignatureMethod%3DHMAC-SHA1%26SignatureNonce%3DNwDAxvLU6tFE0DVb%26SignatureVersion%3D1.0%26TimeStamp%3D2012-12-26T10%253A33%253A56Z%26Version%3D2014-05-26"

	signature := CreateSignature(str, "testsecret")

	t.Log(signature)
}
