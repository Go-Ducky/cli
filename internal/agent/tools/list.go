package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ListTool struct{ base }

func NewList() *ListTool {
	return &ListTool{base: base{
		name:        "list",
		description: "List files and directories in a directory. Use to explore the project structure. Returns names plus size/modified info for files.",
		params: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "Directory to list (default: working directory)."},
				"recursive": {"type": "boolean", "description": "If true, list recursively (optional, default false)."}
			}
		}`),
	}}
}

type listArgs struct {
	Path      string `json:"path"`
	Recursive *bool  `json:"recursive"`
}

func (t *ListTool) Execute(ctx context.Context, tctx *Context, raw json.RawMessage) (*Result, error) {
	var args listArgs
	if err := ParseArgs(raw, &args); err != nil {
		return nil, err
	}
	base := tctx.WorkDir
	if args.Path != "" {
		p, err := resolveAbs(tctx.WorkDir, args.Path)
		if err != nil {
			return &Result{Content: err.Error(), IsError: true}, nil
		}
		base = p
	}

	recursive := false
	if args.Recursive != nil {
		recursive = *args.Recursive
	}

	var sb strings.Builder
	if recursive {
		excluded := map[string]bool{}
		for _, d := range excludedDefaults {
			excluded[d] = true
		}
		sb.WriteString("Recursive listing of " + base + ":\n")
		filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if path != base && excluded[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(base, path)
			sb.WriteString(rel + "\n")
			return nil
		})
	} else {
		entries, err := os.ReadDir(base)
		if err != nil {
			return &Result{Content: "Error listing directory: " + err.Error(), IsError: true}, nil
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		sb.WriteString("Contents of " + base + ":\n")
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() {
				sb.WriteString("[dir]  " + name + "/\n")
			} else {
				info, _ := e.Info()
				size := ""
				if info != nil {
					size = " (" + itoa(int(info.Size())) + " bytes)"
				}
				sb.WriteString("[file] " + name + size + "\n")
			}
		}
	}
	return &Result{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

var excludedDefaults = []string{".git", "node_modules", "vendor", "dist", "build", ".venv", "__pycache__"}
