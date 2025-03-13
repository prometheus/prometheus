package middleware

import (
	"context"
	"mime"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

// metadataCtxKey key for Metadata in context.
type metadataCtxKey struct{}

// ContextWithMetadata append to context Metadata.
func ContextWithMetadata(ctx context.Context, metadata model.Metadata) context.Context {
	return context.WithValue(ctx, metadataCtxKey{}, metadata)
}

// MetadataFromContext extract from context Metadata.
func MetadataFromContext(ctx context.Context) model.Metadata {
	return ctx.Value(metadataCtxKey{}).(model.Metadata)
}

// Middleware function.
type Middleware func(next http.HandlerFunc) http.HandlerFunc

// ResolveMetadata middleware for extract metadata from request.
func ResolveMetadata(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		metadata := extractMetadata(request.Header)
		if metadata == nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		ctx := request.Context()
		metadata.RelabelerID = extractRelabelerIDFromPath(ctx)
		next(writer, request.WithContext(ContextWithMetadata(ctx, *metadata)))
	}
}

// ResolveMetadataFromHeader middleware for extract metadata from request header.
func ResolveMetadataFromHeader(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		metadata := extractMetadata(request.Header)
		if metadata == nil {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		metadata.RelabelerID = extractRelabelerIDFromHeader(request.Header)
		next(writer, request.WithContext(ContextWithMetadata(request.Context(), *metadata)))
	}
}

// ResolveMetadataRemoteWrite middleware for extract metadata from request.
func ResolveMetadataRemoteWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		next(writer, request.WithContext(ContextWithMetadata(
			ctx,
			model.Metadata{RelabelerID: extractRelabelerIDFromPath(ctx)},
		)))
	}
}

// ResolveMetadataRemoteWriteFromHeader middleware for extract metadata from request header.
func ResolveMetadataRemoteWriteFromHeader(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		next(writer, request.WithContext(ContextWithMetadata(
			request.Context(),
			model.Metadata{RelabelerID: extractRelabelerIDFromHeader(request.Header)},
		)))
	}
}

// ResolveMetadataRemoteWriteVanilla middleware for extract metadata from request header.
func ResolveMetadataRemoteWriteVanilla(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		next(writer, request.WithContext(ContextWithMetadata(
			request.Context(),
			model.Metadata{RelabelerID: config.TransparentRelabeler},
		)))
	}
}

// extractMetadata extract metadata from header.
func extractMetadata(header http.Header) *model.Metadata {
	agentUUIDString := header.Get("X-Agent-UUID")
	if agentUUIDString == "" {
		return nil
	}

	agentUUID, err := uuid.Parse(agentUUIDString)
	if err != nil {
		return nil
	}

	productName := header.Get("User-Agent")
	agentHostname := header.Get("X-Agent-Hostname")

	blockIDString := header.Get("X-Block-ID")
	if blockIDString == "" {
		return nil
	}

	blockID, err := uuid.Parse(blockIDString)
	if err != nil {
		return nil
	}

	shardIDStr := header.Get("X-Shard-ID")
	if shardIDStr == "" {
		return nil
	}
	//revive:disable-next-line:add-constant not need const
	shardID, err := strconv.ParseUint(shardIDStr, 10, 0)
	if err != nil {
		return nil
	}

	shardsLogStr := header.Get("X-Shards-Log")
	if shardsLogStr == "" {
		return nil
	}
	//revive:disable-next-line:add-constant not need const
	shardsLog, err := strconv.ParseUint(shardsLogStr, 10, 0)
	if err != nil {
		return nil
	}

	contentType := header.Get("Content-Type")
	if contentType == "" {
		return nil
	}
	mediaType, param, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil
	}

	protocolVersionString, ok := param["version"]
	if !ok {
		return nil
	}
	//revive:disable-next-line:add-constant not need const
	protocolVersion, err := strconv.ParseUint(protocolVersionString, 10, 0)
	if err != nil {
		return nil
	}

	seversionStr, ok := param["segment_encoding_version"]
	if !ok {
		return nil
	}
	//revive:disable-next-line:add-constant not need const
	segmentEncodingVersion, err := strconv.ParseUint(seversionStr, 10, 0)
	if err != nil {
		return nil
	}

	return &model.Metadata{
		BlockID:                blockID,
		ShardID:                uint16(shardID),
		ShardsLog:              uint8(shardsLog),
		SegmentEncodingVersion: uint8(segmentEncodingVersion),
		ProtocolVersion:        uint8(protocolVersion),
		ProductName:            productName,
		AgentHostname:          agentHostname,
		AgentUUID:              agentUUID,
		MediaType:              mediaType,
	}
}

// extractRelabelerIDFromPath extract metadata from path.
func extractRelabelerIDFromPath(ctx context.Context) string {
	return route.Param(ctx, "relabeler_id")
}

// extractRelabelerIDFromHeader extract metadata from Header.
func extractRelabelerIDFromHeader(header http.Header) string {
	return header.Get("relabeler_id")
}
