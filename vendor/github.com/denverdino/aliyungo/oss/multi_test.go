package oss_test

import (
	//"encoding/xml"
	"testing"

	"github.com/denverdino/aliyungo/oss"
	//"io"
	//"io/ioutil"
	"strings"
)

func TestCreateBucketMulti(t *testing.T) {
	TestCreateBucket(t)
}

func TestInitMulti(t *testing.T) {
	b := client.Bucket(TestBucket)

	metadata := make(map[string][]string)
	metadata["key1"] = []string{"value1"}
	metadata["key2"] = []string{"value2"}
	options := oss.Options{
		ServerSideEncryption: true,
		Meta:                 metadata,
		ContentEncoding:      "text/utf8",
		CacheControl:         "no-cache",
		ContentMD5:           "0000000000000000",
	}

	multi, err := b.InitMulti("multi", "text/plain", oss.Private, options)
	if err != nil {
		t.Errorf("Failed for InitMulti: %v", err)
	} else {
		t.Logf("InitMulti result: %++v", multi)
	}
}

func TestMultiReturnOld(t *testing.T) {

	b := client.Bucket(TestBucket)

	multi, err := b.Multi("multi", "text/plain", oss.Private, oss.Options{})
	if err != nil {
		t.Errorf("Failed for Multi: %v", err)
	} else {
		t.Logf("Multi result: %++v", multi)
	}

}

func TestPutPart(t *testing.T) {

	b := client.Bucket(TestBucket)

	multi, err := b.Multi("multi", "text/plain", oss.Private, oss.Options{})
	if err != nil {
		t.Fatalf("Failed for Multi: %v", err)
	}

	part, err := multi.PutPart(1, strings.NewReader("<part 1>"))
	if err != nil {
		t.Errorf("Failed for PutPart: %v", err)
	} else {
		t.Logf("PutPart result: %++v", part)
	}

}
func TestPutPartCopy(t *testing.T) {

	TestPutObject(t)

	b := client.Bucket(TestBucket)

	multi, err := b.Multi("multi", "text/plain", oss.Private, oss.Options{})
	if err != nil {
		t.Fatalf("Failed for Multi: %v", err)
	}

	res, part, err := multi.PutPartCopy(2, oss.CopyOptions{}, b.Path("name"))
	if err != nil {
		t.Errorf("Failed for PutPartCopy: %v", err)
	} else {
		t.Logf("PutPartCopy result: %++v %++v", part, res)
	}
	TestDelObject(t)
}

func TestListParts(t *testing.T) {

	b := client.Bucket(TestBucket)

	multi, err := b.Multi("multi", "text/plain", oss.Private, oss.Options{})
	if err != nil {
		t.Fatalf("Failed for Multi: %v", err)
	}

	parts, err := multi.ListParts()
	if err != nil {
		t.Errorf("Failed for ListParts: %v", err)
	} else {
		t.Logf("ListParts result: %++v", parts)
	}
}
func TestListMulti(t *testing.T) {

	b := client.Bucket(TestBucket)

	multis, prefixes, err := b.ListMulti("", "/")
	if err != nil {
		t.Errorf("Failed for ListMulti: %v", err)
	} else {
		t.Logf("ListMulti result : %++v  %++v", multis, prefixes)
	}
}
func TestMultiAbort(t *testing.T) {

	b := client.Bucket(TestBucket)

	multi, err := b.Multi("multi", "text/plain", oss.Private, oss.Options{})
	if err != nil {
		t.Fatalf("Failed for Multi: %v", err)
	}

	err = multi.Abort()
	if err != nil {
		t.Errorf("Failed for Abort: %v", err)
	}

}

func TestPutAll(t *testing.T) {
	TestInitMulti(t)
	// Don't retry the NoSuchUpload error.
	b := client.Bucket(TestBucket)

	multi, err := b.Multi("multi", "text/plain", oss.Private, oss.Options{})
	if err != nil {
		t.Fatalf("Failed for Multi: %v", err)
	}

	// Must send at least one part, so that completing it will work.
	parts, err := multi.PutAll(strings.NewReader("part1part2last"), 5)
	if err != nil {
		t.Errorf("Failed for PutAll: %v", err)
	} else {
		t.Logf("PutAll result: %++v", parts)
	}
	//	// Must send at least one part, so that completing it will work.
	//	err = multi.Complete(parts)
	//	if err != nil {
	//		t.Errorf("Failed for Complete: %v", err)
	//	}
	err = multi.Abort()
	if err != nil {
		t.Errorf("Failed for Abort: %v", err)
	}
}

func TestCleanUp(t *testing.T) {
	TestDelBucket(t)
}
