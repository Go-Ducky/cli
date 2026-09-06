package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/Go-Ducky/cli/internal/config"
)

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

func OpenRouterFreeModels(ctx context.Context, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter models: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt string `json:"prompt"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	var free []string
	seen := map[string]bool{}
	for _, m := range out.Data {
		if (m.Pricing.Prompt == "0" || strings.HasSuffix(m.ID, ":free") || strings.Contains(m.ID, "free")) && !seen[m.ID] {
			seen[m.ID] = true
			free = append(free, m.ID)
		}
	}
	sort.Strings(free)
	return free, nil
}
