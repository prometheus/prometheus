package oss_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	//"net/http"
	"testing"
	"time"

	"github.com/denverdino/aliyungo/oss"
)

var (
	client           *oss.Client
	assumeRoleClient *oss.Client
	TestBucket       = strconv.FormatInt(time.Now().Unix(), 10)
)

func init() {
	AccessKeyId := os.Getenv("AccessKeyId")
	AccessKeySecret := os.Getenv("AccessKeySecret")
	if len(AccessKeyId) != 0 && len(AccessKeySecret) != 0 {
		client = oss.NewOSSClient(TestRegion, false, AccessKeyId, AccessKeySecret, false)
	} else {
		client = oss.NewOSSClient(TestRegion, false, TestAccessKeyId, TestAccessKeySecret, false)
	}

	assumeRoleClient = oss.NewOSSClientForAssumeRole(TestRegion, false, TestAccessKeyId, TestAccessKeySecret, TestSecurityToken, false)
	assumeRoleClient.SetDebug(true)
}

func TestCreateBucket(t *testing.T) {
	time.Sleep(20 * time.Second)
	b := client.Bucket(TestBucket)
	err := b.PutBucket(oss.Private)
	if err != nil {
		t.Errorf("Failed for PutBucket: %v", err)
	}
	t.Log("Wait a while for bucket creation ...")
}

func TestHead(t *testing.T) {

	b := client.Bucket(TestBucket)
	_, err := b.Head("name", nil)

	if err == nil {
		t.Errorf("Failed for Head: %v", err)
	}
}

func TestPutObject(t *testing.T) {
	const DISPOSITION = "attachment; filename=\"0x1a2b3c.jpg\""

	b := client.Bucket(TestBucket)
	err := b.Put("name", []byte("content"), "content-type", oss.Private, oss.Options{ContentDisposition: DISPOSITION})
	if err != nil {
		t.Errorf("Failed for Put: %v", err)
	}
}

func TestGet(t *testing.T) {

	b := client.Bucket(TestBucket)
	data, err := b.Get("name")

	if err != nil || string(data) != "content" {
		t.Errorf("Failed for Get: %v", err)
	}
}

func TestURL(t *testing.T) {

	b := client.Bucket(TestBucket)
	url := b.URL("name")

	t.Log("URL: ", url)
	//	/c.Assert(req.URL.Path, check.Equals, "/denverdino_test/name")
}

func TestGetReader(t *testing.T) {

	b := client.Bucket(TestBucket)
	rc, err := b.GetReader("name")
	if err != nil {
		t.Fatalf("Failed for GetReader: %v", err)
	}
	data, err := ioutil.ReadAll(rc)
	rc.Close()
	if err != nil || string(data) != "content" {
		t.Errorf("Failed for ReadAll: %v", err)
	}
}

func aTestGetNotFound(t *testing.T) {

	b := client.Bucket("non-existent-bucket")
	_, err := b.Get("non-existent")
	if err == nil {
		t.Fatalf("Failed for TestGetNotFound: %v", err)
	}
	ossErr, _ := err.(*oss.Error)
	if ossErr.StatusCode != 404 || ossErr.BucketName != "non-existent-bucket" {
		t.Errorf("Failed for TestGetNotFound: %v", err)
	}

}

func TestPutCopy(t *testing.T) {
	b := client.Bucket(TestBucket)
	t.Log("Source: ", b.Path("name"))
	res, err := b.PutCopy("newname", oss.Private, oss.CopyOptions{},
		b.Path("name"))
	if err == nil {
		t.Logf("Copy result: %v", res)
	} else {
		t.Errorf("Failed for PutCopy: %v", err)
	}
}

func TestList(t *testing.T) {

	b := client.Bucket(TestBucket)

	data, err := b.List("n", "", "", 0)
	if err != nil || len(data.Contents) != 2 {
		t.Errorf("Failed for List: %v", err)
	} else {
		t.Logf("Contents = %++v", data)
	}
}

func TestListWithDelimiter(t *testing.T) {

	b := client.Bucket(TestBucket)

	data, err := b.List("photos/2006/", "/", "some-marker", 1000)
	if err != nil || len(data.Contents) != 0 {
		t.Errorf("Failed for List: %v", err)
	} else {
		t.Logf("Contents = %++v", data)
	}

}

func TestPutReader(t *testing.T) {

	b := client.Bucket(TestBucket)
	buf := bytes.NewBufferString("content")
	err := b.PutReader("name", buf, int64(buf.Len()), "application/octet-stream", oss.Private, oss.Options{})
	if err != nil {
		t.Errorf("Failed for PutReader: %v", err)
	}
	TestGetReader(t)
}

var _fileSize int64 = 50 * 1024 * 1024
var _offset int64 = 10 * 1024 * 1024

func TestPutLargeFile(t *testing.T) {

	reader := newRandReader(_fileSize)

	b := client.Bucket(TestBucket)
	err := b.PutReader("largefile", reader, _fileSize, "application/octet-stream", oss.Private, oss.Options{})
	if err != nil {
		t.Errorf("Failed for PutReader: %v", err)
	}
}

func TestGetLargeFile(t *testing.T) {
	b := client.Bucket(TestBucket)
	headers := http.Header{}
	resp, err := b.GetResponseWithHeaders("largefile", headers)
	if err != nil {
		t.Fatalf("Failed for GetResponseWithHeaders: %v", err)
	}
	if resp.ContentLength != _fileSize {
		t.Errorf("Read file with incorrect ContentLength: %d", resp.ContentLength)

	}
	t.Logf("Large file response headers: %++v", resp.Header)

	data, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		t.Errorf("Failed for Read file: %v", err)
	}

	if len(data) != int(_fileSize) {
		t.Errorf("Incorrect length for Read with offset: %v", len(data))
	}
	resp.Body.Close()
}

func TestGetLargeFileWithOffset(t *testing.T) {
	b := client.Bucket(TestBucket)
	headers := http.Header{}
	headers.Add("Range", "bytes="+strconv.FormatInt(_offset, 10)+"-")
	resp, err := b.GetResponseWithHeaders("largefile", headers)
	if err != nil {
		t.Fatalf("Failed for GetResponseWithHeaders: %v", err)
	}
	t.Logf("Large file response headers: %++v", resp.Header)

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Failed for Read with offset: %v", err)
	}
	if len(data) != int(_fileSize-_offset) {
		t.Errorf("Incorrect length for Read with offset: %v", len(data))
	}
	resp.Body.Close()
}

func TestSignedURL(t *testing.T) {
	b := client.Bucket(TestBucket)
	expires := time.Now().Add(20 * time.Minute)
	url := b.SignedURL("largefile", expires)
	resp, err := http.Get(url)
	t.Logf("Large file response headers: %++v", resp.Header)

	if err != nil {
		t.Fatalf("Failed for GetResponseWithHeaders: %v", err)
	}
	data, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		t.Errorf("Failed for Read file: %v", err)
	}

	if len(data) != int(_fileSize) {
		t.Errorf("Incorrect length for Read with offset: %v", len(data))
	}
	resp.Body.Close()
}

func TestCopyLargeFile(t *testing.T) {
	b := client.Bucket(TestBucket)
	err := b.CopyLargeFile("largefile", "largefile2", "application/octet-stream", oss.Private, oss.Options{})
	if err != nil {
		t.Errorf("Failed for copy large file: %v", err)
	}
	t.Log("Large file copy successfully.")
	len1, err := b.GetContentLength("largefile")

	if err != nil {
		t.Fatalf("Failed for Head file: %v", err)
	}
	len2, err := b.GetContentLength("largefile2")

	if err != nil {
		t.Fatalf("Failed for Head file: %v", err)
	}

	if len1 != len2 || len1 != _fileSize {
		t.Fatalf("Content-Length should be equal %d != %d", len1, len2)
	}

	bytes1, err := b.Get("largefile")
	if err != nil {
		t.Fatalf("Failed for Get file: %v", err)
	}
	bytes2, err := b.Get("largefile2")
	if err != nil {
		t.Fatalf("Failed for Get file: %v", err)
	}

	if bytes.Compare(bytes1, bytes2) != 0 {
		t.Fatal("The result should be equal")
	}
}

func TestCopyLargeFileInParallel(t *testing.T) {
	b := client.Bucket(TestBucket)
	err := b.CopyLargeFileInParallel("largefile", "largefile3", "application/octet-stream", oss.Private, oss.Options{}, 10)
	if err != nil {
		t.Errorf("Failed for copy large file: %v", err)
	}
	t.Log("Large file copy successfully.")
	len1, err := b.GetContentLength("largefile")

	if err != nil {
		t.Fatalf("Failed for Head file: %v", err)
	}
	len2, err := b.GetContentLength("largefile3")

	if err != nil {
		t.Fatalf("Failed for Head file: %v", err)
	}

	if len1 != len2 || len1 != _fileSize {
		t.Fatalf("Content-Length should be equal %d != %d", len1, len2)
	}

	bytes1, err := b.Get("largefile")
	if err != nil {
		t.Fatalf("Failed for Get file: %v", err)
	}
	bytes2, err := b.Get("largefile3")
	if err != nil {
		t.Fatalf("Failed for Get file: %v", err)
	}

	if bytes.Compare(bytes1, bytes2) != 0 {
		t.Fatal("The result should be equal")
	}
}

func TestDelLargeObject(t *testing.T) {

	b := client.Bucket(TestBucket)
	err := b.Del("largefile")
	if err != nil {
		t.Errorf("Failed for Del largefile: %v", err)
	}
	err = b.Del("largefile2")
	if err != nil {
		t.Errorf("Failed for Del largefile2: %v", err)
	}
	err = b.Del("largefile3")
	if err != nil {
		t.Errorf("Failed for Del largefile2: %v", err)
	}
}

func TestExists(t *testing.T) {

	b := client.Bucket(TestBucket)
	result, err := b.Exists("name")
	if err != nil || result != true {
		t.Errorf("Failed for Exists: %v", err)
	}
}

func TestLocation(t *testing.T) {
	b := client.Bucket(TestBucket)
	result, err := b.Location()

	if err != nil || result != string(TestRegion) {
		t.Errorf("Failed for Location: %v %s", err, result)
	}
}

func TestACL(t *testing.T) {
	b := client.Bucket(TestBucket)
	result, err := b.ACL()

	if err != nil {
		t.Errorf("Failed for ACL: %v", err)
	} else {
		t.Logf("AccessControlPolicy: %++v", result)
	}
}

func TestDelObject(t *testing.T) {

	b := client.Bucket(TestBucket)
	err := b.Del("name")
	if err != nil {
		t.Errorf("Failed for Del: %v", err)
	}
}

func TestDelMultiObjects(t *testing.T) {

	b := client.Bucket(TestBucket)
	objects := []oss.Object{oss.Object{Key: "newname"}}
	err := b.DelMulti(oss.Delete{
		Quiet:   false,
		Objects: objects,
	})
	if err != nil {
		t.Errorf("Failed for DelMulti: %v", err)
	}
}

func TestGetService(t *testing.T) {
	bucketList, err := client.GetService()
	if err != nil {
		t.Errorf("Unable to get service: %v", err)
	} else {
		t.Logf("GetService: %++v", bucketList)
	}
}

func TestGetBucketInfo(t *testing.T) {
	b := client.Bucket(TestBucket)
	resp, err := b.Info()
	if err != nil {
		t.Errorf("Failed for Info: %v", err)
	} else {
		t.Logf("Bucket Info: %v", resp)
	}
}

func TestDelBucket(t *testing.T) {

	b := client.Bucket(TestBucket)
	err := b.DelBucket()
	if err != nil {
		t.Errorf("Failed for DelBucket: %v", err)
	}
}

type randReader struct {
	r int64
	m sync.Mutex
}

func (rr *randReader) Read(p []byte) (n int, err error) {
	rr.m.Lock()
	defer rr.m.Unlock()
	for i := 0; i < len(p) && rr.r > 0; i++ {
		p[i] = byte(rand.Intn(255))
		n++
		rr.r--
	}
	if rr.r == 0 {
		err = io.EOF
	}
	return
}

func newRandReader(n int64) *randReader {
	return &randReader{r: n}
}

func TestNewOSSClientForAssumeRole_GetServices(t *testing.T) {
	bucketList, err := assumeRoleClient.GetService()
	if err != nil {
		t.Fatalf("Error %++v", err)
	} else {
		t.Logf("GetService: %++v", bucketList)
	}
}
