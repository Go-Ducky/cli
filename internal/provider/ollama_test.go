package provider

import (
	"testing"

	"github.com/Go-Ducky/cli/internal/config"
)

func TestNewOllamaUsesEnvHost(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "https://aiui.fishie.xyz/api/ollama")
	o := NewOllama(&config.Config{
		Ollama: config.OllamaConfig{Host: "http://localhost:11434"},
	})
	if o.host != "https://aiui.fishie.xyz/api/ollama" {
		t.Fatalf("expected OLLAMA_HOST to override config host, got %q", o.host)
	}
}

func TestNewOllamaDefaultsToConfigHost(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")
	o := NewOllama(&config.Config{
		Ollama: config.OllamaConfig{Host: "http://localhost:11434"},
	})
	if o.host != "http://localhost:11434" {
		t.Fatalf("expected config host when env unset, got %q", o.host)
	}
}
