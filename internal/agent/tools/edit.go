package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type EditTool struct{ base }

func NewEdit() *EditTool {
	return &EditTool{base: base{
		name:        "edit",
		description: "Replace a specific block of text in a file with new content. Require 'old_string' (text already present) and 'new_string' (replacement). Use for targeted edits without rewriting the whole file. If old_string appears multiple times, provide more context to disambiguate.",
		params: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": {"type": "string", "description": "Path of the file to edit."},
				"old_string": {"type": "string", "description": "Exact text currently in the file to replace."},
				"new_string": {"type": "string", "description": "Text to replace the old_string with."},
				"replace_all": {"type": "boolean", "description": "If true, replace every occurrence of old_string (optional)."}
			},
			"required": ["file_path", "old_string", "new_string"]
		}`),
	}}
}

type editArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func (t *EditTool) Execute(ctx context.Context, tctx *Context, raw json.RawMessage) (*Result, error) {
	var args editArgs
	if err := ParseArgs(raw, &args); err != nil {
		return nil, err
	}
	if args.OldString == "" {
		return &Result{Content: "old_string cannot be empty.", IsError: true}, nil
	}
	if !tctx.Approval("edit file", map[string]any{"file": args.FilePath}) {
		return &Result{Content: "User denied the edit operation.", IsError: true}, nil
	}
	path, err := resolveAbs(tctx.WorkDir, args.FilePath)
	if err != nil {
		return &Result{Content: err.Error(), IsError: true}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return &Result{Content: "Error reading file: " + err.Error(), IsError: true}, nil
	}
	content := string(data)

	oldN := strings.ReplaceAll(args.OldString, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r\n", "\n")

	var newContent string
	occurrences := strings.Count(content, oldN)
	if occurrences == 0 {
		return &Result{Content: "old_string not found in the file.", IsError: true}, nil
	}
	if occurrences > 1 && !args.ReplaceAll {
		return &Result{Content: "old_string found " + itoa(occurrences) + " times. Provide more surrounding context or set replace_all=true."}, nil
	}

	newString := strings.ReplaceAll(args.NewString, "\r\n", "\n")
	if args.ReplaceAll {
		newContent = strings.ReplaceAll(content, oldN, newString)
	} else {
		newContent = strings.Replace(content, oldN, newString, 1)
	}

	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return &Result{Content: "Error writing file: " + err.Error(), IsError: true}, nil
	}
	rel := path
	if wd := tctx.WorkDir; wd != "" {
		if r, err := filepath.Rel(wd, path); err == nil {
			rel = r
		}
	}
	return &Result{Content: "Updated " + rel + " (" + itoa(occurrences) + " occurrence(s) replaced)."}, nil
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
