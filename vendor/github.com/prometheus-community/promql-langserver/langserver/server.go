// Copyright 2019 Tobias Guggenmos
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This File includes code from the go/tools project which is governed by the following license:
// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package langserver

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"

	promClient "github.com/prometheus-community/promql-langserver/prometheus"

	"github.com/go-kit/kit/log"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/jsonrpc2"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus-community/promql-langserver/langserver/cache"
)

// Server wraps language server instance that can connect to exactly one client.
type Server struct {
	server *server
}

// HeadlessServer is a modified Server interface that is used by the REST API.
type HeadlessServer interface {
	protocol.Server
	GetDiagnostics(uri protocol.DocumentURI) (*protocol.PublishDiagnosticsParams, error)
}

// server is a language server instance that can connect to exactly one client.
type server struct {
	Conn   *jsonrpc2.Conn
	client protocol.Client

	state   serverState
	stateMu sync.Mutex

	cache cache.DocumentCache

	config *Config

	prometheusClient promClient.Client

	lifetime context.Context
	exit     func()
	headless bool
}

type serverState int

const (
	serverCreated = serverState(iota)
	serverInitializing
	serverInitialized // set once the server has received "initialized" request
	serverShutDown
)

// Run starts the language server instance.
func (s Server) Run() error {
	return s.server.Conn.Run(s.server.lifetime)
}

// CreateHeadlessServer creates a locked down server instance for the REST API.
//
// "locked down" in this case means, that the instance cannot send or receive any JSONRPC communication. Logging messages that the instance tries to send over JSONRPC are redirected to stderr.
func CreateHeadlessServer(ctx context.Context, prometheusClient promClient.Client, logger log.Logger) (HeadlessServer, error) {
	s := &server{
		client:           &headlessClient{logger: logger},
		headless:         true,
		config:           &Config{PrometheusURL: prometheusClient.GetURL()},
		prometheusClient: prometheusClient,
	}

	s.lifetime, s.exit = context.WithCancel(ctx)

	if _, err := s.Initialize(ctx, &protocol.ParamInitialize{}); err != nil {
		return nil, err
	}

	if err := s.Initialized(ctx, &protocol.InitializedParams{}); err != nil {
		return nil, err
	}

	return s, nil
}

// ServerFromStream generates a Server from a jsonrpc2.Stream.
func ServerFromStream(ctx context.Context, stream jsonrpc2.Stream, config *Config) (context.Context, Server) {
	s := &server{}

	switch config.RPCTrace {
	case "text":
		stream = protocol.LoggingStream(stream, os.Stderr)
	case "json":
		stream = jSONLogStream(stream, os.Stderr)
	}

	s.Conn = jsonrpc2.NewConn(stream)
	s.client = protocol.ClientDispatcher(s.Conn)

	s.Conn.AddHandler(protocol.ServerHandler(s))

	s.config = config

	s.lifetime, s.exit = context.WithCancel(ctx)

	prometheusClient, err := promClient.NewClient("")
	if err != nil {
		// nolint: errcheck
		s.client.ShowMessage(s.lifetime, &protocol.ShowMessageParams{
			Type:    protocol.Error,
			Message: fmt.Sprintf("Failed to inialized the prometheus client\n\n%s ", err.Error()),
		})
		panic(err)
	}
	s.prometheusClient = prometheusClient

	return ctx, Server{s}
}

func (s *server) connectPrometheus(url string) error {
	if err := s.prometheusClient.ChangeDataSource(url); err != nil {
		// nolint: errcheck
		s.client.ShowMessage(s.lifetime, &protocol.ShowMessageParams{
			Type:    protocol.Error,
			Message: fmt.Sprintf("Failed to connect to Prometheus at %s:\n\n%s ", url, err.Error()),
		})
		return err
	}
	return nil
}

// RunTCPServer generates a server listening on the provided TCP Address, creating a new language Server
// instance using plain HTTP for every connection.
func RunTCPServer(ctx context.Context, addr string, config *Config) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		go ServerFromStream(ctx, jsonrpc2.NewHeaderStream(conn, conn), config)
	}
}

// StdioServer generates a Server instance talking to stdio.
func StdioServer(ctx context.Context, config *Config) (context.Context, Server) {
	stream := jsonrpc2.NewHeaderStream(os.Stdin, os.Stdout)
	return ServerFromStream(ctx, stream, config)
}
