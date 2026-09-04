package provider

import (
	"net/http"
	"os"

	"github.com/Go-Ducky/cli/internal/config"
)

// NewOpenRouter creates a provider backed by OpenRouter's OpenAI-compatible API.
// It reuses the generic OpenAI client pointed at OpenRouter's base URL.
func NewOpenRouter(cfg *config.Config, auth *config.Auth) *OpenAI {
	apiKey := cfg.OpenRouter.APIKey
	if apiKey == "" && auth != nil {
		apiKey = auth.OpenRouterAPIKey
	}
	if apiKey == "" && cfg.OpenRouter.EnvKey != "" {
		apiKey = os.Getenv(cfg.OpenRouter.EnvKey)
	}
	return &OpenAI{
		baseURL: cfg.OpenRouter.BaseURL,
		apiKey:  apiKey,
		model:   cfg.OpenRouter.Model,
		name:    "openrouter",
		client:  &http.Client{},
	}
}