// Package mcpserver implements a minimal Model Context Protocol server over
// stdio (newline-delimited JSON-RPC 2.0). It exposes merger's offline analysis
// as agent tools (merger_scan, merger_validate) so an agent can classify a diff
// and preview its merge lane without the control-plane services.
package mcpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/devr-tools/merger/internal/version"
)

type Server struct {
	initialized bool
	tools       []Definition
}

func New() *Server {
	return &Server{tools: Definitions()}
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	for {
		// ReadBytes can return data together with io.EOF when the final request
		// has no trailing newline (common for one-shot piped clients); process
		// the line before acting on the error so that request is answered
		// instead of silently dropped.
		line, readErr := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			if resp := s.HandleLine(ctx, line); resp != nil {
				if err := writeMessage(out, resp); err != nil {
					return err
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

func (s *Server) HandleLine(ctx context.Context, line []byte) *responseEnvelope {
	var req requestEnvelope
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nil, jsonRPCParseError, "Parse error", nil)
	}
	return s.HandleRequest(ctx, req)
}

func (s *Server) HandleRequest(ctx context.Context, req requestEnvelope) *responseEnvelope {
	if req.JSONRPC != "2.0" {
		return errorResponse(req.ID, jsonRPCInvalidRequest, "invalid JSON-RPC version", nil)
	}

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		s.initialized = true
		return nil
	case "ping":
		return successResponse(req.ID, map[string]any{})
	case "tools/list":
		if errResp := s.requireInitialized(req.ID); errResp != nil {
			return errResp
		}
		return successResponse(req.ID, map[string]any{"tools": s.tools})
	case "tools/call":
		if errResp := s.requireInitialized(req.ID); errResp != nil {
			return errResp
		}
		return s.handleToolCall(ctx, req)
	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			return nil
		}
		return errorResponse(req.ID, jsonRPCMethodNotFound, fmt.Sprintf("method not found: %s", req.Method), nil)
	}
}

func (s *Server) handleInitialize(req requestEnvelope) *responseEnvelope {
	var params initializeParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errorResponse(req.ID, jsonRPCInvalidParams, "invalid initialize params", nil)
		}
	}

	negotiatedVersion := protocolVersion
	if strings.TrimSpace(params.ProtocolVersion) == protocolVersion {
		negotiatedVersion = params.ProtocolVersion
	}

	return successResponse(req.ID, map[string]any{
		"protocolVersion": negotiatedVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]any{
			"name":    "merger",
			"title":   "merger MCP Server",
			"version": version.Number,
		},
		"instructions": "Use merger_scan to analyze a unified diff and get its Change Packet and merge lane. Pass that packet to merger_explain, merger_plan_evidence, or merger_check_readiness to plan and verify an agent's next actions. Use merger_validate to check a repository's merger config and policy.",
	})
}

func (s *Server) handleToolCall(ctx context.Context, req requestEnvelope) *responseEnvelope {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, jsonRPCInvalidParams, "invalid tool call params", nil)
	}
	if strings.TrimSpace(params.Name) == "" {
		return errorResponse(req.ID, jsonRPCInvalidParams, "tool name is required", nil)
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}

	result, err := safeToolCall(ctx, params.Name, params.Arguments)
	if err != nil {
		// Per the MCP spec: an unknown tool is a protocol-level invalid-params
		// error and a panic is a genuine server fault, but ordinary execution
		// failures (bad config path, invalid arguments) are returned as isError
		// results so the calling model can read them and self-correct instead
		// of the client treating the server as broken.
		switch {
		case errors.Is(err, ErrUnknownTool):
			return errorResponse(req.ID, jsonRPCInvalidParams, err.Error(), nil)
		case errors.Is(err, errToolPanicked):
			return errorResponse(req.ID, jsonRPCInternalError, err.Error(), nil)
		default:
			return successResponse(req.ID, Result{
				Content: []Content{{Type: "text", Text: err.Error()}},
				IsError: true,
			})
		}
	}
	return successResponse(req.ID, result)
}

// errToolPanicked marks a contained tool-handler panic so it surfaces as an
// internal JSON-RPC error rather than an isError tool result.
var errToolPanicked = errors.New("tool handler panicked")

// safeToolCall contains a panicking tool handler: the server speaks stdio, so
// an uncaught panic would kill the whole process and every session with it.
func safeToolCall(ctx context.Context, name string, args map[string]any) (result Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: tool %s panicked: %v", errToolPanicked, name, r)
		}
	}()
	return Call(ctx, name, args)
}

func (s *Server) requireInitialized(id json.RawMessage) *responseEnvelope {
	if s.initialized {
		return nil
	}
	return errorResponse(id, jsonRPCServerError, "server not initialized", nil)
}

// Run serves the MCP protocol over stdin/stdout until EOF.
func Run(ctx context.Context) error {
	return New().Serve(ctx, os.Stdin, os.Stdout)
}
