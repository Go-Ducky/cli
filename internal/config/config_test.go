package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Default()
	if c.Provider != "ollama" {
		t.Fatalf("default provider should be ollama, got %q", c.Provider)
	}
	if c.Ollama.Host != "http://localhost:11434" {
		t.Fatalf("default ollama host mismatch: %q", c.Ollama.Host)
	}
	if c.OpenRouter.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("default openrouter base url mismatch: %q", c.OpenRouter.BaseURL)
	}
	if c.OpenRouter.EnvKey != "OPENROUTER_API_KEY" {
		t.Fatalf("default openrouter env key mismatch: %q", c.OpenRouter.EnvKey)
	}
	if !strings.HasSuffix(c.OpenRouter.Model, ":free") {
		t.Fatalf("default openrouter model should be a free model, got %q", c.OpenRouter.Model)
	}
	if c.Agent.MaxIterations == 0 {
		t.Fatal("default max iterations should be non-zero")
	}
}

func TestSerializationRoundtrip(t *testing.T) {
	c := Default()
	c.Provider = "openai"
	c.OpenAI.Model = "gpt-4o-mini"
	c.Agent.AutoApprove = true

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if loaded.Provider != "openai" {
		t.Fatalf("provider not roundtripped, got %q", loaded.Provider)
	}
	if loaded.OpenAI.Model != "gpt-4o-mini" {
		t.Fatalf("model not roundtripped, got %q", loaded.OpenAI.Model)
	}
	if !loaded.Agent.AutoApprove {
		t.Fatal("auto_approve not roundtripped")
	}
}

func TestAuthSerialization(t *testing.T) {
	a := &Auth{OpenAIAPIKey: "sk-test", AnthropicAPIKey: "ak-test"}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var loaded Auth
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if loaded.OpenAIAPIKey != "sk-test" || loaded.AnthropicAPIKey != "ak-test" {
		t.Fatalf("auth not roundtripped: %+v", loaded)
	}
}

func TestSet(t *testing.T) {
	c := Default()

	if err := c.Set("provider", "openrouter"); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if c.Provider != "openrouter" {
		t.Fatalf("provider not set, got %q", c.Provider)
	}
	if err := c.Set("provider", "nope"); err == nil {
		t.Fatal("expected error for unknown provider")
	}

	if err := c.Set("model", "qwen/qwen3-coder:free"); err != nil || c.Model != "qwen/qwen3-coder:free" {
		t.Fatalf("set model: %v (model=%q)", err, c.Model)
	}
	if err := c.Set("model", ""); err == nil {
		t.Fatal("expected error for empty model")
	}

	if err := c.Set("ollama.host", "http://192.168.1.10:11434"); err != nil || c.Ollama.Host != "http://192.168.1.10:11434" {
		t.Fatalf("set ollama.host: %v (host=%q)", err, c.Ollama.Host)
	}
	if err := c.Set("openrouter.model", "openai/gpt-4o-mini"); err != nil || c.OpenRouter.Model != "openai/gpt-4o-mini" {
		t.Fatalf("set openrouter.model: %v (model=%q)", err, c.OpenRouter.Model)
	}

	if err := c.Set("agent.auto_approve", "true"); err != nil || !c.Agent.AutoApprove {
		t.Fatalf("set agent.auto_approve true: %v", err)
	}
	if err := c.Set("agent.auto_approve", "banana"); err == nil {
		t.Fatal("expected error for non-bool auto_approve")
	}

	if err := c.Set("agent.max_iterations", "5"); err != nil || c.Agent.MaxIterations != 5 {
		t.Fatalf("set agent.max_iterations: %v", err)
	}
	if err := c.Set("agent.max_iterations", "zero"); err == nil {
		t.Fatal("expected error for non-int max_iterations")
	}

	if err := c.Set("agent.exclude_dirs", ".git, node_modules"); err != nil || len(c.Agent.ExcludeDirs) != 2 || c.Agent.ExcludeDirs[1] != "node_modules" {
		t.Fatalf("set agent.exclude_dirs: %v (dirs=%v)", err, c.Agent.ExcludeDirs)
	}

	if err := c.Set("nope.nothing", "x"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
