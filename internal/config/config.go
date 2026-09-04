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

// Config is the top-level user configuration.
type Config struct {
	Onboarded  bool            `json:"onboarded"` // set to true after first-run setup
	Provider   string          `json:"provider"`  // ollama | groq | openai | openai_compatible | anthropic | gemini | openrouter
	Model      string          `json:"model"`
	Ollama     OllamaConfig    `json:"ollama"`
	Groq       GroqConfig      `json:"groq"`
	OpenAI     OpenAIConfig    `json:"openai"`
	OpenRouter OpenAIConfig    `json:"openrouter"`
	Anthropic  AnthropicConfig `json:"anthropic"`
	Gemini     GeminiConfig    `json:"gemini"`
	Agent      AgentConfig     `json:"agent"`
}

// OllamaConfig configures the local Ollama runtime.
type OllamaConfig struct {
	Host  string `json:"host"`  // default http://localhost:11434
	Model string `json:"model"` // e.g. llama3.1
}

// OpenAIConfig configures any OpenAI-compatible endpoint.
type OpenAIConfig struct {
	BaseURL string `json:"base_url"` // e.g. https://api.openai.com/v1
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	EnvKey  string `json:"env_key"` // env var holding the key, e.g. OPENAI_API_KEY
}

// AnthropicConfig configures Anthropic Claude.
type AnthropicConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	EnvKey string `json:"env_key"`
}

// GeminiConfig configures Google Gemini.
type GeminiConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	EnvKey string `json:"env_key"`
}

// GroqConfig configures Groq (free fast cloud models).
type GroqConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
	EnvKey string `json:"env_key"`
}

// AgentConfig controls agentic behavior.
type AgentConfig struct {
	MaxIterations  int      `json:"max_iterations"` // max tool-calling loops per prompt
	MaxOutputChars int      `json:"max_output_chars"`
	AutoApprove    bool     `json:"auto_approve"` // skip permission prompts
	ExcludeDirs    []string `json:"exclude_dirs"`
}

const (
	configFileName = "config.json"
	authFileName   = "auth.json"
)

// Default returns a configuration with sensible defaults.
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
			Model:   "qwen/qwen3-coder:free",
			EnvKey:  "OPENROUTER_API_KEY",
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

// ConfigDir returns the platform-appropriate config directory.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goducky"), nil
}

// DataDir returns the platform-appropriate data directory.
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

// ConfigPath returns the full path to the config file.
func ConfigPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, configFileName), nil
}

// AuthPath returns the full path to the auth file (API keys).
func AuthPath() (string, error) {
	d, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, authFileName), nil
}

// Load reads the config from disk, returning defaults if none exists.
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

// Save writes the config to disk, creating directories as needed.
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

var validProviders = []string{"ollama", "groq", "openai", "openai_compatible", "anthropic", "gemini", "openrouter"}

// ValidProvider reports whether name is a known provider.
func ValidProvider(name string) bool {
	for _, p := range validProviders {
		if p == name {
			return true
		}
	}
	return false
}

// Set applies a dotted config key (e.g. "ollama.host", "agent.auto_approve")
// with an untyped string value, as typed into the /config command.
func (c *Config) Set(key, value string) error {
	switch key {
	case "provider":
		if !ValidProvider(value) {
			return fmt.Errorf("unknown provider %q (valid: ollama, groq, openai, openai_compatible, anthropic, gemini, openrouter)", value)
		}
		c.Provider = value
	case "model":
		if value == "" {
			return errors.New("model cannot be empty")
		}
		c.Model = value
	case "ollama.host":
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
	case "anthropic.model":
		c.Anthropic.Model = value
	case "gemini.model":
		c.Gemini.Model = value
	case "agent.auto_approve":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("agent.auto_approve expects true/false, got %q", value)
		}
		c.Agent.AutoApprove = b
	case "agent.max_iterations":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return fmt.Errorf("agent.max_iterations expects a positive integer, got %q", value)
		}
		c.Agent.MaxIterations = n
	case "agent.max_output_chars":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return fmt.Errorf("agent.max_output_chars expects a positive integer, got %q", value)
		}
		c.Agent.MaxOutputChars = n
	case "agent.exclude_dirs":
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

// Auth holds API keys, kept separate from config so they aren't committed.
type Auth struct {
	GroqAPIKey        string `json:"groq_api_key"`
	OpenAIAPIKey      string `json:"openai_api_key"`
	OpenRouterAPIKey  string `json:"openrouter_api_key"`
	AnthropicAPIKey   string `json:"anthropic_api_key"`
	GeminiAPIKey      string `json:"gemini_api_key"`
}

// LoadAuth reads the auth file, returning zero-value if absent.
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

// SaveAuth writes the auth file.
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

// ErrUnknownProvider is returned for unsupported provider names.
var ErrUnknownProvider = errors.New("unknown provider")
