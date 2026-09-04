# GoDucky

A terminal-based AI coding agent written in Go. It reads, writes, and edits files, runs
shell commands, and searches your codebase. It works with local models via Ollama or any
of the common cloud providers (OpenAI, Anthropic, Gemini, Groq, plus anything that speaks
the OpenAI-compatible API).

One static binary. Runs on Windows, macOS, and Linux.

## Install

### Windows

Download `goducky-windows-amd64.exe` (or `goducky-windows-arm64.exe` on ARM devices)
from the [Releases](https://github.com/Go-Ducky/cli/releases) page, rename it to
`goducky.exe`, and place it in a folder on your PATH:

```powershell
$dir = "$HOME\bin"; New-Item -ItemType Directory -Force $dir | Out-Null
Move-Item goducky.exe "$dir\goducky.exe"
$env:PATH += ";$dir"
setx PATH "$env:PATH;$dir"
```

If Windows warns when you first run it, right-click `goducky.exe` in Explorer and
choose *Properties → Unblock*.

### macOS

```
curl -fsSL https://raw.githubusercontent.com/Go-Ducky/cli/main/scripts/install.sh | bash
```

### Linux

Same curl installer as above.

## Shell completion

```bash
goducky completion bash        # or: zsh, fish, powershell
```

Enable it per shell:

- **bash** — `source <(goducky completion bash)` (add that line to `~/.bashrc`)
- **zsh** — `source <(goducky completion zsh)` (add to `~/.zshrc`)
- **fish** — `goducky completion fish | source`
- **PowerShell** — `goducky completion powershell | Out-String | Invoke-Expression` (add to your `$PROFILE`)

Completes all flags (`--provider`, `--model`, `--login`, ...) and offers provider
values.

## Quick start

Run `goducky` in any directory. The first run walks you through setup: it can install
Ollama and pull a local model (`qwen2.5-coder`) for you, or you can plug in a cloud API
key. Groq is a good free starting point.

```bash
goducky                                  # interactive TUI
goducky --dir /path/to/project
goducky -p "explain this repo"           # one-shot, non-interactive
goducky --models                         # list available models
goducky --provider groq                  # skip the wizard, pick a provider
```

## Models

### Local (Ollama)

```bash
ollama pull qwen2.5-coder:7b
goducky --provider ollama --model qwen2.5-coder:7b
```

### Cloud (API key)

```bash
goducky --login groq       # prompts for your key
goducky --provider groq
```

```bash
goducky --login openai
goducky --login anthropic
goducky --login gemini
```

You can also set the corresponding environment variables (`GROQ_API_KEY`,
`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`) instead.

Pick a model at launch:

```bash
goducky --provider openai --model gpt-4o
goducky --provider anthropic --model claude-3-5-sonnet-latest
goducky --provider gemini --model gemini-1.5-pro
```

### Any OpenAI-compatible endpoint

```bash
goducky --provider openai_compatible \
  --base-url http://localhost:1234/v1 \
  --model local-model
```

## Configuration

Settings live in `~/.config/goducky/config.json` (macOS:
`~/Library/Application Support/goducky/config.json`). Example:

```json
{
  "provider": "ollama",
  "model": "qwen2.5-coder:7b",
  "ollama": { "host": "http://localhost:11434", "model": "qwen2.5-coder:7b" },
  "groq": { "model": "llama-3.3-70b-versatile" },
  "openai": { "base_url": "https://api.openai.com/v1", "model": "gpt-4o", "env_key": "OPENAI_API_KEY" },
  "anthropic": { "model": "claude-3-5-sonnet-latest", "env_key": "ANTHROPIC_API_KEY" },
  "gemini": { "model": "gemini-1.5-pro", "env_key": "GEMINI_API_KEY" },
  "agent": { "auto_approve": false }
}
```

## Usage

### In the TUI

```
/help           Show help
/provider <n>   Switch provider (ollama, groq, openai, openai_compatible, anthropic, gemini)
/providers      List providers
/model <name>   Set the model for the current provider
/login          How to add a cloud API key
/clear          Clear the conversation
/exit           Quit
```

### Flags

```
goducky
  -p string          Run a one-shot prompt and exit
  -provider string   ollama | groq | openai | openai_compatible | anthropic | gemini
  -model string      Model name (overrides config)
  -base-url string   Base URL for OpenAI-compatible endpoints
  -key string        API key (overrides config/env)
  -login string      Save an API key
  -models            List available models and exit
  -yes               Auto-approve all tool actions
  -dir string        Working directory (default: current)
  -version           Print version and exit
  completion <shell> Print a tab-completion script (bash, zsh, fish, powershell)
```

### Permissions

By default GoDucky asks before writing/editing files and running commands. Use `--yes` or
set `"auto_approve": true` in the config to skip the prompts.

## Development

```bash
go mod download
go build -o goducky ./cmd/goducky
./scripts/build.ps1    # cross-compile all 6 platform binaries into dist/ (Windows)
powershell -ExecutionPolicy Bypass -File scripts/watch.ps1   # auto-rebuild on save
go test ./...
```

GitHub Actions builds and releases binaries automatically on every push (see
`.github/workflows/`).

## Releases

Push a tag (`v1.0.0`) and the GitHub Actions workflow cross-compiles all platforms and
publishes a release (see `.github/workflows/release.yml`). The installers and package
manifests pull from that release.

## License

[MIT](./LICENSE)
