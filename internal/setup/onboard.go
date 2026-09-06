package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Go-Ducky/cli/internal/config"
	"github.com/Go-Ducky/cli/internal/provider"
	"github.com/Go-Ducky/cli/internal/ui"
)

var ErrQuit = errors.New("quit")

func Onboard(cfg *config.Config) (providerName, modelName string, finished bool, err error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("┌────────────────────────────────────────────────┐")
	fmt.Println("│             GoDucky CLI — Setup                │")
	fmt.Println("└────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Println("GoDucky can run models two ways:")
	fmt.Println("  1. Locally (free) via Ollama — private, no API key, works offline")
	fmt.Println("  2. In the cloud via Groq/OpenAI/Claude/Gemini — needs a free API key")
	fmt.Println()

	localDone, err := localFlow(cfg, reader)
	if err != nil {
		if errors.Is(err, ErrQuit) {
			return "", "", false, ErrQuit
		}
		fmt.Printf("\n  Setup had an issue: %v\n", err)
	}
	if localDone {
		return cfg.Provider, cfg.Model, true, nil
	}

	return cloudFlow(cfg, reader)
}

func localFlow(cfg *config.Config, reader *bufio.Reader) (bool, error) {
	if IsOllamaInstalled() {
		ok, err := ui.RunConfirm("Ollama is already installed. Use local models?", "Yes, use Ollama", "No, use the cloud")
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		if !IsOllamaRunning() {
			fmt.Println("Starting Ollama...")
			if err := EnsureRunning(context.Background(), func(s string) { fmt.Println("  " + s) }); err != nil {
				fmt.Printf("\n  Could not start Ollama: %v\n", err)
				return false, nil
			}
		}
		return pickAndPull(cfg, reader)
	}

	ok, err := ui.RunConfirm("Auto-install Ollama now? (free, local, no account)", "Yes, install Ollama", "No, use the cloud instead")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	status := func(s string) { fmt.Println("  " + s) }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := InstallOllama(ctx, status); err != nil {
		fmt.Printf("\n  Ollama install had an issue: %v\n", err)
		return false, nil
	}
	if err := EnsureRunning(ctx, status); err != nil {
		fmt.Printf("\n  Could not start Ollama: %v\n", err)
		return false, nil
	}
	return pickAndPull(cfg, reader)
}

func pickAndPull(cfg *config.Config, reader *bufio.Reader) (bool, error) {
	opts := RecommendedModelOptions()
	fmt.Println()
	idx, cancel, err := ui.RunSelect("Pick a local model to pull (bigger = smarter, needs more RAM)", opts)
	if err != nil {
		fmt.Printf("  Could not show the menu (%v); pulling the default %s.\n", err, DefaultModel)
		idx = -1
		cancel = false
	}
	if cancel {
		return false, nil
	}

	model := ""
	if idx >= 0 {
		model = ModelFromOption(opts[idx])
	}
	if model == "" {
		if idx >= 0 && strings.Contains(opts[idx], "Quit") {
			return false, ErrQuit
		}
		fmt.Println("  Skipped pulling a model. You can pick one later with /models.")
		return false, nil
	}

	cfg.Ollama.Host = OllamaHost
	cfg.Ollama.Model = model
	cfg.Provider = "ollama"
	cfg.Model = model
	fmt.Printf("Pulling %s (first download may take a while)...\n", model)
	status := func(s string) { fmt.Println("  " + s) }
	pctx, pcancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer pcancel()
	if err := PullModel(pctx, model, status); err != nil {
		fmt.Printf("\n  Model pull had an issue: %v\n", err)
		fmt.Println("  You can still use the chat, and retry later with `ollama pull " + model + "`.")
		return false, nil
	}
	fmt.Printf("\n✔ Ready! Using local model %s.\n", model)
	return true, nil
}

func cloudFlow(cfg *config.Config, reader *bufio.Reader) (string, string, bool, error) {
	fmt.Println()
	fmt.Println("Let's add a cloud provider instead (needs a free API key).")
	cloudOpts := []string{
		"Groq — fast free cloud tier (recommended)",
		"OpenRouter — one key, many free models",
		"OpenCode Zen — curated coding models (big-pickle)",
		"OpenAI — ChatGPT",
		"Anthropic — Claude",
		"Gemini — Google",
		"Skip for now",
	}
	pk, cancel, err := ui.RunSelect("Choose a cloud provider", cloudOpts)
	if err != nil {
		return cfg.Provider, cfg.Model, false, err
	}
	if cancel || pk >= len(cloudOpts)-1 {
		fmt.Println("  (no provider configured yet — you can add one later with `goducky --login <provider>`)")
		return cfg.Provider, cfg.Model, false, nil
	}
	names := []string{"groq", "openrouter", "opencode", "openai", "anthropic", "gemini"}
	choice := names[pk]

	for {
		fmt.Printf("Paste your %s API key: ", choice)
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		if key == "" {
			fmt.Println("  No key entered; skipping.")
			return cfg.Provider, cfg.Model, false, nil
		}
		if err := provider.ValidateAPIKey(choice, key); err != nil {
			fmt.Printf("  ✗ %v\n", err)
			fmt.Println("  Paste it again, or press Enter to skip.")
			continue
		}
		if err := setCloudKey(cfg, choice, key); err != nil {
			fmt.Printf("  Error saving key: %v\n", err)
			return cfg.Provider, cfg.Model, false, err
		}
		cfg.Provider = choice
		cfg.Model = providerDefaultModel(choice)
		setCloudModel(cfg, choice, cfg.Model)
		fmt.Printf("\n✔ Saved %s key and verified it. You can switch models with /models.\n", choice)
		return cfg.Provider, cfg.Model, true, nil
	}
}

func providerDefaultModel(provider string) string {
	switch provider {
	case "groq":
		return "llama-3.3-70b-versatile"
	case "openai":
		return "gpt-4o-mini"
	case "openrouter":
		return "openrouter/free"
	case "opencode":
		return "big-pickle"
	case "anthropic":
		return "claude-3-5-haiku-latest"
	case "gemini":
		return "gemini-1.5-flash"
	}
	return ""
}

func setCloudModel(cfg *config.Config, provider, model string) {
	switch provider {
	case "groq":
		cfg.Groq.Model = model
	case "openai":
		cfg.OpenAI.Model = model
	case "openrouter":
		cfg.OpenRouter.Model = model
	case "opencode":
		cfg.OpenCode.Model = model
	case "anthropic":
		cfg.Anthropic.Model = model
	case "gemini":
		cfg.Gemini.Model = model
	}
}

func setCloudKey(cfg *config.Config, provider, key string) error {
	auth, err := config.LoadAuth()
	if err != nil {
		return err
	}
	switch provider {
	case "groq":
		auth.GroqAPIKey = key
	case "openai":
		auth.OpenAIAPIKey = key
	case "anthropic":
		auth.AnthropicAPIKey = key
	case "gemini":
		auth.GeminiAPIKey = key
	case "openrouter":
		auth.OpenRouterAPIKey = key
	case "opencode":
		auth.OpenCodeAPIKey = key
	}
	return auth.Save()
}
