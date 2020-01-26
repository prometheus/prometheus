package server

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/empty"
	examples "github.com/grpc-ecosystem/grpc-gateway/examples/proto/examplepb"
	"github.com/grpc-ecosystem/grpc-gateway/examples/proto/sub"
	"github.com/grpc-ecosystem/grpc-gateway/examples/proto/sub2"
	"github.com/rogpeppe/fastuuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Implements of ABitOfEverythingServiceServer

var uuidgen = fastuuid.MustNewGenerator()

type _ABitOfEverythingServer struct {
	v map[string]*examples.ABitOfEverything
	m sync.Mutex
}

type ABitOfEverythingServer interface {
	examples.ABitOfEverythingServiceServer
	examples.StreamServiceServer
}

func newABitOfEverythingServer() ABitOfEverythingServer {
	return &_ABitOfEverythingServer{
		v: make(map[string]*examples.ABitOfEverything),
	}
}

func (s *_ABitOfEverythingServer) Create(ctx context.Context, msg *examples.ABitOfEverything) (*examples.ABitOfEverything, error) {
	s.m.Lock()
	defer s.m.Unlock()

	glog.Info(msg)
	var uuid string
	for {
		uuid = fmt.Sprintf("%x", uuidgen.Next())
		if _, ok := s.v[uuid]; !ok {
			break
		}
	}
	s.v[uuid] = msg
	s.v[uuid].Uuid = uuid
	glog.Infof("%v", s.v[uuid])
	return s.v[uuid], nil
}

func (s *_ABitOfEverythingServer) CreateBody(ctx context.Context, msg *examples.ABitOfEverything) (*examples.ABitOfEverything, error) {
	return s.Create(ctx, msg)
}

func (s *_ABitOfEverythingServer) BulkCreate(stream examples.StreamService_BulkCreateServer) error {
	ctx := stream.Context()

	if header, ok := metadata.FromIncomingContext(ctx); ok {
		if v, ok := header["error"]; ok {
			return status.Errorf(codes.InvalidArgument, "error metadata: %v", v)
		}
	}

	count := 0
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
		glog.Error(msg)
		if _, err = s.Create(ctx, msg); err != nil {
			return err
		}
	}

	err := stream.SendHeader(metadata.New(map[string]string{
		"count": fmt.Sprintf("%d", count),
	}))
	if err != nil {
		return nil
	}

	stream.SetTrailer(metadata.New(map[string]string{
		"foo": "foo2",
		"bar": "bar2",
	}))
	return stream.SendAndClose(new(empty.Empty))
}

func (s *_ABitOfEverythingServer) Lookup(ctx context.Context, msg *sub2.IdMessage) (*examples.ABitOfEverything, error) {
	s.m.Lock()
	defer s.m.Unlock()
	glog.Info(msg)

	err := grpc.SendHeader(ctx, metadata.New(map[string]string{
		"uuid": msg.Uuid,
	}))
	if err != nil {
		return nil, err
	}

	if a, ok := s.v[msg.Uuid]; ok {
		return a, nil
	}

	grpc.SetTrailer(ctx, metadata.New(map[string]string{
		"foo": "foo2",
		"bar": "bar2",
	}))
	return nil, status.Errorf(codes.NotFound, "not found")
}

func (s *_ABitOfEverythingServer) List(_ *empty.Empty, stream examples.StreamService_ListServer) error {
	s.m.Lock()
	defer s.m.Unlock()

	err := stream.SendHeader(metadata.New(map[string]string{
		"count": fmt.Sprintf("%d", len(s.v)),
	}))
	if err != nil {
		return nil
	}

	for _, msg := range s.v {
		if err := stream.Send(msg); err != nil {
			return err
		}
	}

	// return error when metadata includes error header
	if header, ok := metadata.FromIncomingContext(stream.Context()); ok {
		if v, ok := header["error"]; ok {
			stream.SetTrailer(metadata.New(map[string]string{
				"foo": "foo2",
				"bar": "bar2",
			}))
			return status.Errorf(codes.InvalidArgument, "error metadata: %v", v)
		}
	}
	return nil
}

func (s *_ABitOfEverythingServer) Update(ctx context.Context, msg *examples.ABitOfEverything) (*empty.Empty, error) {
	s.m.Lock()
	defer s.m.Unlock()

	glog.Info(msg)
	if _, ok := s.v[msg.Uuid]; ok {
		s.v[msg.Uuid] = msg
	} else {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	return new(empty.Empty), nil
}

func (s *_ABitOfEverythingServer) UpdateV2(ctx context.Context, msg *examples.UpdateV2Request) (*empty.Empty, error) {
	glog.Info(msg)
	// If there is no update mask do a regular update
	if msg.UpdateMask == nil || len(msg.UpdateMask.GetPaths()) == 0 {
		return s.Update(ctx, msg.Abe)
	}

	s.m.Lock()
	defer s.m.Unlock()
	if a, ok := s.v[msg.Abe.Uuid]; ok {
		applyFieldMask(a, msg.Abe, msg.UpdateMask)
	} else {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	return new(empty.Empty), nil
}

// PatchWithFieldMaskInBody differs from UpdateV2 only in that this method exposes the field mask in the request body,
// so that clients can specify their mask explicitly
func (s *_ABitOfEverythingServer) PatchWithFieldMaskInBody(ctx context.Context, request *examples.UpdateV2Request) (*empty.Empty, error) {
	// low-effort attempt to modify the field mask to only include paths for the ABE struct. Since this is only for the
	// integration tests, this narrow implementaion is fine.
	if request.UpdateMask != nil {
		var shifted []string
		for _, path := range request.UpdateMask.GetPaths() {
			shifted = append(shifted, strings.TrimPrefix(path, "Abe."))
		}
		request.UpdateMask.Paths = shifted
	}

	return s.UpdateV2(ctx, request)
}

func (s *_ABitOfEverythingServer) Delete(ctx context.Context, msg *sub2.IdMessage) (*empty.Empty, error) {
	s.m.Lock()
	defer s.m.Unlock()

	glog.Info(msg)
	if _, ok := s.v[msg.Uuid]; ok {
		delete(s.v, msg.Uuid)
	} else {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	return new(empty.Empty), nil
}

func (s *_ABitOfEverythingServer) GetQuery(ctx context.Context, msg *examples.ABitOfEverything) (*empty.Empty, error) {
	s.m.Lock()
	defer s.m.Unlock()

	glog.Info(msg)
	if _, ok := s.v[msg.Uuid]; ok {
		s.v[msg.Uuid] = msg
	} else {
		return nil, status.Errorf(codes.NotFound, "not found")
	}
	return new(empty.Empty), nil
}

func (s *_ABitOfEverythingServer) GetRepeatedQuery(ctx context.Context, msg *examples.ABitOfEverythingRepeated) (*examples.ABitOfEverythingRepeated, error) {
	s.m.Lock()
	defer s.m.Unlock()

	glog.Info(msg)
	return msg, nil
}

func (s *_ABitOfEverythingServer) Echo(ctx context.Context, msg *sub.StringMessage) (*sub.StringMessage, error) {
	s.m.Lock()
	defer s.m.Unlock()

	glog.Info(msg)
	return msg, nil
}

func (s *_ABitOfEverythingServer) BulkEcho(stream examples.StreamService_BulkEchoServer) error {
	var msgs []*sub.StringMessage
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		msgs = append(msgs, msg)
	}

	hmd := metadata.New(map[string]string{
		"foo": "foo1",
		"bar": "bar1",
	})
	if err := stream.SendHeader(hmd); err != nil {
		return err
	}

	for _, msg := range msgs {
		glog.Info(msg)
		if err := stream.Send(msg); err != nil {
			return err
		}
	}

	stream.SetTrailer(metadata.New(map[string]string{
		"foo": "foo2",
		"bar": "bar2",
	}))
	return nil
}

func (s *_ABitOfEverythingServer) DeepPathEcho(ctx context.Context, msg *examples.ABitOfEverything) (*examples.ABitOfEverything, error) {
	s.m.Lock()
	defer s.m.Unlock()

	glog.Info(msg)
	return msg, nil
}

func (s *_ABitOfEverythingServer) NoBindings(ctx context.Context, msg *duration.Duration) (*empty.Empty, error) {
	return nil, nil
}

func (s *_ABitOfEverythingServer) Timeout(ctx context.Context, msg *empty.Empty) (*empty.Empty, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *_ABitOfEverythingServer) ErrorWithDetails(ctx context.Context, msg *empty.Empty) (*empty.Empty, error) {
	stat, err := status.New(codes.Unknown, "with details").
		WithDetails(proto.Message(
			&errdetails.DebugInfo{
				StackEntries: []string{"foo:1"},
				Detail:       "error debug details",
			},
		))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unexpected error adding details: %s", err)
	}
	return nil, stat.Err()
}

func (s *_ABitOfEverythingServer) GetMessageWithBody(ctx context.Context, msg *examples.MessageWithBody) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

func (s *_ABitOfEverythingServer) PostWithEmptyBody(ctx context.Context, msg *examples.Body) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

func (s *_ABitOfEverythingServer) CheckGetQueryParams(ctx context.Context, msg *examples.ABitOfEverything) (*examples.ABitOfEverything, error) {
	return msg, nil
}

func (s *_ABitOfEverythingServer) CheckPostQueryParams(ctx context.Context, msg *examples.ABitOfEverything) (*examples.ABitOfEverything, error) {
	return msg, nil
}
