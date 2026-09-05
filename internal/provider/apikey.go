package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ValidateAPIKey checks a key against the provider's own endpoint so users get
// immediate feedback at `--login` / onboarding instead of confusing failures
// later. It returns nil if the key is accepted by the provider.
func ValidateAPIKey(name, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key = strings.TrimSpace(key)

	var err error
	switch name {
	case "groq":
		err = checkBearer(ctx, "https://api.groq.com/openai/v1/models", key)
	case "openai", "openai_compatible":
		base := "https://api.openai.com/v1"
		if env := os.Getenv("OPENAI_BASE_URL"); env != "" {
			base = env
		}
		err = checkBearer(ctx, strings.TrimRight(base, "/")+"/models", key)
	case "anthropic":
		err = checkAnthropic(ctx, key)
	case "gemini":
		err = checkGemini(ctx, key)
	case "openrouter":
		err = checkBearer(ctx, "https://openrouter.ai/api/v1/auth/key", key)
	default:
		return nil // nothing to check for unknown/local providers
	}
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "400") ||
		strings.Contains(msg, "invalid") || strings.Contains(msg, "unauthor") || strings.Contains(msg, "api key not valid") {
		return fmt.Errorf("%s rejected the key — double-check it for spaces or typos (%v)", name, err)
	}
	return fmt.Errorf("could not reach %s to verify the key: %w", name, err)
}

func checkBearer(ctx context.Context, url, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	return doStatus(req)
}

func checkAnthropic(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	return doStatus(req)
}

func checkGemini(ctx context.Context, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://generativelanguage.googleapis.com/v1beta/models?key="+key, nil)
	if err != nil {
		return err
	}
	return doStatus(req)
}

func doStatus(req *http.Request) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("HTTP %d", resp.StatusCode)
}
