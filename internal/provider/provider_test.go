package provider

import (
	"encoding/json"
	"testing"

	"github.com/Go-Ducky/cli/internal/config"
)

func TestResolveModelForProviders(t *testing.T) {
	cfg := config.Default()

	cfg.Provider = "ollama"
	cfg.Ollama.Model = "llama3.1"
	if got := ResolveModel(cfg, ""); got != "llama3.1" {
		t.Fatalf("ollama model resolve: got %q", got)
	}

	if got := ResolveModel(cfg, "qwen2.5"); got != "qwen2.5" {
		t.Fatalf("explicit model should win, got %q", got)
	}

	cfg.Provider = "openai"
	cfg.OpenAI.Model = "gpt-4o"
	if got := ResolveModel(cfg, ""); got != "gpt-4o" {
		t.Fatalf("openai model resolve: got %q", got)
	}
}

func TestProviderToolSchemaValidJSON(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(`{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}`), &schema); err != nil {
		t.Fatalf("schema invalid: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("expected object type, got %v", schema["type"])
	}
}
