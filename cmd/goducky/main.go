package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Go-Ducky/cli/internal/agent"
	"github.com/Go-Ducky/cli/internal/agent/tools"
	"github.com/Go-Ducky/cli/internal/config"
	"github.com/Go-Ducky/cli/internal/mcp"
	"github.com/Go-Ducky/cli/internal/provider"
	"github.com/Go-Ducky/cli/internal/session"
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
	providerFlag := flag.String("provider", "", "provider: ollama|groq|openai|openai_compatible|anthropic|gemini|openrouter")
	modelFlag := flag.String("model", "", "model name (overrides config)")
	baseURLFlag := flag.String("base-url", "", "base URL for openai compatible endpoints")
	keyFlag := flag.String("key", "", "API key (overrides config/env)")
	listModels := flag.Bool("models", false, "list available models and exit")
	autoApprove := flag.Bool("yes", false, "auto-approve all tool actions")
	apiKeyCmd := flag.String("login", "", "save an API key for a provider (groq|openai|openai_compatible|anthropic|gemini|openrouter) and exit")
	dir := flag.String("dir", "", "working directory (default: ~/Documents/goducky)")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("goducky " + version)
		return nil
	}

	if len(flag.Args()) > 0 {
		switch flag.Args()[0] {
		case "completion":
			return completionCmd(flag.Args()[1:])
		case "update":
			return updateCmd(flag.Args()[1:])
		case "mcp":
			return mcpCmd(flag.Args()[1:])
		case "sessions":
			return session.PrintList()
		case "resume":
			if len(flag.Args()) == 1 {
				return session.PrintList()
			}
			s, err := session.Load(flag.Args()[1])
			if err != nil {
				return err
			}
			return resumeTUI(s)
		case "rename":
			args := flag.Args()[1:]
			if len(args) != 2 {
				return fmt.Errorf("usage: goducky rename <number-or-name> <new-name>")
			}
			if err := session.Rename(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Chat renamed to %q. Resume with: goducky resume %s\n", args[1], args[1])
			return nil
		}
	}

	if *apiKeyCmd != "" {
		return saveAPIKey(*apiKeyCmd)
	}

	workDir := defaultWorkDir()
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
		case "openrouter":
			cfg.OpenRouter.APIKey = *keyFlag
		}
	}

	// Run first-time onboarding wizard unless a provider was explicitly chosen.
	if !cfg.Onboarded && *providerFlag == "" && *uniquePrompt == "" {
		_, _, finished, oerr := setup.Onboard(cfg)
		if errors.Is(oerr, setup.ErrQuit) {
			os.Exit(0)
		}
		if finished {
			cfg.Onboarded = true
		} else {
			// Even if setup was skipped, don't nag on every run.
			cfg.Onboarded = true
		}
		_ = oerr
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
		auth, aer := config.LoadAuth()
		if aer != nil {
			auth = &config.Auth{}
		}
		printModelsCatalog(cfg, auth)
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

	return startTUI(a, cfg, auth, workDir, modelName, "", nil)
}

// startTUI launches the interactive chat, auto-saving the transcript when the
// user quits so it can be resumed later with `goducky resume`.
//
// Mouse reporting is enabled so the wheel scrolls the chat; arrow up/down
// recall past prompts. On most terminals you can still select/copy by holding
// Ctrl (or Shift) while dragging, and paste with Ctrl+V or right-click.
func startTUI(a *agent.Agent, cfg *config.Config, auth *config.Auth, workDir, modelName, resumeName string, history []provider.Message) error {
	m := tui.New(a, workDir, providerLabel(cfg.Provider), modelName, cfg, auth)
	m.SetHistory(resumeName, history)
	prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.SetProgram(prog)
	if _, err := prog.Run(); err != nil {
		return err
	}
	return autoSaveChat(m)
}

// defaultWorkDir returns ~/Documents/goducky on every OS (creating it when
// needed) so chats and project files always have a stable home.
func defaultWorkDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return agent.CurrentDir()
	}
	wd := filepath.Join(home, "Documents", "goducky")
	if err := os.MkdirAll(wd, 0o755); err == nil {
		return wd
	}
	return agent.CurrentDir()
}

// printModelsCatalog lists the best models for the active provider only.
func printModelsCatalog(cfg *config.Config, auth *config.Auth) {
	prov := cfg.Provider
	fmt.Printf("Models for %s\n", prov)
	fmt.Println(strings.Repeat("=", 40))

	switch prov {
	case "ollama":
		printOllamaModels(cfg)
	default:
		printCloudModels(prov, cfg, auth)
	}

	fmt.Println()
	fmt.Println("See another provider with:  goducky --provider <name> --models")
	fmt.Println("Switch the default in the chat with:  /config provider  (or /provider)")
}

// printOllamaModels lists the recommended local models and what's already pulled.
func printOllamaModels(cfg *config.Config) {
	fmt.Println("(local, free, private; bigger = smarter, needs more RAM)")
	fmt.Println()
	pulled := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	models, err := provider.NewOllama(cfg).ListModels(ctx)
	cancel()
	if err == nil {
		for _, m := range models {
			pulled[m] = true
		}
	}
	if len(pulled) > 0 {
		var have []string
		for _, g := range setup.RecommendedModels {
			for _, m := range g.Models {
				if pulled[m] {
					have = append(have, "  ✔ "+m)
				}
			}
		}
		if len(have) > 0 {
			fmt.Println("Already pulled locally:")
			fmt.Println(strings.Join(have, "\n"))
			fmt.Println()
		}
	} else if err != nil {
		fmt.Printf("(Ollama not reachable: %v)\n", err)
		fmt.Println()
	}
	for _, g := range setup.RecommendedModels {
		fmt.Println(g.Family + " — " + g.Blurb + ":")
		fmt.Println("  " + strings.Join(g.Models, "  "))
		fmt.Println()
	}
	fmt.Println("Pull one with:  goducky -p \"/pull <model>\"  — or /pull <model> in the chat")
}

// printCloudModels lists the featured models for one cloud provider and shows
// how to add the API key if none is saved yet.
func printCloudModels(prov string, cfg *config.Config, auth *config.Auth) {
	best := map[string][]string{
		"groq":              {"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "llama3-8b-8192"},
		"openrouter":        {"openrouter/free (any currently-free model)", "qwen/qwen-2.5-coder-7b-instruct"},
		"openai":            {"gpt-5-mini", "gpt-4o-mini", "gpt-4o"},
		"openai_compatible": {"qwen2.5-coder:7b", "gpt-4o-mini"},
		"anthropic":         {"claude-3-5-haiku-latest", "claude-3-5-sonnet-latest"},
		"gemini":            {"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash"},
	}
	list := best[prov]
	if len(list) == 0 {
		fmt.Println("No featured list for provider " + prov + " yet.")
		return
	}
	fmt.Println("(cloud — these are the best picks with a free API key)")
	fmt.Println()
	for _, m := range list {
		fmt.Println("  • " + m)
	}
	hasKey := hasAPIKey(prov, cfg, auth)
	fmt.Println()
	if !hasKey {
		fmt.Printf("No API key yet. Add one now:\n\n")
		fmt.Printf("    goducky --login %s\n\n", prov)
		fmt.Println("It asks you to paste the key, verifies it against the provider, and saves it.")
	} else {
		fmt.Printf("✔ API key saved — run goducky to chat on %s.\n", prov)
	}
}

// hasAPIKey reports whether the provider already has a key (config, auth file,
// or environment).
func hasAPIKey(prov string, cfg *config.Config, auth *config.Auth) bool {
	var key, env string
	switch prov {
	case "groq":
		key, env = auth.GroqAPIKey, cfg.Groq.EnvKey
	case "openai", "openai_compatible":
		key, env = auth.OpenAIAPIKey, cfg.OpenAI.EnvKey
	case "openrouter":
		key, env = auth.OpenRouterAPIKey, cfg.OpenRouter.EnvKey
	case "anthropic":
		key, env = auth.AnthropicAPIKey, cfg.Anthropic.EnvKey
	case "gemini":
		key, env = auth.GeminiAPIKey, cfg.Gemini.EnvKey
	}
	if key != "" {
		return true
	}
	if env != "" && os.Getenv(env) != "" {
		return true
	}
	return false
}

// mcpCmd starts the MCP stdio server so external tools (Claude Desktop, IDEs)
// can use goducky's file/bash tools in a working directory you select.
func mcpCmd(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	dir := fs.String("dir", "", "working directory for the tools (default: ~/Documents/goducky)")
	fs.Parse(args)

workDir := defaultWorkDir()
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

	reg := tools.DefaultRegistry()
	srv := mcp.New(workDir, reg, version)
	// stdout carries the MCP protocol; any diagnostics must go to stderr.
	return srv.Run(context.Background(), os.Stdin, os.Stdout)
}

func autoSaveChat(m interface{ Session() *session.Session }) error {
	s := m.Session()
	if len(s.Messages) == 0 {
		return nil
	}
	if s.Name == "" {
		s.Name = session.AutoName()
	}
	if err := session.Save(s); err != nil {
		return fmt.Errorf("could not save chat: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\nChat saved as %q. Resume later with: goducky resume %s\n", s.Name, s.Name)
	return nil
}

// resumeTUI loads a saved chat and starts the TUI with its history, provider,
// model and working directory.
func resumeTUI(s *session.Session) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	auth, err := config.LoadAuth()
	if err != nil {
		return err
	}
	cfg.Provider = s.Provider
	cfg.SetProviderModel(s.Provider, s.Model)

	workDir := s.WorkDir
	if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
		workDir = defaultWorkDir()
	}

	p, err := provider.New(cfg, auth)
	if err != nil {
		return err
	}
	modelName := provider.ResolveModel(cfg, "")

	reg := tools.DefaultRegistry()
	sys := agent.SystemPrompt(workDir)
	agentCfg := &agent.Config{
		MaxIterations:  cfg.Agent.MaxIterations,
		MaxOutputChars: cfg.Agent.MaxOutputChars,
		AutoApprove:    cfg.Agent.AutoApprove,
	}
	a := agent.New(p, modelName, sys, workDir, agentCfg, reg)
	a.SetAutoApprove(cfg.Agent.AutoApprove)
	return startTUI(a, cfg, auth, workDir, modelName, s.Name, s.Messages)
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
	switch providerName {
	case "groq", "openai", "openai_compatible", "anthropic", "gemini", "openrouter":
	default:
		return fmt.Errorf("unknown provider %q for login", providerName)
	}

	fmt.Fprintf(os.Stderr, "Enter API key for %s: ", providerName)
	var key string
	fmt.Scanln(&key)
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("no key entered")
	}

	if err := provider.ValidateAPIKey(providerName, key); err != nil {
		if strings.Contains(err.Error(), "rejected") {
			return fmt.Errorf("key not saved: %w", err)
		}
		fmt.Fprintf(os.Stderr, "warning: could not verify the key (no internet?), saving anyway: %v\n", err)
	}

	auth, err := config.LoadAuth()
	if err != nil {
		return err
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
	case "openrouter":
		auth.OpenRouterAPIKey = key
	}
	if err := auth.Save(); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Provider = providerName
	_ = cfg.Save()
	fmt.Printf("Saved and verified API key for %s. Start GoDucky to chat.\n", providerName)
	return nil
}

func providerLabel(p string) string {
	switch p {
	case "openai_compatible":
		return "OpenAI-compatible"
	case "openrouter":
		return "OpenRouter"
	default:
		return p
	}
}
