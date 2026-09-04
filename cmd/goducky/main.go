package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Go-Ducky/cli/internal/agent"
	"github.com/Go-Ducky/cli/internal/agent/tools"
	"github.com/Go-Ducky/cli/internal/config"
	"github.com/Go-Ducky/cli/internal/provider"
	"github.com/Go-Ducky/cli/internal/setup"
	"github.com/Go-Ducky/cli/internal/tui"
	"github.com/charmbracelet/bubbletea"
)

var version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "goducky:", err)
		os.Exit(1)
	}
}

func run() error {
	uniquePrompt := flag.String("p", "", "run a one-shot prompt and exit (non-interactive)")
	providerFlag := flag.String("provider", "", "provider: ollama|groq|openai|openai_compatible|anthropic|gemini")
	modelFlag := flag.String("model", "", "model name (overrides config)")
	baseURLFlag := flag.String("base-url", "", "base URL for openai compatible endpoints")
	keyFlag := flag.String("key", "", "API key (overrides config/env)")
	listModels := flag.Bool("models", false, "list available models and exit")
	autoApprove := flag.Bool("yes", false, "auto-approve all tool actions")
	apiKeyCmd := flag.String("login", "", "save an API key for a provider (groq|openai|anthropic|gemini) and exit")
	dir := flag.String("dir", "", "working directory (default: current)")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("goducky " + version)
		return nil
	}

	if len(flag.Args()) > 0 && flag.Args()[0] == "completion" {
		return completionCmd(flag.Args()[1:])
	}

	if *apiKeyCmd != "" {
		return saveAPIKey(*apiKeyCmd)
	}

	workDir := agent.CurrentDir()
	if *dir != "" {
		abs, err := filepath.Abs(*dir)
		if err != nil {
			return fmt.Errorf("invalid directory: %w", err)
		}
		workDir = abs
	}
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		return fmt.Errorf("working directory does not exist: %s", workDir)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	auth, err := config.LoadAuth()
	if err != nil {
		return err
	}

	if *providerFlag != "" {
		normalized := strings.ToLower(*providerFlag)
		if normalized == "openai" {
			cfg.Provider = "openai"
			cfg.OpenAI.EnvKey = "OPENAI_API_KEY"
		} else {
			cfg.Provider = normalized
		}
	}
	if *baseURLFlag != "" {
		cfg.OpenAI.BaseURL = *baseURLFlag
	}
	if *keyFlag != "" {
		switch cfg.Provider {
		case "groq":
			cfg.Groq.APIKey = *keyFlag
		case "openai", "openai_compatible":
			cfg.OpenAI.APIKey = *keyFlag
		case "anthropic":
			cfg.Anthropic.APIKey = *keyFlag
		case "gemini":
			cfg.Gemini.APIKey = *keyFlag
		}
	}

	// Run first-time onboarding wizard unless a provider was explicitly chosen.
	if !cfg.Onboarded && *providerFlag == "" && *uniquePrompt == "" {
		if _, _, finished, oerr := setup.Onboard(cfg); finished {
			cfg.Onboarded = true
			_ = oerr
		} else {
			// Even if setup was skipped, don't nag on every run.
			cfg.Onboarded = true
		}
		_ = cfg.Save()
		auth, err = config.LoadAuth()
		if err != nil {
			return err
		}
	}

	p, err := provider.New(cfg, auth)
	if err != nil {
		return err
	}

	modelName := provider.ResolveModel(cfg, *modelFlag)

	if *listModels {
		models, err := p.ListModels(context.Background())
		if err != nil {
			return fmt.Errorf("listing models: %w", err)
		}
		if len(models) == 0 {
			fmt.Println("model listing not supported for provider:", cfg.Provider)
			return nil
		}
		for _, m := range models {
			fmt.Println(m)
		}
		return nil
	}

	reg := tools.DefaultRegistry()
	sys := agent.SystemPrompt(workDir)
	agentCfg := &agent.Config{
		MaxIterations:  cfg.Agent.MaxIterations,
		MaxOutputChars: cfg.Agent.MaxOutputChars,
		AutoApprove:    cfg.Agent.AutoApprove || *autoApprove,
	}
	a := agent.New(p, modelName, sys, workDir, agentCfg, reg)
	a.SetAutoApprove(agentCfg.AutoApprove)

	if *uniquePrompt != "" {
		return oneShot(os.Args, a, workDir, *uniquePrompt)
	}

	m := tui.New(a, workDir, providerLabel(cfg.Provider), modelName, cfg, auth)
	prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.SetProgram(prog)
	if _, err := prog.Run(); err != nil {
		return err
	}
	return nil
}

func oneShot(args []string, a *agent.Agent, workDir, prompt string) error {
	// One-shot mode (no TUI): print model output to stdout.
	msgs := []provider.Message{provider.NewTextMessage(provider.RoleUser, prompt)}
	cb := &printCallback{}
	result, _, err := a.Run(context.Background(), msgs, cb)
	if err != nil {
		return err
	}
	fmt.Println(result)
	return nil
}

type printCallback struct{}

func (p *printCallback) OnText(text string)                               {}
func (p *printCallback) OnToolStart(name string, args json.RawMessage)    {}
func (p *printCallback) OnToolEnd(name string, result *tools.Result)      {}
func (p *printCallback) OnStatus(msg string)                              {}
func (p *printCallback) OnComplete(response string, usage provider.Usage) {}

func saveAPIKey(providerName string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	auth, err := config.LoadAuth()
	if err != nil {
		return err
	}
	_ = cfg

	fmt.Fprintf(os.Stderr, "Enter API key for %s: ", providerName)
	var key string
	fmt.Scanln(&key)
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("no key entered")
	}
	switch providerName {
	case "groq":
		auth.GroqAPIKey = key
	case "openai", "openai_compatible":
		auth.OpenAIAPIKey = key
	case "anthropic":
		auth.AnthropicAPIKey = key
	case "gemini":
		auth.GeminiAPIKey = key
	default:
		return fmt.Errorf("unknown provider %q for login", providerName)
	}
	if err := auth.Save(); err != nil {
		return err
	}
	fmt.Printf("Saved API key for %s.\n", providerName)
	return nil
}

func providerLabel(p string) string {
	switch p {
	case "openai_compatible":
		return "OpenAI-compatible"
	default:
		return p
	}
}
