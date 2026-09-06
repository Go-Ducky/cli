package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/Go-Ducky/cli/internal/agent/tools"
)

const protocolVersion = "2024-11-05"

type Server struct {
	workDir string
	reg     *tools.Registry
	version string
}

func New(workDir string, reg *tools.Registry, version string) *Server {
	return &Server{workDir: workDir, reg: reg, version: version}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) Run(ctx context.Context, r io.Reader, w io.Writer) error {
	enc := json.NewEncoder(w)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 256*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		isNotification := len(req.ID) == 0 || string(req.ID) == "null"
		result, err := s.dispatch(ctx, req.Method, req.Params)
		if isNotification {
			continue
		}
		if err := enc.Encode(response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: err}); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]string{"name": "goducky", "version": s.version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		list := make([]map[string]any, 0, s.reg.Size())
		for _, t := range s.reg.All() {
			schema := t.Parameters()
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			list = append(list, map[string]any{
				"name":        t.Name(),
				"description": t.Description(),
				"inputSchema": schema,
			})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
		t, ok := s.reg.Get(p.Name)
		if !ok {
			return nil, &rpcError{Code: -32602, Message: "unknown tool: " + p.Name}
		}
		tctx := &tools.Context{
			WorkDir:  s.workDir,
			Approval: func(string, map[string]any) bool { return true },
			OnLog:    func(string) {},
		}
		res, err := t.Execute(ctx, tctx, p.Arguments)
		if err != nil {
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": "tool error: " + err.Error()}},
				"isError": true,
			}, nil
		}
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": res.Content}},
			"isError": res.IsError,
		}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + method}
	}
}
