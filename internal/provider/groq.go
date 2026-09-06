package provider

import (
	"net/http"
	"os"

	"github.com/Go-Ducky/cli/internal/config"
)

const groqBaseURL = "https://api.groq.com/openai/v1"

func NewGroq(cfg *config.Config, auth *config.Auth, _ bool) *OpenAI {
	apiKey := cfg.Groq.APIKey
	if apiKey == "" && auth != nil {
		apiKey = auth.GroqAPIKey
	}
	if apiKey == "" && cfg.Groq.EnvKey != "" {
		apiKey = os.Getenv(cfg.Groq.EnvKey)
	}
	return &OpenAI{
		baseURL:    groqBaseURL,
		apiKey:     apiKey,
		model:      cfg.Groq.Model,
		compatible: true,
		name:       "groq",
		client:     &http.Client{},
	}
}
