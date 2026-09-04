package setup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultModel is the recommended local coding model to auto-pull.
const DefaultModel = "qwen2.5-coder:7b"

// OllamaHost is the default Ollama server address.
const OllamaHost = "http://localhost:11434"

// IsOllamaInstalled reports whether the ollama binary is on PATH.
func IsOllamaInstalled() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}

// IsOllamaRunning checks whether the Ollama server responds.
func IsOllamaRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := ollamaGet(ctx, "/api/tags")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// OllamaHome returns the directory where Ollama stores its data, if known.
func OllamaHome() string {
	if h := os.Getenv("OLLAMA_MODELS"); h != "" {
		return h
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("USERPROFILE"), ".ollama")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ollama")
}

// InstallOllama downloads and installs Ollama for the current OS.
func InstallOllama(ctx context.Context, status func(string)) error {
	status("Installing Ollama...")

	switch runtime.GOOS {
	case "windows":
		return installOllamaWindows(ctx, status)
	case "darwin":
		return installOllamaMacOS(ctx, status)
	case "linux":
		return installOllamaLinux(ctx, status)
	default:
		return fmt.Errorf("unsupported OS for auto-install: %s", runtime.GOOS)
	}
}

// installOllamaWindows tries winget, then direct download.
func installOllamaWindows(ctx context.Context, status func(string)) error {
	if _, err := exec.LookPath("winget"); err == nil {
		status("Using winget to install Ollama...")
		if err := runCmd(ctx, "winget", "install", "--id", "Ollama.Ollama", "-e",
			"--accept-source-agreements", "--accept-package-agreements", "--silent"); err == nil {
			return nil
		}
		status("winget failed, trying direct download...")
	}
	installer := filepath.Join(os.TempDir(), "OllamaSetup.exe")
	if err := downloadFile(ctx, "https://ollama.com/download/OllamaSetup.exe", installer, status); err != nil {
		return fmt.Errorf("downloading Ollama installer: %w", err)
	}
	status("Running Ollama installer (this may open a window)...")
	if err := runCmd(ctx, installer); err != nil {
		return fmt.Errorf("running Ollama installer: %w", err)
	}
	return nil
}

func installOllamaMacOS(ctx context.Context, status func(string)) error {
	if _, err := exec.LookPath("brew"); err == nil {
		status("Using Homebrew to install Ollama...")
		if err := runCmd(ctx, "brew", "install", "--cask", "ollama"); err == nil {
			return nil
		}
	}
	status("Downloading Ollama for macOS...\nRun the downloaded app to finish installing.")
	zipPath := filepath.Join(os.TempDir(), "Ollama-darwin.zip")
	if err := downloadFile(ctx, "https://ollama.com/download/Ollama-darwin.zip", zipPath, status); err != nil {
		return fmt.Errorf("downloading Ollama: %w", err)
	}
	runCmd(ctx, "open", zipPath)
	return nil
}

func installOllamaLinux(ctx context.Context, status func(string)) error {
	status("Using the official Ollama Linux installer...")
	cmd := exec.CommandContext(ctx, "sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Ollama Linux install failed: %w", err)
	}
	return nil
}

// EnsureRunning starts the Ollama server if installed but not running,
// waits for it to become responsive, then returns.
func EnsureRunning(ctx context.Context, status func(string)) error {
	if IsOllamaRunning() {
		return nil
	}
	if !IsOllamaInstalled() {
		return fmt.Errorf("Ollama is not installed")
	}

	ollamaBin, err := exec.LookPath("ollama")
	if err != nil {
		return err
	}

	status("Starting Ollama server...")
	cmd := exec.CommandContext(ctx, ollamaBin, "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting Ollama: %w", err)
	}
	cmd.Process.Release()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if IsOllamaRunning() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("Ollama server did not become ready in time")
}

// PullModel pulls a model into the running Ollama instance.
func PullModel(ctx context.Context, model string, status func(string)) error {
	status("Pulling model " + model + " (first download may take a while)...")
	cmd := exec.CommandContext(ctx, "ollama", "pull", model)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pulling model %s: %w", model, err)
	}
	return nil
}

// HasModel checks whether a model is already available locally.
func HasModel(ctx context.Context, model string) (bool, error) {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := ollamaGet(ctx2, "/api/tags")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("ollama responded %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return false, err
	}
	return strings.Contains(string(body), model), nil
}

func ollamaGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, OllamaHost+path, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}
