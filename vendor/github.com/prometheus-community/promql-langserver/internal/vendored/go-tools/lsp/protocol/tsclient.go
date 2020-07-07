package protocol

// Package protocol contains data types and code for LSP jsonrpcs
// generated automatically from vscode-languageserver-node
// commit: 151b520c995ee3d76729b5c46258ab273d989726
// last fetched Fri Mar 13 2020 17:02:20 GMT-0400 (Eastern Daylight Time)

// Code generated (see typescript/README.md) DO NOT EDIT.

import (
	"context"
	"encoding/json"

	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/jsonrpc2"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/telemetry/event"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/xcontext"
)

type Client interface {
	ShowMessage(context.Context, *ShowMessageParams) error
	LogMessage(context.Context, *LogMessageParams) error
	Event(context.Context, *interface{}) error
	PublishDiagnostics(context.Context, *PublishDiagnosticsParams) error
	Progress(context.Context, *ProgressParams) error
	WorkspaceFolders(context.Context) ([]WorkspaceFolder /*WorkspaceFolder[] | null*/, error)
	Configuration(context.Context, *ParamConfiguration) ([]interface{}, error)
	WorkDoneProgressCreate(context.Context, *WorkDoneProgressCreateParams) error
	RegisterCapability(context.Context, *RegistrationParams) error
	UnregisterCapability(context.Context, *UnregistrationParams) error
	ShowMessageRequest(context.Context, *ShowMessageRequestParams) (*MessageActionItem /*MessageActionItem | null*/, error)
	ApplyEdit(context.Context, *ApplyWorkspaceEditParams) (*ApplyWorkspaceEditResponse, error)
}

func (h clientHandler) Deliver(ctx context.Context, r *jsonrpc2.Request, delivered bool) bool {
	if delivered {
		return false
	}
	if ctx.Err() != nil {
		ctx := xcontext.Detach(ctx)
		r.Reply(ctx, nil, jsonrpc2.NewErrorf(RequestCancelledError, ""))
		return true
	}
	switch r.Method {
	case "window/showMessage": // notif
		var params ShowMessageParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		if err := h.client.ShowMessage(ctx, &params); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "window/logMessage": // notif
		var params LogMessageParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		if err := h.client.LogMessage(ctx, &params); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "telemetry/event": // notif
		var params interface{}
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		if err := h.client.Event(ctx, &params); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "textDocument/publishDiagnostics": // notif
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		if err := h.client.PublishDiagnostics(ctx, &params); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "$/progress": // notif
		var params ProgressParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		if err := h.client.Progress(ctx, &params); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "workspace/workspaceFolders": // req
		if r.Params != nil {
			r.Reply(ctx, nil, jsonrpc2.NewErrorf(jsonrpc2.CodeInvalidParams, "Expected no params"))
			return true
		}
		resp, err := h.client.WorkspaceFolders(ctx)
		if err := r.Reply(ctx, resp, err); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "workspace/configuration": // req
		var params ParamConfiguration
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		resp, err := h.client.Configuration(ctx, &params)
		if err := r.Reply(ctx, resp, err); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "window/workDoneProgress/create": // req
		var params WorkDoneProgressCreateParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		err := h.client.WorkDoneProgressCreate(ctx, &params)
		if err := r.Reply(ctx, nil, err); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "client/registerCapability": // req
		var params RegistrationParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		err := h.client.RegisterCapability(ctx, &params)
		if err := r.Reply(ctx, nil, err); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "client/unregisterCapability": // req
		var params UnregistrationParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		err := h.client.UnregisterCapability(ctx, &params)
		if err := r.Reply(ctx, nil, err); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "window/showMessageRequest": // req
		var params ShowMessageRequestParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		resp, err := h.client.ShowMessageRequest(ctx, &params)
		if err := r.Reply(ctx, resp, err); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	case "workspace/applyEdit": // req
		var params ApplyWorkspaceEditParams
		if err := json.Unmarshal(*r.Params, &params); err != nil {
			sendParseError(ctx, r, err)
			return true
		}
		resp, err := h.client.ApplyEdit(ctx, &params)
		if err := r.Reply(ctx, resp, err); err != nil {
			event.Error(ctx, "", err)
		}
		return true
	default:
		return false

	}
}

type clientDispatcher struct {
	*jsonrpc2.Conn
}

func (s *clientDispatcher) ShowMessage(ctx context.Context, params *ShowMessageParams) error {
	return s.Conn.Notify(ctx, "window/showMessage", params)
}

func (s *clientDispatcher) LogMessage(ctx context.Context, params *LogMessageParams) error {
	return s.Conn.Notify(ctx, "window/logMessage", params)
}

func (s *clientDispatcher) Event(ctx context.Context, params *interface{}) error {
	return s.Conn.Notify(ctx, "telemetry/event", params)
}

func (s *clientDispatcher) PublishDiagnostics(ctx context.Context, params *PublishDiagnosticsParams) error {
	return s.Conn.Notify(ctx, "textDocument/publishDiagnostics", params)
}

func (s *clientDispatcher) Progress(ctx context.Context, params *ProgressParams) error {
	return s.Conn.Notify(ctx, "$/progress", params)
}
func (s *clientDispatcher) WorkspaceFolders(ctx context.Context) ([]WorkspaceFolder /*WorkspaceFolder[] | null*/, error) {
	var result []WorkspaceFolder /*WorkspaceFolder[] | null*/
	if err := s.Conn.Call(ctx, "workspace/workspaceFolders", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *clientDispatcher) Configuration(ctx context.Context, params *ParamConfiguration) ([]interface{}, error) {
	var result []interface{}
	if err := s.Conn.Call(ctx, "workspace/configuration", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *clientDispatcher) WorkDoneProgressCreate(ctx context.Context, params *WorkDoneProgressCreateParams) error {
	return s.Conn.Call(ctx, "window/workDoneProgress/create", params, nil) // Call, not Notify
}

func (s *clientDispatcher) RegisterCapability(ctx context.Context, params *RegistrationParams) error {
	return s.Conn.Call(ctx, "client/registerCapability", params, nil) // Call, not Notify
}

func (s *clientDispatcher) UnregisterCapability(ctx context.Context, params *UnregistrationParams) error {
	return s.Conn.Call(ctx, "client/unregisterCapability", params, nil) // Call, not Notify
}

func (s *clientDispatcher) ShowMessageRequest(ctx context.Context, params *ShowMessageRequestParams) (*MessageActionItem /*MessageActionItem | null*/, error) {
	var result *MessageActionItem /*MessageActionItem | null*/
	if err := s.Conn.Call(ctx, "window/showMessageRequest", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *clientDispatcher) ApplyEdit(ctx context.Context, params *ApplyWorkspaceEditParams) (*ApplyWorkspaceEditResponse, error) {
	var result *ApplyWorkspaceEditResponse
	if err := s.Conn.Call(ctx, "workspace/applyEdit", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}
