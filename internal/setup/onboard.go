package setup

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Go-Ducky/cli/internal/config"
)

// Onboard sets up GoDucky on first run: it ensures a model is available,
// auto-installing Ollama and pulling a local coding model if the user consents,
// otherwise guiding them to add a cloud API key (Groq recommended).
//
// It returns the chosen provider and model, saving nothing itself (the caller
// persists config). finished is true when setup completed successfully.
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
	installOllama := false
	if IsOllamaInstalled() {
		fmt.Println("✔ Ollama is already installed.")
		installOllama = IsOllamaRunning()
		if !installOllama {
			fmt.Print("Start Ollama now? [Y/n]: ")
			if confirm(reader) {
				if err := EnsureRunning(context.Background(), func(s string) { fmt.Println("  " + s) }); err == nil {
					installOllama = true
				}
			}
		}
	} else {
		fmt.Print("Want me to auto-install Ollama (free, local, no account)? [Y/n]: ")
		if confirm(reader) {
			installOllama = true
		}
	}

	if installOllama {
		status := func(s string) { fmt.Println("  " + s) }
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if !IsOllamaInstalled() {
			if err := InstallOllama(ctx, status); err != nil {
				fmt.Printf("\n  Ollama install had an issue: %v\n", err)
			}
		}
		if err := EnsureRunning(ctx, status); err != nil {
			fmt.Printf("\n  Could not start Ollama: %v\n", err)
		} else {
			cfg.Ollama.Host = OllamaHost
			cfg.Ollama.Model = DefaultModel
			cfg.Provider = "ollama"
			cfg.Model = DefaultModel
			fmt.Println()
			fmt.Printf("Pulling model %s (first download is large, ~4GB)...\n", DefaultModel)
			if err := PullModel(ctx, DefaultModel, status); err != nil {
				fmt.Printf("\n  Model pull had an issue: %v\n", err)
			} else {
				fmt.Printf("\n✔ Ready! Using local model %s.\n", DefaultModel)
				return "ollama", DefaultModel, true, nil
			}
		}
	}

	fmt.Println()
	fmt.Println("Let's add a cloud provider instead (needs a free API key).")
	fmt.Println("  * Groq      — fast, has a free tier, no credit card (recommended)")
	fmt.Println("  * OpenRouter— many models in one place, incl. free ones")
	fmt.Println("  * OpenAI    — ChatGPT models")
	fmt.Println("  * Anthropic — Claude models")
	fmt.Println("  * Gemini    — Google models")
	for {
		fmt.Print("Choose provider [groq/openai/anthropic/gemini/openrouter/skip]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.ToLower(strings.TrimSpace(choice))
		switch choice {
		case "":
			fmt.Println("  (no provider configured yet — you can add one later with `goducky --login <provider>`)")
			return cfg.Provider, cfg.Model, false, nil
		case "groq", "openai", "anthropic", "gemini", "openrouter":
			fmt.Print("Paste your API key: ")
			key, _ := reader.ReadString('\n')
			key = strings.TrimSpace(key)
			if key == "" {
				fmt.Println("  No key entered; skipping.")
				continue
			}
			if err := setCloudKey(cfg, choice, key); err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			cfg.Provider = choice
			fmt.Printf("\n✔ Saved %s key. You can change models with the /model command.\n", choice)
			return cfg.Provider, providerDefaultModel(choice), true, nil
		case "skip", "s", "later":
			return cfg.Provider, cfg.Model, false, nil
		default:
			fmt.Println("  Please choose groq, openai, anthropic, gemini, openrouter, or skip.")
		}
	}
}

// Quick returns the recommended cloud provider the user should try first.
func providerDefaultModel(provider string) string {
	switch provider {
	case "groq":
		return "llama-3.3-70b-versatile"
	case "openai":
		return "gpt-4o-mini"
	case "openrouter":
		return "qwen/qwen3-coder:free"
	case "anthropic":
		return "claude-3-5-haiku-latest"
	case "gemini":
		return "gemini-1.5-flash"
	}
	return ""
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
	}
	return auth.Save()
}

func confirm(reader *bufio.Reader) bool {
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "" || line == "y" || line == "yes"
}
