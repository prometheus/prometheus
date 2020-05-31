// Copyright 2020 Tobias Guggenmos
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

package langserver

import (
	"context"
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
)

type headlessClient struct{ logger log.Logger }

func (c *headlessClient) log(typ protocol.MessageType, msg string) error {
	var logLevel func(log.Logger) log.Logger

	switch typ {
	case protocol.Error:
		logLevel = level.Error
	case protocol.Warning:
		logLevel = level.Warn
	case protocol.Info:
		logLevel = level.Info
	case protocol.Log:
		logLevel = level.Debug
	default:
		if err := level.Error(c.logger).Log("msg", "Message with unknown log level: "+msg); err != nil {
			return err
		}
		return fmt.Errorf("unknown log level %f", typ)
	}

	return logLevel(c.logger).Log("msg", msg)
}

func (c *headlessClient) ShowMessage(_ context.Context, params *protocol.ShowMessageParams) error {
	return c.log(params.Type, params.Message)
}

func (c *headlessClient) LogMessage(_ context.Context, params *protocol.LogMessageParams) error {
	return c.log(params.Type, params.Message)
}

func (*headlessClient) Event(_ context.Context, _ *interface{}) error {
	// ignore
	return nil
}

func (*headlessClient) PublishDiagnostics(_ context.Context, _ *protocol.PublishDiagnosticsParams) error {
	// ignore
	return nil
}

func (*headlessClient) WorkspaceFolders(_ context.Context) ([]protocol.WorkspaceFolder, error) {
	// ignore
	return nil, nil
}

func (*headlessClient) Configuration(_ context.Context, _ *protocol.ParamConfiguration) ([]interface{}, error) {
	// ignore
	return nil, nil
}

func (*headlessClient) RegisterCapability(_ context.Context, _ *protocol.RegistrationParams) error {
	// ignore
	return nil
}

func (*headlessClient) UnregisterCapability(_ context.Context, _ *protocol.UnregistrationParams) error {
	// ignore
	return nil
}

func (*headlessClient) ShowMessageRequest(_ context.Context, _ *protocol.ShowMessageRequestParams) (*protocol.MessageActionItem, error) {
	// ignore
	return nil, nil
}

func (*headlessClient) ApplyEdit(_ context.Context, _ *protocol.ApplyWorkspaceEditParams) (*protocol.ApplyWorkspaceEditResponse, error) {
	// ignore
	return nil, nil
}

func (*headlessClient) Progress(_ context.Context, _ *protocol.ProgressParams) error {
	// ignore
	return nil
}

func (*headlessClient) WorkDoneProgressCreate(_ context.Context, _ *protocol.WorkDoneProgressCreateParams) error {
	// ignore
	return nil
}
