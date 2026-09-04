package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// GrepTool searches file contents for a pattern.
type GrepTool struct{ base }

// NewGrep creates the grep tool.
func NewGrep() *GrepTool {
	return &GrepTool{base: base{
		name:        "grep",
		description: "Search file contents for a regular expression. Returns file paths and line numbers with matching lines. Use to find where symbols, functions, or strings appear in code.",
		params: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Regular expression to search for."},
				"include": {"type": "string", "description": "File pattern to filter by (e.g. *.go, *.{ts,tsx}) (optional)."},
				"path": {"type": "string", "description": "Directory to search in (default: working directory)."}
			},
			"required": ["pattern"]
		}`),
	}}
}

type grepArgs struct {
	Pattern string `json:"pattern"`
	Include string `json:"include"`
	Path    string `json:"path"`
}

func (t *GrepTool) Execute(ctx context.Context, tctx *Context, raw json.RawMessage) (*Result, error) {
	var args grepArgs
	if err := ParseArgs(raw, &args); err != nil {
		return nil, err
	}
	if args.Pattern == "" {
		return &Result{Content: "Pattern cannot be empty.", IsError: true}, nil
	}

	root := tctx.WorkDir
	if args.Path != "" {
		p, err := resolveAbs(tctx.WorkDir, args.Path)
		if err != nil {
			return &Result{Content: err.Error(), IsError: true}, nil
		}
		root = p
	}

	var sb strings.Builder
	count := 0
	const maxResults = 200

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			for _, ex := range excludedDefaults {
				if info.Name() == ex {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if count >= maxResults {
			return filepath.SkipDir
		}
		if args.Include != "" {
			ok, _ := filepath.Match(args.Include, info.Name())
			if !ok {
				return nil
			}
		}
		// skip binaries
		if isLikelyBinary(path) {
			return nil
		}
		if err := grepFile(path, info, args.Pattern, root, &sb, &count, maxResults); err != nil {
			return nil
		}
		return nil
	})
	if err != nil {
		return &Result{Content: "Error searching: " + err.Error(), IsError: true}, nil
	}

	if count == 0 {
		return &Result{Content: "No matches found for pattern: " + args.Pattern}, nil
	}
	if count >= maxResults {
		sb.WriteString("\n... truncated (showing first " + itoa(maxResults) + " matches)")
	}
	return &Result{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

func grepFile(path string, info os.FileInfo, pattern, root string, sb *strings.Builder, count *int, max int) error {
	if info.Size() > 2*1024*1024 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	re, err := compileRegex(pattern)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			rel, _ := filepath.Rel(root, path)
			line = strings.TrimRight(line, "\r")
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			sb.WriteString(rel + ":" + itoa(lineNum) + ":" + line + "\n")
			*count++
			if *count >= max {
				break
			}
		}
	}
	return nil
}

func isLikelyBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, _ := f.Read(buf)
	for _, c := range buf[:n] {
		if c == 0 {
			return true
		}
	}
	return false
}

func compileRegex(pattern string) (interface {
	MatchString(string) bool
}, error) {
	return newRegex(pattern)
}
