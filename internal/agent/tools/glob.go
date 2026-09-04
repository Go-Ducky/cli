package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
)

// GlobTool finds files matching a glob pattern.
type GlobTool struct{ base }

// NewGlob creates the glob tool.
func NewGlob() *GlobTool {
	return &GlobTool{base: base{
		name:        "glob",
		description: "Find files matching a glob pattern (e.g. **/*.go, src/**/*.{ts,tsx}). Use to locate files by name pattern or extension.",
		params: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "The glob pattern to match files against."}
			},
			"required": ["pattern"]
		}`),
	}}
}

type globArgs struct {
	Pattern string `json:"pattern"`
}

func (t *GlobTool) Execute(ctx context.Context, tctx *Context, raw json.RawMessage) (*Result, error) {
	var args globArgs
	if err := ParseArgs(raw, &args); err != nil {
		return nil, err
	}
	if args.Pattern == "" {
		return &Result{Content: "Pattern cannot be empty.", IsError: true}, nil
	}

	var sb strings.Builder
	matches, err := filepath.Glob(filepath.Join(tctx.WorkDir, args.Pattern))
	if err != nil {
		return &Result{Content: "Invalid glob pattern: " + err.Error(), IsError: true}, nil
	}
	if len(matches) == 0 {
		return &Result{Content: "No files matched pattern: " + args.Pattern}, nil
	}
	for _, m := range matches {
		rel, _ := filepath.Rel(tctx.WorkDir, m)
		sb.WriteString(rel + "\n")
	}
	return &Result{Content: strings.TrimRight(sb.String(), "\n")}, nil
}
