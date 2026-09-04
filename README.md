# 🦆 GoDucky CLI

**GoDucky CLI** is an AI coding agent that runs directly in your terminal. It reads,
writes, and edits files, runs shell commands, and searches your codebase — powered by
**local models (Ollama)** or **any API provider** (OpenAI, Anthropic, Gemini, and
anything OpenAI-compatible like LM Studio, OpenRouter, Groq, vLLM).

Built with Go. One binary, works on **Windows**, **macOS**, and **Linux**.

---

## ✨ Features

- 🧠 **Local models** via Ollama (Llama, Qwen, DeepSeek, Mistral, ...)
- 🔌 **API-key models**: OpenAI, Anthropic (Claude), Google Gemini, or any OpenAI-compatible endpoint
- 🔧 **Full agentic loop** — creates/edits files, runs commands, searches code autonomously
- 🖥️ **Beautiful TUI** with streaming output, tool call visibility, and approval prompts
- 🛡️ **Permission system** — approve or deny every file write / command run
- 📦 **Single static binary** per platform, no runtime dependencies

---

## 🚀 Install

### Windows

**PowerShell (one-liner):**
```powershell
powershell -ExecutionPolicy Bypass -c "irm https://raw.githubusercontent.com/Go-Ducky/goducky-cli/main/scripts/install.ps1 | iex"
```

**Chocolatey:**
```powershell
choco install goducky
```

**Scoop:**
```powershell
scoop bucket add goducky https://github.com/Go-Ducky/scoop-bucket
scoop install goducky
```

**Manual (EXE):** Download `goducky-windows-amd64.exe` from the [Releases](https://github.com/Go-Ducky/goducky-cli/releases) page and rename it to `goducky.exe`.

### macOS

**Homebrew:**
```bash
brew tap Go-Ducky/tap
brew install goducky
```

**curl (install script):**
```bash
curl -fsSL https://raw.githubusercontent.com/Go-Ducky/goducky-cli/main/scripts/install.sh | bash
```

### Linux

**Homebrew (Linux):**
```bash
brew tap Go-Ducky/tap
brew install goducky
```

**curl (install script):**
```bash
curl -fsSL https://raw.githubusercontent.com/Go-Ducky/goducky-cli/main/scripts/install.sh | bash
```

**Arch Linux (AUR):**
```bash
# with yay
yay -S goducky-bin

# or with paru
paru -S goducky-bin
```

**Manual:** Download the binary for your distro from [Releases](https://github.com/Go-Ducky/goducky-cli/releases), make it executable, and add it to your `PATH`:
```bash
chmod +x goducky-linux-amd64
sudo mv goducky-linux-amd64 /usr/local/bin/goducky
```

---

## 🎯 Quick Start

Just type `goducky` in any directory. The **first run guides you through setup** — it
auto-installs [Ollama](https://ollama.com) if needed and pulls a free local coding model
(`qwen2.5-coder`), or lets you plug in a **cloud API key** (Groq has a free tier).

```bash
# Start the interactive TUI in the current directory (runs first-time setup)
goducky

# Start in a specific project directory
goducky --dir /path/to/project

# One-shot (non-interactive) prompt
goducky -p "explain this repo"

# List available models
goducky --models

# Skip the first-run wizard by picking a provider up front
goducky --provider groq
```

---

## 🧠 Connecting Models

### Option 1: Local models via Ollama (recommended)

On first run GoDucky offers to install Ollama and pull a model automatically. To set it up manually:
```bash
# 1. install Ollama (if not present): the wizard does this for you
# 2. pull a model
ollama pull qwen2.5-coder:7b
# 3. run GoDucky against the local model
goducky --provider ollama --model qwen2.5-coder:7b
```

### Option 2: API key providers

**Free tier first — Groq** (fast, generous free usage):
```bash
goducky --login groq     # prompts for a GROQ_API_KEY
goducky --provider groq  # defaults to llama-3.3-70b-versatile
```

**OpenAI, Anthropic, Gemini:**
```bash
# Save to GoDucky's auth store
goducky --login openai      # prompts for key
goducky --login anthropic
goducky --login gemini

# OR set env vars (recommended for CI/shared machines)
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=...
export GEMINI_API_KEY=...
```

**Choose a provider:**
```bash
goducky --provider openai --model gpt-4o
goducky --provider anthropic --model claude-3-5-sonnet-latest
goducky --provider gemini --model gemini-1.5-pro
```

### Option 3: Any OpenAI-compatible endpoint (LM Studio, OpenRouter, Groq, vLLM, LocalAI...)

```bash
goducky --provider openai_compatible \
      --base-url http://localhost:1234/v1 \
      --model local-model
```

### Option 4: Persist settings in a config file

Settings live in `~/.config/goducky/config.json` (macOS: `~/Library/Application Support/goducky/config.json`). Edit it to set your default provider, model, and agent behavior:
```json
{
  "provider": "ollama",
  "model": "qwen2.5-coder:7b",
  "persona": {
    "name": "Big Pickle",
    "prompt": "You are opencode's coding model — concise, proactive, terminal-native.\nLight, dry tone. Verify your work. Sign off as Big Pickle."
  },
  "ollama": { "host": "http://localhost:11434", "model": "qwen2.5-coder:7b" },
  "groq": { "model": "llama-3.3-70b-versatile" },
  "openai": { "base_url": "https://api.openai.com/v1", "model": "gpt-4o", "env_key": "OPENAI_API_KEY" },
  "anthropic": { "model": "claude-3-5-sonnet-latest", "env_key": "ANTHROPIC_API_KEY" },
  "gemini": { "model": "gemini-1.5-pro", "env_key": "GEMINI_API_KEY" },
  "agent": { "auto_approve": false }
}
```

---

## 🎭 Persona

GoDucky ships with a **"Big Pickle"** persona modeled on opencode's coding agent — concise,
proactive, terminal-native, and verification-minded. The persona sets the assistant's
display name and its personality, and is fully configurable per project/user:

- `persona.name` — what shows in the TUI as the assistant label (e.g. `Big Pickle`).
- `persona.prompt` — personality/style instructions appended to the system prompt.

Edit `~/.config/goducky/config.json`, rebuild, and the assistant will adopt the new identity
across the TUI and one-shot mode.

---

## 🧰 Usage

### TUI commands
```
/help           Show help
/provider <n>   Switch provider (ollama, groq, openai, openai_compatible, anthropic, gemini)
/providers      List providers and how to switch
/model <name>   Set the model for the current provider
/login          How to add a cloud API key
/clear          Clear the conversation
/exit           Quit GoDucky
```
```
Enter              Send message
Ctrl+C             Quit
PageUp / PageDown  Scroll
```

### CLI flags
```
goducky
  -p string          Run a one-shot prompt and exit
  -provider string   ollama | groq | openai | openai_compatible | anthropic | gemini
  -model string      Model name (overrides config)
  -base-url string   Base URL for OpenAI-compatible endpoints
  -key string        API key (overrides config/env)
  -login string      Save an API key (groq | openai | anthropic | gemini)
  -models            List available models and exit
  -yes               Auto-approve all tool actions
  -dir string        Working directory (default: current)
  -version           Print version and exit
```

---

## 🔒 Permissions

By default, GoDucky asks before **writing/editing files** and **running commands**:
```
⚠ write file file=/tmp/foo? [Enter] approve  [Esc] deny
```
Use `--yes` to auto-approve everything (interactive) or set `"auto_approve": true` in config.

---

## 🛠️ Development

```bash
# Dependencies
go mod download

# Build for your platform
go build -o goducky ./cmd/goducky

# Cross-compile everything
./scripts/build.sh

# Test
go test ./...
```

To run with a local model during development:
```bash
ollama pull qwen2.5-coder:7b
go run ./cmd/goducky --provider ollama --model qwen2.5-coder:7b
```

---

## 📦 Packaging

| Target | Method |
|--------|--------|
| Windows EXE | Manual download or `scripts/install.ps1` |
| Windows | Chocolatey (`packaging/choco`) |
| Windows | Scoop (`packaging/scoop`) |
| macOS | Homebrew (`packaging/homebrew`) |
| Linux/macOS | curl installer (`scripts/install.sh`) |
| Apple Silicon + Intel | Universal macOS binary via `lipo` |
| Arch Linux | AUR PKGBUILD (`packaging/aur`) |

**Releasing:** push a tag `v1.0.0` and GitHub Actions cross-compiles all platforms and
publishes a GitHub Release automatically (see `.github/workflows/release.yml`). The
curl/Homebrew/Scoop/AUR packages pull from that release.

---

## 📄 License

[MIT](./LICENSE)
