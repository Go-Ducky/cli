package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReadTool struct{ base }

func NewRead() *ReadTool {
	return &ReadTool{base: base{
		name:        "read",
		description: "Read the contents of a file. Use this to inspect source code, configs, or any text file. Returns the file content, or an error if the file is binary or too large.",
		params: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": {"type": "string", "description": "Absolute path or path relative to the working directory of the file to read."},
				"offset": {"type": "integer", "description": "0-based line number to start reading from (optional)."},
				"limit": {"type": "integer", "description": "Maximum number of lines to read (optional, default all)."}
			},
			"required": ["file_path"]
		}`),
	}}
}

type readArgs struct {
	FilePath string `json:"file_path"`
	Offset   *int   `json:"offset"`
	Limit    *int   `json:"limit"`
}

func (t *ReadTool) Execute(ctx context.Context, tctx *Context, raw json.RawMessage) (*Result, error) {
	var args readArgs
	if err := ParseArgs(raw, &args); err != nil {
		return nil, err
	}
	path, err := resolveAbs(tctx.WorkDir, args.FilePath)
	if err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Error: %v", err), IsError: true}, nil
	}
	if info.Size() > 5*1024*1024 {
		return &Result{Content: fmt.Sprintf("File too large to read (%d bytes). Use grep to search it instead.", info.Size()), IsError: true}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return &Result{Content: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	if hasBinaryBytes(data) {
		return &Result{Content: fmt.Sprintf("File %q appears to be binary and cannot be read as text.", path), IsError: true}, nil
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	start := 0
	if args.Offset != nil && *args.Offset > 0 {
		start = *args.Offset
		if start >= len(lines) {
			return &Result{Content: fmt.Sprintf("Offset %d is beyond the end of the file (%d lines).", start, len(lines)), IsError: true}, nil
		}
	}
	end := len(lines)
	if args.Limit != nil && *args.Limit > 0 {
		end = start + *args.Limit
		if end > len(lines) {
			end = len(lines)
		}
	}

	out := strings.Join(lines[start:end], "\n")
	header := fmt.Sprintf("File: %s (lines %d-%d of %d)", path, start+1, end, len(lines))
	return &Result{Content: header + "\n" + out}, nil
}

func hasBinaryBytes(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	for _, c := range b[:n] {
		if c == 0 {
			return true
		}
	}
	return false
}

func resolveAbs(workDir, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Abs(filepath.Join(workDir, p))
}
