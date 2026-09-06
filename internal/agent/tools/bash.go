package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"time"
)

type BashTool struct{ base }

func NewBash() *BashTool {
	shellDesc := "Execute a shell command in the working directory"
	if runtime.GOOS == "windows" {
		shellDesc = "Execute a PowerShell command in the working directory (Windows). Use PowerShell syntax: Get-ChildItem, Write-Output, etc."
	} else {
		shellDesc = "Execute a bash command in the working directory (macOS/Linux). Use bash syntax: ls, cat, grep, etc."
	}
	return &BashTool{base: base{
		name:        "bash",
		description: shellDesc,
		params: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {"type": "string", "description": "The shell command to execute."},
				"timeout": {"type": "integer", "description": "Timeout in seconds (optional, default 60)."}
			},
			"required": ["command"]
		}`),
	}}
}

type bashArgs struct {
	Command string `json:"command"`
	Timeout *int   `json:"timeout"`
}

func (t *BashTool) Execute(ctx context.Context, tctx *Context, raw json.RawMessage) (*Result, error) {
	var args bashArgs
	if err := ParseArgs(raw, &args); err != nil {
		return nil, err
	}
	if args.Command == "" {
		return &Result{Content: "Empty command.", IsError: true}, nil
	}
	if !tctx.Approval("run command", map[string]any{"command": args.Command}) {
		return &Result{Content: "User denied command execution.", IsError: true}, nil
	}

	timeout := 60 * time.Second
	if args.Timeout != nil && *args.Timeout > 0 {
		timeout = time.Duration(*args.Timeout) * time.Second
	}

	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx2, "powershell", "-NoProfile", "-NonInteractive", "-Command", args.Command)
	} else {
		cmd = exec.CommandContext(ctx2, "bash", "-lc", args.Command)
	}
	cmd.Dir = tctx.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := ""

	if len(stdout.Bytes()) > 0 {
		result += stdout.String()
	}
	if len(stderr.Bytes()) > 0 {
		if result != "" {
			result += "\n"
		}
		result += "stderr:\n" + stderr.String()
	}
	if result == "" {
		result = "(command completed with no output)"
	}

	if ctx2.Err() == context.DeadlineExceeded {
		return &Result{Content: "Command timed out after " + itoa(int(timeout.Seconds())) + "s.\n" + result}, nil
	}
	if err != nil {
		return &Result{Content: "Command failed (exit error): " + err.Error() + "\n" + result, IsError: true}, nil
	}
	return &Result{Content: result}, nil
}
