package oss

import (
	"hash"
	"io"
	"net/http"
)

// Response defines HTTP response from OSS
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       io.ReadCloser
	ClientCRC  uint64
	ServerCRC  uint64
}

func (r *Response) Read(p []byte) (n int, err error) {
	return r.Body.Read(p)
}

func (r *Response) Close() error {
	return r.Body.Close()
}

// PutObjectRequest is the request of DoPutObject
type PutObjectRequest struct {
	ObjectKey string
	Reader    io.Reader
}

// GetObjectRequest is the request of DoGetObject
type GetObjectRequest struct {
	ObjectKey string
}

// GetObjectResult is the result of DoGetObject
type GetObjectResult struct {
	Response  *Response
	ClientCRC hash.Hash64
	ServerCRC uint64
}

// AppendObjectRequest is the requtest of DoAppendObject
type AppendObjectRequest struct {
	ObjectKey string
	Reader    io.Reader
	Position  int64
}

// AppendObjectResult is the result of DoAppendObject
type AppendObjectResult struct {
	NextPosition int64
	CRC          uint64
}

// UploadPartRequest is the request of DoUploadPart
type UploadPartRequest struct {
	InitResult *InitiateMultipartUploadResult
	Reader     io.Reader
	PartSize   int64
	PartNumber int
}

// UploadPartResult is the result of DoUploadPart
type UploadPartResult struct {
	Part UploadPart
}
