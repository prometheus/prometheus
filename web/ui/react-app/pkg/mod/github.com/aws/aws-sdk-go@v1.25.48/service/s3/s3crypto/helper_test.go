package s3crypto

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/awstesting"
	"github.com/aws/aws-sdk-go/internal/sdkio"
)

func TestBytesReadWriteSeeker_Read(t *testing.T) {
	b := &bytesReadWriteSeeker{[]byte{1, 2, 3}, 0}
	expected := []byte{1, 2, 3}
	buf := make([]byte, 3)
	n, err := b.Read(buf)

	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}

	if e, a := 3, n; e != a {
		t.Errorf("expected %d, but received %d", e, a)
	}

	if !bytes.Equal(expected, buf) {
		t.Error("expected equivalent byte slices, but received otherwise")
	}
}

func TestBytesReadWriteSeeker_Write(t *testing.T) {
	b := &bytesReadWriteSeeker{}
	expected := []byte{1, 2, 3}
	buf := make([]byte, 3)
	n, err := b.Write([]byte{1, 2, 3})

	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}

	if e, a := 3, n; e != a {
		t.Errorf("expected %d, but received %d", e, a)
	}

	n, err = b.Read(buf)
	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}

	if e, a := 3, n; e != a {
		t.Errorf("expected %d, but received %d", e, a)
	}

	if !bytes.Equal(expected, buf) {
		t.Error("expected equivalent byte slices, but received otherwise")
	}
}

func TestBytesReadWriteSeeker_Seek(t *testing.T) {
	b := &bytesReadWriteSeeker{[]byte{1, 2, 3}, 0}
	expected := []byte{2, 3}
	m, err := b.Seek(1, sdkio.SeekStart)

	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}

	if e, a := 1, int(m); e != a {
		t.Errorf("expected %d, but received %d", e, a)
	}

	buf := make([]byte, 3)
	n, err := b.Read(buf)

	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}

	if e, a := 2, n; e != a {
		t.Errorf("expected %d, but received %d", e, a)
	}

	if !bytes.Equal(expected, buf[:n]) {
		t.Error("expected equivalent byte slices, but received otherwise")
	}
}

func TestGetWriterStore_TempFile(t *testing.T) {
	response := http.Response{StatusCode: 200}
	s := awstesting.NewClient(aws.NewConfig().WithMaxRetries(10))
	s.Handlers.Validate.Clear()
	s.Handlers.Send.Clear() // mock sending
	s.Handlers.Send.PushBack(func(r *request.Request) {
		r.HTTPResponse = &response
	})
	type testData struct {
		Data string
	}
	out := &testData{}
	r := s.NewRequest(&request.Operation{Name: "Operation"}, nil, out)
	f, err := getWriterStore(r, "", true)
	if err != nil {
		t.Fatalf("expected no error, but received %v", err)
	}
	tempFile, ok := f.(*os.File)
	if !ok {
		t.Fatal("io.ReadWriteSeeker expected to be *os.file")
	}
	err = r.Send()
	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}
	if _, err := os.Stat(tempFile.Name()); !os.IsNotExist(err) {
		t.Errorf("expected temp file be deleted, but still exists %v", tempFile.Name())
	}
}

func TestGetWriterStore_Memory(t *testing.T) {
	response := http.Response{StatusCode: 200}
	s := awstesting.NewClient(aws.NewConfig().WithMaxRetries(10))
	s.Handlers.Validate.Clear()
	s.Handlers.Send.Clear() // mock sending
	s.Handlers.Send.PushBack(func(r *request.Request) {
		r.HTTPResponse = &response
	})
	type testData struct {
		Data string
	}
	out := &testData{}
	r := s.NewRequest(&request.Operation{Name: "Operation"}, nil, out)
	f, err := getWriterStore(r, "", false)
	if err != nil {
		t.Fatalf("expected no error, but received %v", err)
	}
	if _, ok := f.(*bytesReadWriteSeeker); !ok {
		t.Fatal("io.ReadWriteSeeker expected to be *bytesReadWriteSeeker")
	}
	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}
	err = r.Send()
	if err != nil {
		t.Errorf("expected no error, but received %v", err)
	}
}
