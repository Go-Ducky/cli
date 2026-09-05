# GoDucky

A terminal-based AI coding agent written in Go. It reads, writes, and edits files, runs
shell commands, and searches your codebase. It works with local models via Ollama or any
of the common cloud providers (OpenAI, Anthropic, Gemini and Groq) plus anything that speaks
the OpenAI-compatible API.

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

Or run the installer script, which downloads the right binary and adds it to your
user PATH automatically (works in the current terminal too):

```powershell
irm https://raw.githubusercontent.com/Go-Ducky/cli/main/scripts/install.ps1 -OutFile "$env:TEMP\goducky-install.ps1"
powershell -ExecutionPolicy Bypass -File "$env:TEMP\goducky-install.ps1"
```

If Windows warns when you first run it, right-click `goducky.exe` in Explorer and
choose _Properties → Unblock_.

### macOS

```
curl -fsSL https://raw.githubusercontent.com/Go-Ducky/cli/main/scripts/install.sh | bash
```

### Linux

Same curl installer as above.

The installers add `goducky` to your shell PATH automatically (bash, zsh, fish, or
`~/.profile`) — open a new terminal and `goducky` just works.

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

Run `goducky` in any directory. The first run walks you through setup with simple
menus (arrow keys / WASD): it can install Ollama and pull a local model for you.
A list of recommended models is grouped by family (Qwen, Starcoder, Deepseek,
Codegemma, Llama) — pick one and it's pulled automatically — or you can plug in a
cloud API key. Groq is a good free starting point.

```bash
goducky                                                  # interactive TUI (default: local Ollama)
goducky --dir /path/to/project
goducky -p "explain this repo"                           # one-shot, non-interactive
goducky --models                                         # list available models
goducky --provider ollama                                # skip the wizard, use local models
goducky --provider openrouter                            # cloud models (needs a key)
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
goducky --login openrouter  # many models in one place, incl. free ones
goducky --provider openrouter
```

OpenRouter defaults to `openrouter/free`, a special model id that routes to **any
currently-free model** — so `goducky --provider openrouter` keeps working even as
individual free models rotate in and out. Use `/models` inside the TUI (or
`goducky --models --provider openrouter`) to see the live list of what's free
right now.

```bash
goducky --login openai
goducky --login anthropic
goducky --login gemini
```

You can also set the corresponding environment variables (`GROQ_API_KEY`,
`OPENROUTER_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`,
`GEMINI_API_KEY`) instead.

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

Settings live in `~/.config/goducky/config.json` or `~/Library/Application Support/goducky/config.json` on macOS. Example:

```json
{
  "provider": "ollama",
  "model": "qwen2.5-coder:7b",
  "ollama": { "host": "http://localhost:11434", "model": "qwen2.5-coder:7b" },
  "groq": { "model": "llama-3.3-70b-versatile" },
  "openrouter": { "base_url": "https://openrouter.ai/api/v1", "model": "openrouter/free", "env_key": "OPENROUTER_API_KEY" },
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
/models         Pick a model for the current provider (live list for Ollama / Groq / OpenRouter)
/config         View or edit configuration
/provider       Choose a provider interactively (or: /provider <name>)
/model <name>   Set the model for the current provider
/login          How to add a cloud API key
/clear          Clear the conversation
/exit           Quit
```

Menus (like `/models`, `/provider`) are navigated with **arrow keys or WASD**:
Enter picks, Esc cancels. `Ctrl+C` or `Ctrl+X` quits. Mouse wheel scrolls the
transcript.

You can also edit config from inside the TUI without touching the file:
`/config` shows the current values, and `/config <key> <value>` saves a change.
Keys are the dotted JSON paths (`provider`, `ollama.host`, `openrouter.model`,
`agent.auto_approve`), with friendly aliases for the common ones:
`host`, `auto-approve` (on/off), `iterations`, `output`, `exclude`. Provider and
model changes apply immediately; everything else is picked up on the next run.

### Updating

```
goducky update              # update to the newest release
goducky update v1.0.0       # update to a specific release tag
```

The updater downloads the matching binary for your OS/CPU from GitHub Releases,
verifies its SHA-256 checksum, and replaces the current executable. On Windows
the running binary is renamed aside first, so you can update from within the app.

### Flags

```
goducky
  -p string          Run a one-shot prompt and exit
  -provider string   ollama | groq | openai | openai_compatible | anthropic | gemini | openrouter
  -model string      Model name (overrides config)
  -base-url string   Base URL for OpenAI-compatible endpoints
  -key string        API key (overrides config/env)
  -login string      Save an API key (groq|openai|openai_compatible|anthropic|gemini|openrouter)
  -models            List available models and exit
  -yes               Auto-approve all tool actions
  -dir string        Working directory (default: current)
  -version           Print version and exit
  completion <shell> Print a tab-completion script (bash, zsh, fish, powershell)
  update [tag]       Self-update to the latest release (or a specific tag)
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

Every push to `main` builds all platforms and publishes a release named
`GoDucky <version>` (e.g. `GoDucky 0.1.0-dev.abcd123`) marked as **Latest**, with
change notes generated from merged commits. Push a `v1.0.0` tag for a stable release.
See `.github/workflows/release.yml`. The installers pull from that release.

## License

[MIT](./LICENSE)
