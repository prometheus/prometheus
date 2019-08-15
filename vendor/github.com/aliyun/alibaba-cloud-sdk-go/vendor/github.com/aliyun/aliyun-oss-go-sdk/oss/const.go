package oss

import "os"

// ACLType bucket/object ACL
type ACLType string

const (
	// ACLPrivate definition : private read and write
	ACLPrivate ACLType = "private"

	// ACLPublicRead definition : public read and private write
	ACLPublicRead ACLType = "public-read"

	// ACLPublicReadWrite definition : public read and public write
	ACLPublicReadWrite ACLType = "public-read-write"

	// ACLDefault Object. It's only applicable for object.
	ACLDefault ACLType = "default"
)

// MetadataDirectiveType specifying whether use the metadata of source object when copying object.
type MetadataDirectiveType string

const (
	// MetaCopy the target object's metadata is copied from the source one
	MetaCopy MetadataDirectiveType = "COPY"

	// MetaReplace the target object's metadata is created as part of the copy request (not same as the source one)
	MetaReplace MetadataDirectiveType = "REPLACE"
)

// StorageClassType bucket storage type
type StorageClassType string

const (
	// StorageStandard standard
	StorageStandard StorageClassType = "Standard"

	// StorageIA infrequent access
	StorageIA StorageClassType = "IA"

	// StorageArchive archive
	StorageArchive StorageClassType = "Archive"
)

// PayerType the type of request payer
type PayerType string

const (
	// Requester the requester who send the request
	Requester PayerType = "requester"
)

// HTTPMethod HTTP request method
type HTTPMethod string

const (
	// HTTPGet HTTP GET
	HTTPGet HTTPMethod = "GET"

	// HTTPPut HTTP PUT
	HTTPPut HTTPMethod = "PUT"

	// HTTPHead HTTP HEAD
	HTTPHead HTTPMethod = "HEAD"

	// HTTPPost HTTP POST
	HTTPPost HTTPMethod = "POST"

	// HTTPDelete HTTP DELETE
	HTTPDelete HTTPMethod = "DELETE"
)

// HTTP headers
const (
	HTTPHeaderAcceptEncoding     string = "Accept-Encoding"
	HTTPHeaderAuthorization             = "Authorization"
	HTTPHeaderCacheControl              = "Cache-Control"
	HTTPHeaderContentDisposition        = "Content-Disposition"
	HTTPHeaderContentEncoding           = "Content-Encoding"
	HTTPHeaderContentLength             = "Content-Length"
	HTTPHeaderContentMD5                = "Content-MD5"
	HTTPHeaderContentType               = "Content-Type"
	HTTPHeaderContentLanguage           = "Content-Language"
	HTTPHeaderDate                      = "Date"
	HTTPHeaderEtag                      = "ETag"
	HTTPHeaderExpires                   = "Expires"
	HTTPHeaderHost                      = "Host"
	HTTPHeaderLastModified              = "Last-Modified"
	HTTPHeaderRange                     = "Range"
	HTTPHeaderLocation                  = "Location"
	HTTPHeaderOrigin                    = "Origin"
	HTTPHeaderServer                    = "Server"
	HTTPHeaderUserAgent                 = "User-Agent"
	HTTPHeaderIfModifiedSince           = "If-Modified-Since"
	HTTPHeaderIfUnmodifiedSince         = "If-Unmodified-Since"
	HTTPHeaderIfMatch                   = "If-Match"
	HTTPHeaderIfNoneMatch               = "If-None-Match"

	HTTPHeaderOssACL                         = "X-Oss-Acl"
	HTTPHeaderOssMetaPrefix                  = "X-Oss-Meta-"
	HTTPHeaderOssObjectACL                   = "X-Oss-Object-Acl"
	HTTPHeaderOssSecurityToken               = "X-Oss-Security-Token"
	HTTPHeaderOssServerSideEncryption        = "X-Oss-Server-Side-Encryption"
	HTTPHeaderOssServerSideEncryptionKeyID   = "X-Oss-Server-Side-Encryption-Key-Id"
	HTTPHeaderOssCopySource                  = "X-Oss-Copy-Source"
	HTTPHeaderOssCopySourceRange             = "X-Oss-Copy-Source-Range"
	HTTPHeaderOssCopySourceIfMatch           = "X-Oss-Copy-Source-If-Match"
	HTTPHeaderOssCopySourceIfNoneMatch       = "X-Oss-Copy-Source-If-None-Match"
	HTTPHeaderOssCopySourceIfModifiedSince   = "X-Oss-Copy-Source-If-Modified-Since"
	HTTPHeaderOssCopySourceIfUnmodifiedSince = "X-Oss-Copy-Source-If-Unmodified-Since"
	HTTPHeaderOssMetadataDirective           = "X-Oss-Metadata-Directive"
	HTTPHeaderOssNextAppendPosition          = "X-Oss-Next-Append-Position"
	HTTPHeaderOssRequestID                   = "X-Oss-Request-Id"
	HTTPHeaderOssCRC64                       = "X-Oss-Hash-Crc64ecma"
	HTTPHeaderOssSymlinkTarget               = "X-Oss-Symlink-Target"
	HTTPHeaderOssStorageClass                = "X-Oss-Storage-Class"
	HTTPHeaderOssCallback                    = "X-Oss-Callback"
	HTTPHeaderOssCallbackVar                 = "X-Oss-Callback-Var"
	HTTPHeaderOSSRequester                   = "X-Oss-Request-Payer"
)

// HTTP Param
const (
	HTTPParamExpires       = "Expires"
	HTTPParamAccessKeyID   = "OSSAccessKeyId"
	HTTPParamSignature     = "Signature"
	HTTPParamSecurityToken = "security-token"
	HTTPParamPlaylistName  = "playlistName"
)

// Other constants
const (
	MaxPartSize = 5 * 1024 * 1024 * 1024 // Max part size, 5GB
	MinPartSize = 100 * 1024             // Min part size, 100KB

	FilePermMode = os.FileMode(0664) // Default file permission

	TempFilePrefix = "oss-go-temp-" // Temp file prefix
	TempFileSuffix = ".temp"        // Temp file suffix

	CheckpointFileSuffix = ".cp" // Checkpoint file suffix

	Version = "1.9.5" // Go SDK version
)
