package oss

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"hash"
	"hash/crc64"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Bucket implements the operations of object.
type Bucket struct {
	Client     Client
	BucketName string
}

// PutObject creates a new object and it will overwrite the original one if it exists already.
//
// objectKey    the object key in UTF-8 encoding. The length must be between 1 and 1023, and cannot start with "/" or "\".
// reader    io.Reader instance for reading the data for uploading
// options    the options for uploading the object. The valid options here are CacheControl, ContentDisposition, ContentEncoding
//            Expires, ServerSideEncryption, ObjectACL and Meta. Refer to the link below for more details.
//            https://help.aliyun.com/document_detail/oss/api-reference/object/PutObject.html
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) PutObject(objectKey string, reader io.Reader, options ...Option) error {
	opts := addContentType(options, objectKey)

	request := &PutObjectRequest{
		ObjectKey: objectKey,
		Reader:    reader,
	}
	resp, err := bucket.DoPutObject(request, opts)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return err
}

// PutObjectFromFile creates a new object from the local file.
//
// objectKey    object key.
// filePath    the local file path to upload.
// options    the options for uploading the object. Refer to the parameter options in PutObject for more details.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) PutObjectFromFile(objectKey, filePath string, options ...Option) error {
	fd, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fd.Close()

	opts := addContentType(options, filePath, objectKey)

	request := &PutObjectRequest{
		ObjectKey: objectKey,
		Reader:    fd,
	}
	resp, err := bucket.DoPutObject(request, opts)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return err
}

// DoPutObject does the actual upload work.
//
// request    the request instance for uploading an object.
// options    the options for uploading an object.
//
// Response    the response from OSS.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DoPutObject(request *PutObjectRequest, options []Option) (*Response, error) {
	isOptSet, _, _ := isOptionSet(options, HTTPHeaderContentType)
	if !isOptSet {
		options = addContentType(options, request.ObjectKey)
	}

	listener := getProgressListener(options)

	params := map[string]interface{}{}
	resp, err := bucket.do("PUT", request.ObjectKey, params, options, request.Reader, listener)
	if err != nil {
		return nil, err
	}

	if bucket.getConfig().IsEnableCRC {
		err = checkCRC(resp, "DoPutObject")
		if err != nil {
			return resp, err
		}
	}

	err = checkRespCode(resp.StatusCode, []int{http.StatusOK})

	return resp, err
}

// GetObject downloads the object.
//
// objectKey    the object key.
// options    the options for downloading the object. The valid values are: Range, IfModifiedSince, IfUnmodifiedSince, IfMatch,
//            IfNoneMatch, AcceptEncoding. For more details, please check out:
//            https://help.aliyun.com/document_detail/oss/api-reference/object/GetObject.html
//
// io.ReadCloser    reader instance for reading data from response. It must be called close() after the usage and only valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) GetObject(objectKey string, options ...Option) (io.ReadCloser, error) {
	result, err := bucket.DoGetObject(&GetObjectRequest{objectKey}, options)
	if err != nil {
		return nil, err
	}

	return result.Response, nil
}

// GetObjectToFile downloads the data to a local file.
//
// objectKey    the object key to download.
// filePath    the local file to store the object data.
// options    the options for downloading the object. Refer to the parameter options in method GetObject for more details.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) GetObjectToFile(objectKey, filePath string, options ...Option) error {
	tempFilePath := filePath + TempFileSuffix

	// Calls the API to actually download the object. Returns the result instance.
	result, err := bucket.DoGetObject(&GetObjectRequest{objectKey}, options)
	if err != nil {
		return err
	}
	defer result.Response.Close()

	// If the local file does not exist, create a new one. If it exists, overwrite it.
	fd, err := os.OpenFile(tempFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, FilePermMode)
	if err != nil {
		return err
	}

	// Copy the data to the local file path.
	_, err = io.Copy(fd, result.Response.Body)
	fd.Close()
	if err != nil {
		return err
	}

	// Compares the CRC value
	hasRange, _, _ := isOptionSet(options, HTTPHeaderRange)
	encodeOpt, _ := findOption(options, HTTPHeaderAcceptEncoding, nil)
	acceptEncoding := ""
	if encodeOpt != nil {
		acceptEncoding = encodeOpt.(string)
	}
	if bucket.getConfig().IsEnableCRC && !hasRange && acceptEncoding != "gzip" {
		result.Response.ClientCRC = result.ClientCRC.Sum64()
		err = checkCRC(result.Response, "GetObjectToFile")
		if err != nil {
			os.Remove(tempFilePath)
			return err
		}
	}

	return os.Rename(tempFilePath, filePath)
}

// DoGetObject is the actual API that gets the object. It's the internal function called by other public APIs.
//
// request    the request to download the object.
// options    the options for downloading the file. Checks out the parameter options in method GetObject.
//
// GetObjectResult    the result instance of getting the object.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DoGetObject(request *GetObjectRequest, options []Option) (*GetObjectResult, error) {
	params, _ := getRawParams(options)
	resp, err := bucket.do("GET", request.ObjectKey, params, options, nil, nil)
	if err != nil {
		return nil, err
	}

	result := &GetObjectResult{
		Response: resp,
	}

	// CRC
	var crcCalc hash.Hash64
	hasRange, _, _ := isOptionSet(options, HTTPHeaderRange)
	if bucket.getConfig().IsEnableCRC && !hasRange {
		crcCalc = crc64.New(crcTable())
		result.ServerCRC = resp.ServerCRC
		result.ClientCRC = crcCalc
	}

	// Progress
	listener := getProgressListener(options)

	contentLen, _ := strconv.ParseInt(resp.Headers.Get(HTTPHeaderContentLength), 10, 64)
	resp.Body = TeeReader(resp.Body, crcCalc, contentLen, listener, nil)

	return result, nil
}

// CopyObject copies the object inside the bucket.
//
// srcObjectKey    the source object to copy.
// destObjectKey    the target object to copy.
// options    options for copying an object. You can specify the conditions of copy. The valid conditions are CopySourceIfMatch,
//            CopySourceIfNoneMatch, CopySourceIfModifiedSince, CopySourceIfUnmodifiedSince, MetadataDirective.
//            Also you can specify the target object's attributes, such as CacheControl, ContentDisposition, ContentEncoding, Expires,
//            ServerSideEncryption, ObjectACL, Meta. Refer to the link below for more details :
//            https://help.aliyun.com/document_detail/oss/api-reference/object/CopyObject.html
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) CopyObject(srcObjectKey, destObjectKey string, options ...Option) (CopyObjectResult, error) {
	var out CopyObjectResult
	options = append(options, CopySource(bucket.BucketName, url.QueryEscape(srcObjectKey)))
	params := map[string]interface{}{}
	resp, err := bucket.do("PUT", destObjectKey, params, options, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// CopyObjectTo copies the object to another bucket.
//
// srcObjectKey    source object key. The source bucket is Bucket.BucketName .
// destBucketName    target bucket name.
// destObjectKey    target object name.
// options    copy options, check out parameter options in function CopyObject for more details.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) CopyObjectTo(destBucketName, destObjectKey, srcObjectKey string, options ...Option) (CopyObjectResult, error) {
	return bucket.copy(srcObjectKey, destBucketName, destObjectKey, options...)
}

//
// CopyObjectFrom copies the object to another bucket.
//
// srcBucketName    source bucket name.
// srcObjectKey    source object name.
// destObjectKey    target object name. The target bucket name is Bucket.BucketName.
// options    copy options. Check out parameter options in function CopyObject.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) CopyObjectFrom(srcBucketName, srcObjectKey, destObjectKey string, options ...Option) (CopyObjectResult, error) {
	destBucketName := bucket.BucketName
	var out CopyObjectResult
	srcBucket, err := bucket.Client.Bucket(srcBucketName)
	if err != nil {
		return out, err
	}

	return srcBucket.copy(srcObjectKey, destBucketName, destObjectKey, options...)
}

func (bucket Bucket) copy(srcObjectKey, destBucketName, destObjectKey string, options ...Option) (CopyObjectResult, error) {
	var out CopyObjectResult
	options = append(options, CopySource(bucket.BucketName, url.QueryEscape(srcObjectKey)))
	headers := make(map[string]string)
	err := handleOptions(headers, options)
	if err != nil {
		return out, err
	}
	params := map[string]interface{}{}
	resp, err := bucket.Client.Conn.Do("PUT", destBucketName, destObjectKey, params, headers, nil, 0, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// AppendObject uploads the data in the way of appending an existing or new object.
//
// AppendObject the parameter appendPosition specifies which postion (in the target object) to append. For the first append (to a non-existing file),
// the appendPosition should be 0. The appendPosition in the subsequent calls will be the current object length.
// For example, the first appendObject's appendPosition is 0 and it uploaded 65536 bytes data, then the second call's position is 65536.
// The response header x-oss-next-append-position after each successful request also specifies the next call's append position (so the caller need not to maintain this information).
//
// objectKey    the target object to append to.
// reader    io.Reader. The read instance for reading the data to append.
// appendPosition    the start position to append.
// destObjectProperties    the options for the first appending, such as CacheControl, ContentDisposition, ContentEncoding,
//                         Expires, ServerSideEncryption, ObjectACL.
//
// int64    the next append position, it's valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) AppendObject(objectKey string, reader io.Reader, appendPosition int64, options ...Option) (int64, error) {
	request := &AppendObjectRequest{
		ObjectKey: objectKey,
		Reader:    reader,
		Position:  appendPosition,
	}

	result, err := bucket.DoAppendObject(request, options)
	if err != nil {
		return appendPosition, err
	}

	return result.NextPosition, err
}

// DoAppendObject is the actual API that does the object append.
//
// request    the request object for appending object.
// options    the options for appending object.
//
// AppendObjectResult    the result object for appending object.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DoAppendObject(request *AppendObjectRequest, options []Option) (*AppendObjectResult, error) {
	params := map[string]interface{}{}
	params["append"] = nil
	params["position"] = strconv.FormatInt(request.Position, 10)
	headers := make(map[string]string)

	opts := addContentType(options, request.ObjectKey)
	handleOptions(headers, opts)

	var initCRC uint64
	isCRCSet, initCRCOpt, _ := isOptionSet(options, initCRC64)
	if isCRCSet {
		initCRC = initCRCOpt.(uint64)
	}

	listener := getProgressListener(options)

	handleOptions(headers, opts)
	resp, err := bucket.Client.Conn.Do("POST", bucket.BucketName, request.ObjectKey, params, headers,
		request.Reader, initCRC, listener)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	nextPosition, _ := strconv.ParseInt(resp.Headers.Get(HTTPHeaderOssNextAppendPosition), 10, 64)
	result := &AppendObjectResult{
		NextPosition: nextPosition,
		CRC:          resp.ServerCRC,
	}

	if bucket.getConfig().IsEnableCRC && isCRCSet {
		err = checkCRC(resp, "AppendObject")
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

// DeleteObject deletes the object.
//
// objectKey    the object key to delete.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DeleteObject(objectKey string) error {
	params := map[string]interface{}{}
	resp, err := bucket.do("DELETE", objectKey, params, nil, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

// DeleteObjects deletes multiple objects.
//
// objectKeys    the object keys to delete.
// options    the options for deleting objects.
//            Supported option is DeleteObjectsQuiet which means it will not return error even deletion failed (not recommended). By default it's not used.
//
// DeleteObjectsResult    the result object.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DeleteObjects(objectKeys []string, options ...Option) (DeleteObjectsResult, error) {
	out := DeleteObjectsResult{}
	dxml := deleteXML{}
	for _, key := range objectKeys {
		dxml.Objects = append(dxml.Objects, DeleteObject{Key: key})
	}
	isQuiet, _ := findOption(options, deleteObjectsQuiet, false)
	dxml.Quiet = isQuiet.(bool)

	bs, err := xml.Marshal(dxml)
	if err != nil {
		return out, err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	contentType := http.DetectContentType(buffer.Bytes())
	options = append(options, ContentType(contentType))
	sum := md5.Sum(bs)
	b64 := base64.StdEncoding.EncodeToString(sum[:])
	options = append(options, ContentMD5(b64))

	params := map[string]interface{}{}
	params["delete"] = nil
	params["encoding-type"] = "url"

	resp, err := bucket.do("POST", "", params, options, buffer, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	if !dxml.Quiet {
		if err = xmlUnmarshal(resp.Body, &out); err == nil {
			err = decodeDeleteObjectsResult(&out)
		}
	}
	return out, err
}

// IsObjectExist checks if the object exists.
//
// bool    flag of object's existence (true:exists; false:non-exist) when error is nil.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) IsObjectExist(objectKey string) (bool, error) {
	_, err := bucket.GetObjectMeta(objectKey)
	if err == nil {
		return true, nil
	}

	switch err.(type) {
	case ServiceError:
		if err.(ServiceError).StatusCode == 404 {
			return false, nil
		}
	}

	return false, err
}

// ListObjects lists the objects under the current bucket.
//
// options    it contains all the filters for listing objects.
//            It could specify a prefix filter on object keys,  the max keys count to return and the object key marker and the delimiter for grouping object names.
//            The key marker means the returned objects' key must be greater than it in lexicographic order.
//
//            For example, if the bucket has 8 objects, my-object-1, my-object-11, my-object-2, my-object-21,
//            my-object-22, my-object-3, my-object-31, my-object-32. If the prefix is my-object-2 (no other filters), then it returns
//            my-object-2, my-object-21, my-object-22 three objects. If the marker is my-object-22 (no other filters), then it returns
//            my-object-3, my-object-31, my-object-32 three objects. If the max keys is 5, then it returns 5 objects.
//            The three filters could be used together to achieve filter and paging functionality.
//            If the prefix is the folder name, then it could list all files under this folder (including the files under its subfolders).
//            But if the delimiter is specified with '/', then it only returns that folder's files (no subfolder's files). The direct subfolders are in the commonPrefixes properties.
//            For example, if the bucket has three objects fun/test.jpg, fun/movie/001.avi, fun/movie/007.avi. And if the prefix is "fun/", then it returns all three objects.
//            But if the delimiter is '/', then only "fun/test.jpg" is returned as files and fun/movie/ is returned as common prefix.
//
//            For common usage scenario, check out sample/list_object.go.
//
// ListObjectsResponse    the return value after operation succeeds (only valid when error is nil).
//
func (bucket Bucket) ListObjects(options ...Option) (ListObjectsResult, error) {
	var out ListObjectsResult

	options = append(options, EncodingType("url"))
	params, err := getRawParams(options)
	if err != nil {
		return out, err
	}

	resp, err := bucket.do("GET", "", params, options, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	if err != nil {
		return out, err
	}

	err = decodeListObjectsResult(&out)
	return out, err
}

// SetObjectMeta sets the metadata of the Object.
//
// objectKey    object
// options    options for setting the metadata. The valid options are CacheControl, ContentDisposition, ContentEncoding, Expires,
//            ServerSideEncryption, and custom metadata.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) SetObjectMeta(objectKey string, options ...Option) error {
	options = append(options, MetadataDirective(MetaReplace))
	_, err := bucket.CopyObject(objectKey, objectKey, options...)
	return err
}

// GetObjectDetailedMeta gets the object's detailed metadata
//
// objectKey    object key.
// options    the constraints of the object. Only when the object meets the requirements this method will return the metadata. Otherwise returns error. Valid options are IfModifiedSince, IfUnmodifiedSince,
//            IfMatch, IfNoneMatch. For more details check out https://help.aliyun.com/document_detail/oss/api-reference/object/HeadObject.html
//
// http.Header    object meta when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) GetObjectDetailedMeta(objectKey string, options ...Option) (http.Header, error) {
	params := map[string]interface{}{}
	resp, err := bucket.do("HEAD", objectKey, params, options, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return resp.Headers, nil
}

// GetObjectMeta gets object metadata.
//
// GetObjectMeta is more lightweight than GetObjectDetailedMeta as it only returns basic metadata including ETag
// size, LastModified. The size information is in the HTTP header Content-Length.
//
// objectKey    object key
//
// http.Header    the object's metadata, valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) GetObjectMeta(objectKey string, options ...Option) (http.Header, error) {
	params := map[string]interface{}{}
	params["objectMeta"] = nil
	//resp, err := bucket.do("GET", objectKey, "?objectMeta", "", nil, nil, nil)
	resp, err := bucket.do("HEAD", objectKey, params, options, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return resp.Headers, nil
}

// SetObjectACL updates the object's ACL.
//
// Only the bucket's owner could update object's ACL which priority is higher than bucket's ACL.
// For example, if the bucket ACL is private and object's ACL is public-read-write.
// Then object's ACL is used and it means all users could read or write that object.
// When the object's ACL is not set, then bucket's ACL is used as the object's ACL.
//
// Object read operations include GetObject, HeadObject, CopyObject and UploadPartCopy on the source object;
// Object write operations include PutObject, PostObject, AppendObject, DeleteObject, DeleteMultipleObjects,
// CompleteMultipartUpload and CopyObject on target object.
//
// objectKey    the target object key (to set the ACL on)
// objectAcl    object ACL. Valid options are PrivateACL, PublicReadACL, PublicReadWriteACL.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) SetObjectACL(objectKey string, objectACL ACLType) error {
	options := []Option{ObjectACL(objectACL)}
	params := map[string]interface{}{}
	params["acl"] = nil
	resp, err := bucket.do("PUT", objectKey, params, options, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// GetObjectACL gets object's ACL
//
// objectKey    the object to get ACL from.
//
// GetObjectACLResult    the result object when error is nil. GetObjectACLResult.Acl is the object ACL.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) GetObjectACL(objectKey string) (GetObjectACLResult, error) {
	var out GetObjectACLResult
	params := map[string]interface{}{}
	params["acl"] = nil
	resp, err := bucket.do("GET", objectKey, params, nil, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// PutSymlink creates a symlink (to point to an existing object)
//
// Symlink cannot point to another symlink.
// When creating a symlink, it does not check the existence of the target file, and does not check if the target file is symlink.
// Neither it checks the caller's permission on the target file. All these checks are deferred to the actual GetObject call via this symlink.
// If trying to add an existing file, as long as the caller has the write permission, the existing one will be overwritten.
// If the x-oss-meta- is specified, it will be added as the metadata of the symlink file.
//
// symObjectKey    the symlink object's key.
// targetObjectKey    the target object key to point to.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) PutSymlink(symObjectKey string, targetObjectKey string, options ...Option) error {
	options = append(options, symlinkTarget(url.QueryEscape(targetObjectKey)))
	params := map[string]interface{}{}
	params["symlink"] = nil
	resp, err := bucket.do("PUT", symObjectKey, params, options, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// GetSymlink gets the symlink object with the specified key.
// If the symlink object does not exist, returns 404.
//
// objectKey    the symlink object's key.
//
// error    it's nil if no error, otherwise it's an error object.
//          When error is nil, the target file key is in the X-Oss-Symlink-Target header of the returned object.
//
func (bucket Bucket) GetSymlink(objectKey string) (http.Header, error) {
	params := map[string]interface{}{}
	params["symlink"] = nil
	resp, err := bucket.do("GET", objectKey, params, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	targetObjectKey := resp.Headers.Get(HTTPHeaderOssSymlinkTarget)
	targetObjectKey, err = url.QueryUnescape(targetObjectKey)
	if err != nil {
		return resp.Headers, err
	}
	resp.Headers.Set(HTTPHeaderOssSymlinkTarget, targetObjectKey)
	return resp.Headers, err
}

// RestoreObject restores the object from the archive storage.
//
// An archive object is in cold status by default and it cannot be accessed.
// When restore is called on the cold object, it will become available for access after some time.
// If multiple restores are called on the same file when the object is being restored, server side does nothing for additional calls but returns success.
// By default, the restored object is available for access for one day. After that it will be unavailable again.
// But if another RestoreObject are called after the file is restored, then it will extend one day's access time of that object, up to 7 days.
//
// objectKey    object key to restore.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) RestoreObject(objectKey string) error {
	params := map[string]interface{}{}
	params["restore"] = nil
	resp, err := bucket.do("POST", objectKey, params, nil, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK, http.StatusAccepted})
}

// SignURL signs the URL. Users could access the object directly with this URL without getting the AK.
//
// objectKey    the target object to sign.
// signURLConfig    the configuration for the signed URL
//
// string    returns the signed URL, when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) SignURL(objectKey string, method HTTPMethod, expiredInSec int64, options ...Option) (string, error) {
	if expiredInSec < 0 {
		return "", fmt.Errorf("invalid expires: %d, expires must bigger than 0", expiredInSec)
	}
	expiration := time.Now().Unix() + expiredInSec

	params, err := getRawParams(options)
	if err != nil {
		return "", err
	}

	headers := make(map[string]string)
	err = handleOptions(headers, options)
	if err != nil {
		return "", err
	}

	return bucket.Client.Conn.signURL(method, bucket.BucketName, objectKey, expiration, params, headers), nil
}

// PutObjectWithURL uploads an object with the URL. If the object exists, it will be overwritten.
// PutObjectWithURL It will not generate minetype according to the key name.
//
// signedURL    signed URL.
// reader    io.Reader the read instance for reading the data for the upload.
// options    the options for uploading the data. The valid options are CacheControl, ContentDisposition, ContentEncoding,
//            Expires, ServerSideEncryption, ObjectACL and custom metadata. Check out the following link for details:
//            https://help.aliyun.com/document_detail/oss/api-reference/object/PutObject.html
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) PutObjectWithURL(signedURL string, reader io.Reader, options ...Option) error {
	resp, err := bucket.DoPutObjectWithURL(signedURL, reader, options)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return err
}

// PutObjectFromFileWithURL uploads an object from a local file with the signed URL.
// PutObjectFromFileWithURL It does not generate mimetype according to object key's name or the local file name.
//
// signedURL    the signed URL.
// filePath    local file path, such as dirfile.txt, for uploading.
// options    options for uploading, same as the options in PutObject function.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) PutObjectFromFileWithURL(signedURL, filePath string, options ...Option) error {
	fd, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fd.Close()

	resp, err := bucket.DoPutObjectWithURL(signedURL, fd, options)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return err
}

// DoPutObjectWithURL is the actual API that does the upload with URL work(internal for SDK)
//
// signedURL    the signed URL.
// reader    io.Reader the read instance for getting the data to upload.
// options    options for uploading.
//
// Response    the response object which contains the HTTP response.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DoPutObjectWithURL(signedURL string, reader io.Reader, options []Option) (*Response, error) {
	listener := getProgressListener(options)

	params := map[string]interface{}{}
	resp, err := bucket.doURL("PUT", signedURL, params, options, reader, listener)
	if err != nil {
		return nil, err
	}

	if bucket.getConfig().IsEnableCRC {
		err = checkCRC(resp, "DoPutObjectWithURL")
		if err != nil {
			return resp, err
		}
	}

	err = checkRespCode(resp.StatusCode, []int{http.StatusOK})

	return resp, err
}

// GetObjectWithURL downloads the object and returns the reader instance,  with the signed URL.
//
// signedURL    the signed URL.
// options    options for downloading the object. Valid options are IfModifiedSince, IfUnmodifiedSince, IfMatch,
//            IfNoneMatch, AcceptEncoding. For more information, check out the following link:
//            https://help.aliyun.com/document_detail/oss/api-reference/object/GetObject.html
//
// io.ReadCloser    the reader object for getting the data from response. It needs be closed after the usage. It's only valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) GetObjectWithURL(signedURL string, options ...Option) (io.ReadCloser, error) {
	result, err := bucket.DoGetObjectWithURL(signedURL, options)
	if err != nil {
		return nil, err
	}
	return result.Response, nil
}

// GetObjectToFileWithURL downloads the object into a local file with the signed URL.
//
// signedURL    the signed URL
// filePath    the local file path to download to.
// options    the options for downloading object. Check out the parameter options in function GetObject for the reference.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) GetObjectToFileWithURL(signedURL, filePath string, options ...Option) error {
	tempFilePath := filePath + TempFileSuffix

	// Get the object's content
	result, err := bucket.DoGetObjectWithURL(signedURL, options)
	if err != nil {
		return err
	}
	defer result.Response.Close()

	// If the file does not exist, create one. If exists, then overwrite it.
	fd, err := os.OpenFile(tempFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, FilePermMode)
	if err != nil {
		return err
	}

	// Save the data to the file.
	_, err = io.Copy(fd, result.Response.Body)
	fd.Close()
	if err != nil {
		return err
	}

	// Compare the CRC value. If CRC values do not match, return error.
	hasRange, _, _ := isOptionSet(options, HTTPHeaderRange)
	encodeOpt, _ := findOption(options, HTTPHeaderAcceptEncoding, nil)
	acceptEncoding := ""
	if encodeOpt != nil {
		acceptEncoding = encodeOpt.(string)
	}

	if bucket.getConfig().IsEnableCRC && !hasRange && acceptEncoding != "gzip" {
		result.Response.ClientCRC = result.ClientCRC.Sum64()
		err = checkCRC(result.Response, "GetObjectToFileWithURL")
		if err != nil {
			os.Remove(tempFilePath)
			return err
		}
	}

	return os.Rename(tempFilePath, filePath)
}

// DoGetObjectWithURL is the actual API that downloads the file with the signed URL.
//
// signedURL    the signed URL.
// options    the options for getting object. Check out parameter options in GetObject for the reference.
//
// GetObjectResult    the result object when the error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) DoGetObjectWithURL(signedURL string, options []Option) (*GetObjectResult, error) {
	params, _ := getRawParams(options)
	resp, err := bucket.doURL("GET", signedURL, params, options, nil, nil)
	if err != nil {
		return nil, err
	}

	result := &GetObjectResult{
		Response: resp,
	}

	// CRC
	var crcCalc hash.Hash64
	hasRange, _, _ := isOptionSet(options, HTTPHeaderRange)
	if bucket.getConfig().IsEnableCRC && !hasRange {
		crcCalc = crc64.New(crcTable())
		result.ServerCRC = resp.ServerCRC
		result.ClientCRC = crcCalc
	}

	// Progress
	listener := getProgressListener(options)

	contentLen, _ := strconv.ParseInt(resp.Headers.Get(HTTPHeaderContentLength), 10, 64)
	resp.Body = TeeReader(resp.Body, crcCalc, contentLen, listener, nil)

	return result, nil
}

//
// ProcessObject apply process on the specified image file.
//
// The supported process includes resize, rotate, crop, watermark, format,
// udf, customized style, etc.
//
//
// objectKey	object key to process.
// process	process string, such as "image/resize,w_100|sys/saveas,o_dGVzdC5qcGc,b_dGVzdA"
//
// error    it's nil if no error, otherwise it's an error object.
//
func (bucket Bucket) ProcessObject(objectKey string, process string) (ProcessObjectResult, error) {
	var out ProcessObjectResult
	params := map[string]interface{}{}
	params["x-oss-process"] = nil
	processData := fmt.Sprintf("%v=%v", "x-oss-process", process)
	data := strings.NewReader(processData)
	resp, err := bucket.do("POST", objectKey, params, nil, data, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = jsonUnmarshal(resp.Body, &out)
	return out, err
}

// Private
func (bucket Bucket) do(method, objectName string, params map[string]interface{}, options []Option,
	data io.Reader, listener ProgressListener) (*Response, error) {
	headers := make(map[string]string)
	err := handleOptions(headers, options)
	if err != nil {
		return nil, err
	}
	return bucket.Client.Conn.Do(method, bucket.BucketName, objectName,
		params, headers, data, 0, listener)
}

func (bucket Bucket) doURL(method HTTPMethod, signedURL string, params map[string]interface{}, options []Option,
	data io.Reader, listener ProgressListener) (*Response, error) {
	headers := make(map[string]string)
	err := handleOptions(headers, options)
	if err != nil {
		return nil, err
	}
	return bucket.Client.Conn.DoURL(method, signedURL, headers, data, 0, listener)
}

func (bucket Bucket) getConfig() *Config {
	return bucket.Client.Config
}

func addContentType(options []Option, keys ...string) []Option {
	typ := TypeByExtension("")
	for _, key := range keys {
		typ = TypeByExtension(key)
		if typ != "" {
			break
		}
	}

	if typ == "" {
		typ = "application/octet-stream"
	}

	opts := []Option{ContentType(typ)}
	opts = append(opts, options...)

	return opts
}
