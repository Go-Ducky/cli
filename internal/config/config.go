package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	Onboarded  bool            `json:"onboarded"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Ollama     OllamaConfig    `json:"ollama"`
	Groq       GroqConfig      `json:"groq"`
	OpenAI     OpenAIConfig    `json:"openai"`
	OpenRouter OpenAIConfig    `json:"openrouter"`
	OpenCode   OpenAIConfig    `json:"opencode"`
	Anthropic  AnthropicConfig `json:"anthropic"`
	Gemini     GeminiConfig    `json:"gemini"`
	Agent      AgentConfig     `json:"agent"`
}

type OllamaConfig struct {
	Host  string `json:"host"`
	Model string `json:"model"`
}

type OpenAIConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	EnvKey  string `json:"env_key"`
}

type AnthropicConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	EnvKey string `json:"env_key"`
}

type GeminiConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	EnvKey string `json:"env_key"`
}

type GroqConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	EnvKey string `json:"env_key"`
}

type AgentConfig struct {
	MaxIterations  int      `json:"max_iterations"`
	MaxOutputChars int      `json:"max_output_chars"`
	AutoApprove    bool     `json:"auto_approve"`
	ExcludeDirs    []string `json:"exclude_dirs"`
}

const (
	configFileName = "config.json"
	authFileName   = "auth.json"
)

func Default() *Config {
	return &Config{
		Provider: "ollama",
		Model:    "qwen2.5-coder:7b",
		Ollama: OllamaConfig{
			Host:  "http://localhost:11434",
			Model: "qwen2.5-coder:7b",
		},
		OpenAI: OpenAIConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4o",
			EnvKey:  "OPENAI_API_KEY",
		},
		OpenRouter: OpenAIConfig{
			BaseURL: "https://openrouter.ai/api/v1",
			Model:   "openrouter/free",
			EnvKey:  "OPENROUTER_API_KEY",
		},
		OpenCode: OpenAIConfig{
			BaseURL: "https://opencode.ai/zen/v1",
			Model:   "big-pickle",
			EnvKey:  "OPENCODE_API_KEY",
		},
		Anthropic: AnthropicConfig{
			Model:  "claude-3-5-sonnet-latest",
			EnvKey: "ANTHROPIC_API_KEY",
		},
		Gemini: GeminiConfig{
			Model:  "gemini-1.5-pro",
			EnvKey: "GEMINI_API_KEY",
		},
		Groq: GroqConfig{
			Model:  "llama-3.3-70b-versatile",
			EnvKey: "GROQ_API_KEY",
		},
		Agent: AgentConfig{
			MaxIterations:  20,
			MaxOutputChars: 12000,
			AutoApprove:    false,
			ExcludeDirs:    []string{".git", "node_modules", "vendor", "dist", "build"},
		},
	}
}

func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goducky"), nil
}

func DataDir() (string, error) {
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "goducky"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goducky"), nil
}

func ConfigPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, configFileName), nil
}

func AuthPath() (string, error) {
	d, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, authFileName), nil
}

func Load() (*Config, error) {
	cfg := Default()
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

var validProviders = []string{"ollama", "groq", "openai", "openai_compatible", "anthropic", "gemini", "openrouter", "opencode"}

func ValidProvider(name string) bool {
	for _, p := range validProviders {
		if p == name {
			return true
		}
	}
	return false
}

func (c *Config) Set(key, value string) error {
	switch key {
	case "provider":
		if !ValidProvider(value) {
			return fmt.Errorf("unknown provider %q (valid: ollama, groq, openai, openai_compatible, anthropic, gemini, openrouter, opencode)", value)
		}
		c.Provider = value
	case "model":
		if value == "" {
			return errors.New("model cannot be empty")
		}
		c.Model = value
	case "host", "ollama.host":
		c.Ollama.Host = value
	case "ollama.model":
		c.Ollama.Model = value
	case "groq.model":
		c.Groq.Model = value
	case "openai.base_url":
		c.OpenAI.BaseURL = value
	case "openai.model":
		c.OpenAI.Model = value
	case "openrouter.base_url":
		c.OpenRouter.BaseURL = value
	case "openrouter.model":
		c.OpenRouter.Model = value
	case "opencode.model":
		c.OpenCode.Model = value
	case "anthropic.model":
		c.Anthropic.Model = value
	case "gemini.model":
		c.Gemini.Model = value
	case "agent.auto_approve", "autoapprove", "auto-approve", "approve":
		b, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("%s expects on/off or true/false, got %q", key, value)
		}
		c.Agent.AutoApprove = b
	case "agent.max_iterations", "iterations":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return fmt.Errorf("iterations expects a positive integer, got %q", value)
		}
		c.Agent.MaxIterations = n
	case "agent.max_output_chars", "output":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return fmt.Errorf("output expects a positive integer, got %q", value)
		}
		c.Agent.MaxOutputChars = n
	case "agent.exclude_dirs", "exclude", "excludes":
		parts := strings.Split(value, ",")
		dirs := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				dirs = append(dirs, p)
			}
		}
		c.Agent.ExcludeDirs = dirs
	default:
		return fmt.Errorf("unknown config key %q (try /config to list keys)", key)
	}
	return nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "yes", "y", "true", "1":
		return true, nil
	case "off", "no", "n", "false", "0":
		return false, nil
	}
	return strconv.ParseBool(s)
}

func (c *Config) SetProviderModel(provider, model string) {
	c.Model = model
	switch provider {
	case "ollama":
		c.Ollama.Model = model
	case "groq":
		c.Groq.Model = model
	case "openai", "openai_compatible":
		c.OpenAI.Model = model
	case "openrouter":
		c.OpenRouter.Model = model
	case "opencode":
		c.OpenCode.Model = model
	case "anthropic":
		c.Anthropic.Model = model
	case "gemini":
		c.Gemini.Model = model
	}
}

type Auth struct {
	GroqAPIKey       string `json:"groq_api_key"`
	OpenAIAPIKey     string `json:"openai_api_key"`
	OpenRouterAPIKey string `json:"openrouter_api_key"`
	OpenCodeAPIKey   string `json:"opencode_api_key"`
	AnthropicAPIKey  string `json:"anthropic_api_key"`
	GeminiAPIKey     string `json:"gemini_api_key"`
}

func LoadAuth() (*Auth, error) {
	a := &Auth{}
	path, err := AuthPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return a, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Auth) Save() error {
	path, err := AuthPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

var ErrUnknownProvider = errors.New("unknown provider")
