package config

import (
	"encoding/json"
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
