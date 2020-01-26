package server

import (
	"context"
	"net"

	"github.com/golang/glog"
	examples "github.com/grpc-ecosystem/grpc-gateway/examples/proto/examplepb"
	"google.golang.org/grpc"
)

// Run starts the example gRPC service.
// "network" and "address" are passed to net.Listen.
func Run(ctx context.Context, network, address string) error {
	l, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	defer func() {
		if err := l.Close(); err != nil {
			glog.Errorf("Failed to close %s %s: %v", network, address, err)
		}
	}()

	s := grpc.NewServer()
	examples.RegisterEchoServiceServer(s, newEchoServer())
	examples.RegisterFlowCombinationServer(s, newFlowCombinationServer())

	abe := newABitOfEverythingServer()
	examples.RegisterABitOfEverythingServiceServer(s, abe)
	examples.RegisterStreamServiceServer(s, abe)
	examples.RegisterResponseBodyServiceServer(s, newResponseBodyServer())

	go func() {
		defer s.GracefulStop()
		<-ctx.Done()
	}()
	return s.Serve(l)
}
