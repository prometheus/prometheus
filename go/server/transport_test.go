package server_test

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/server"
	"github.com/prometheus/prometheus/pp/go/transport"
)

func ExampleTCPReader() {
	ctx := context.Background()

	handleStream := func(ctx context.Context, fe *frames.ReadFrame, tcpReader *server.TCPReader) {
		reader := server.NewProtocolReader(server.StartWith(tcpReader, fe))
		defer reader.Destroy()
		for {
			rq, err := reader.Next(ctx)
			if err != nil {
				log.Printf("fail to read next message: %s", err)
				return
			}
			_ = rq // process data
			if err := tcpReader.SendResponse(ctx, &frames.ResponseMsg{
				Text:      "OK",
				Code:      200,
				SegmentID: rq.SegmentID,
				SendAt:    rq.SentAt,
			}); err != nil {
				log.Printf("fail to send response: %s", err)
				return
			}
		}
	}

	handleRefill := func(ctx context.Context, fe *frames.ReadFrame, tcpReader *server.TCPReader) {
		// write in file until all segments have been read
		// make FileReader
		// make ProtocolReader over FileReader
		// make BlockWriter
		// read until EOF from ProtocolReader and append to BlockWriter
		// save BlockWriter
		// send block to S3
		if err := tcpReader.SendResponse(ctx, &frames.ResponseMsg{
			Text: "OK",
			Code: 200,
		}); err != nil {
			log.Printf("fail to send response: %s", err)
		}
	}

	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", "")
	if err != nil {
		panic(err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		go func(conn net.Conn) {
			defer conn.Close()
			tcpReader := server.NewTCPReader(&transport.Config{
				ReadTimeout:  time.Second,
				WriteTimeout: time.Second,
			}, conn)
			auth, err := tcpReader.Authorization(ctx)
			if err != nil {
				log.Printf("fail to read auth message: %s", err)
				return
			}
			if auth.Token != "111" {
				log.Printf("invalid authorization token: %s", auth.Token)
				return
			}
			msg, err := tcpReader.Next(ctx)
			if err != nil {
				log.Printf("fail to read next message: %s", err)
				return
			}
			if msg.GetType() == frames.RefillType {
				handleRefill(ctx, msg, tcpReader)
			} else {
				handleStream(ctx, msg, tcpReader)
			}

		}(conn)
	}
}
