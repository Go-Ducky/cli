package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// WriteTool creates or overwrites a file.
type WriteTool struct{ base }

// NewWrite creates the write tool.
func NewWrite() *WriteTool {
	return &WriteTool{base: base{
		name:        "write",
		description: "Write content to a file, creating it (and parent directories) if it does not exist. Overwrites the entire file. Use this to create new files or replace entire file contents.",
		params: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": {"type": "string", "description": "Path of the file to write."},
				"content": {"type": "string", "description": "The full content to write to the file."}
			},
			"required": ["file_path", "content"]
		}`),
	}}
}

type writeArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (t *WriteTool) Execute(ctx context.Context, tctx *Context, raw json.RawMessage) (*Result, error) {
	var args writeArgs
	if err := ParseArgs(raw, &args); err != nil {
		return nil, err
	}
	if !tctx.Approval("write file", map[string]any{"file": args.FilePath}) {
		return &Result{Content: "User denied the write operation.", IsError: true}, nil
	}
	path, err := resolveAbs(tctx.WorkDir, args.FilePath)
	if err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &Result{Content: "Error creating directory: " + err.Error(), IsError: true}, nil
	}

	// Normalize CRLF -> LF for consistency across platforms.
	content := strings.ReplaceAll(args.Content, "\r\n", "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return &Result{Content: "Error writing file: " + err.Error(), IsError: true}, nil
	}
	return &Result{Content: "File written successfully: " + path}, nil
}
