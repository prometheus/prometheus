package oss

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
)

// InitiateMultipartUpload initializes multipart upload
//
// objectKey    object name
// options    the object constricts for upload. The valid options are CacheControl, ContentDisposition, ContentEncoding, Expires,
//            ServerSideEncryption, Meta, check out the following link:
//            https://help.aliyun.com/document_detail/oss/api-reference/multipart-upload/InitiateMultipartUpload.html
//
// InitiateMultipartUploadResult    the return value of the InitiateMultipartUpload, which is used for calls later on such as UploadPartFromFile,UploadPartCopy.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) InitiateMultipartUpload(objectKey string, options ...Option) (InitiateMultipartUploadResult, error) {
	var imur InitiateMultipartUploadResult
	opts := addContentType(options, objectKey)
	params := map[string]interface{}{}
	params["uploads"] = nil
	resp, err := bucket.do("POST", objectKey, params, opts, nil, nil)
	if err != nil {
		return imur, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &imur)
	return imur, err
}

// UploadPart uploads parts
//
// After initializing a Multipart Upload, the upload Id and object key could be used for uploading the parts.
// Each part has its part number (ranges from 1 to 10,000). And for each upload Id, the part number identifies the position of the part in the whole file.
// And thus with the same part number and upload Id, another part upload will overwrite the data.
// Except the last one, minimal part size is 100KB. There's no limit on the last part size.
//
// imur    the returned value of InitiateMultipartUpload.
// reader    io.Reader the reader for the part's data.
// size    the part size.
// partNumber    the part number (ranges from 1 to 10,000). Invalid part number will lead to InvalidArgument error.
//
// UploadPart    the return value of the upload part. It consists of PartNumber and ETag. It's valid when error is nil.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) UploadPart(imur InitiateMultipartUploadResult, reader io.Reader,
	partSize int64, partNumber int, options ...Option) (UploadPart, error) {
	request := &UploadPartRequest{
		InitResult: &imur,
		Reader:     reader,
		PartSize:   partSize,
		PartNumber: partNumber,
	}

	result, err := bucket.DoUploadPart(request, options)

	return result.Part, err
}

// UploadPartFromFile uploads part from the file.
//
// imur    the return value of a successful InitiateMultipartUpload.
// filePath    the local file path to upload.
// startPosition    the start position in the local file.
// partSize    the part size.
// partNumber    the part number (from 1 to 10,000)
//
// UploadPart    the return value consists of PartNumber and ETag.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) UploadPartFromFile(imur InitiateMultipartUploadResult, filePath string,
	startPosition, partSize int64, partNumber int, options ...Option) (UploadPart, error) {
	var part = UploadPart{}
	fd, err := os.Open(filePath)
	if err != nil {
		return part, err
	}
	defer fd.Close()
	fd.Seek(startPosition, os.SEEK_SET)

	request := &UploadPartRequest{
		InitResult: &imur,
		Reader:     fd,
		PartSize:   partSize,
		PartNumber: partNumber,
	}

	result, err := bucket.DoUploadPart(request, options)

	return result.Part, err
}

// DoUploadPart does the actual part upload.
//
// request    part upload request
//
// UploadPartResult    the result of uploading part.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) DoUploadPart(request *UploadPartRequest, options []Option) (*UploadPartResult, error) {
	listener := getProgressListener(options)
	options = append(options, ContentLength(request.PartSize))
	params := map[string]interface{}{}
	params["partNumber"] = strconv.Itoa(request.PartNumber)
	params["uploadId"] = request.InitResult.UploadID
	resp, err := bucket.do("PUT", request.InitResult.Key, params, options,
		&io.LimitedReader{R: request.Reader, N: request.PartSize}, listener)
	if err != nil {
		return &UploadPartResult{}, err
	}
	defer resp.Body.Close()

	part := UploadPart{
		ETag:       resp.Headers.Get(HTTPHeaderEtag),
		PartNumber: request.PartNumber,
	}

	if bucket.getConfig().IsEnableCRC {
		err = checkCRC(resp, "DoUploadPart")
		if err != nil {
			return &UploadPartResult{part}, err
		}
	}

	return &UploadPartResult{part}, nil
}

// UploadPartCopy uploads part copy
//
// imur    the return value of InitiateMultipartUpload
// copySrc    source Object name
// startPosition    the part's start index in the source file
// partSize    the part size
// partNumber    the part number, ranges from 1 to 10,000. If it exceeds the range OSS returns InvalidArgument error.
// options    the constraints of source object for the copy. The copy happens only when these contraints are met. Otherwise it returns error.
//            CopySourceIfNoneMatch, CopySourceIfModifiedSince  CopySourceIfUnmodifiedSince, check out the following link for the detail
//            https://help.aliyun.com/document_detail/oss/api-reference/multipart-upload/UploadPartCopy.html
//
// UploadPart    the return value consists of PartNumber and ETag.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) UploadPartCopy(imur InitiateMultipartUploadResult, srcBucketName, srcObjectKey string,
	startPosition, partSize int64, partNumber int, options ...Option) (UploadPart, error) {
	var out UploadPartCopyResult
	var part UploadPart

	opts := []Option{CopySource(srcBucketName, url.QueryEscape(srcObjectKey)),
		CopySourceRange(startPosition, partSize)}
	opts = append(opts, options...)
	params := map[string]interface{}{}
	params["partNumber"] = strconv.Itoa(partNumber)
	params["uploadId"] = imur.UploadID
	resp, err := bucket.do("PUT", imur.Key, params, opts, nil, nil)
	if err != nil {
		return part, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	if err != nil {
		return part, err
	}
	part.ETag = out.ETag
	part.PartNumber = partNumber

	return part, nil
}

// CompleteMultipartUpload completes the multipart upload.
//
// imur    the return value of InitiateMultipartUpload.
// parts    the array of return value of UploadPart/UploadPartFromFile/UploadPartCopy.
//
// CompleteMultipartUploadResponse    the return value when the call succeeds. Only valid when the error is nil.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) CompleteMultipartUpload(imur InitiateMultipartUploadResult,
	parts []UploadPart, options ...Option) (CompleteMultipartUploadResult, error) {
	var out CompleteMultipartUploadResult

	sort.Sort(uploadParts(parts))
	cxml := completeMultipartUploadXML{}
	cxml.Part = parts
	bs, err := xml.Marshal(cxml)
	if err != nil {
		return out, err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	params := map[string]interface{}{}
	params["uploadId"] = imur.UploadID
	resp, err := bucket.do("POST", imur.Key, params, options, buffer, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// AbortMultipartUpload aborts the multipart upload.
//
// imur    the return value of InitiateMultipartUpload.
//
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) AbortMultipartUpload(imur InitiateMultipartUploadResult, options ...Option) error {
	params := map[string]interface{}{}
	params["uploadId"] = imur.UploadID
	resp, err := bucket.do("DELETE", imur.Key, params, options, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

// ListUploadedParts lists the uploaded parts.
//
// imur    the return value of InitiateMultipartUpload.
//
// ListUploadedPartsResponse    the return value if it succeeds, only valid when error is nil.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) ListUploadedParts(imur InitiateMultipartUploadResult, options ...Option) (ListUploadedPartsResult, error) {
	var out ListUploadedPartsResult
	options = append(options, EncodingType("url"))

	params := map[string]interface{}{}
	params, err := getRawParams(options)
	if err != nil {
		return out, err
	}

	params["uploadId"] = imur.UploadID
	resp, err := bucket.do("GET", imur.Key, params, nil, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	if err != nil {
		return out, err
	}
	err = decodeListUploadedPartsResult(&out)
	return out, err
}

// ListMultipartUploads lists all ongoing multipart upload tasks
//
// options    listObject's filter. Prefix specifies the returned object's prefix; KeyMarker specifies the returned object's start point in lexicographic order;
//            MaxKeys specifies the max entries to return; Delimiter is the character for grouping object keys.
//
// ListMultipartUploadResponse    the return value if it succeeds, only valid when error is nil.
// error    it's nil if the operation succeeds, otherwise it's an error object.
//
func (bucket Bucket) ListMultipartUploads(options ...Option) (ListMultipartUploadResult, error) {
	var out ListMultipartUploadResult

	options = append(options, EncodingType("url"))
	params, err := getRawParams(options)
	if err != nil {
		return out, err
	}
	params["uploads"] = nil

	resp, err := bucket.do("GET", "", params, options, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	if err != nil {
		return out, err
	}
	err = decodeListMultipartUploadResult(&out)
	return out, err
}
