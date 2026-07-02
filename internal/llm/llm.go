// Package llm is the OPTIONAL model layer. It lives outside core/ on purpose:
// the deterministic engine (color, budget, write door) must never depend on a
// model. A provider is used only for bounded judgment (e.g. contradiction
// detection) and is OFF by default — COGO is fully functional without it.
//
// One client (OpenAI-compatible /chat/completions) covers both worlds: a local
// Ollama/LM Studio/vLLM endpoint, or a cheap remote API (DeepSeek, Qwen, GLM,
// OpenAI). Small hosts that can't run a local model just point at a remote URL.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Provider is the minimal contract a model must satisfy for COGO.
type Provider interface {
	Available() bool
	Name() string
	Complete(ctx context.Context, prompt string) (string, error)
}

// Noop is the default: no model. Everything that needs one is simply skipped.
type Noop struct{}

func (Noop) Available() bool { return false }
func (Noop) Name() string    { return "none" }
func (Noop) Complete(context.Context, string) (string, error) {
	return "", fmt.Errorf("no LLM provider configured")
}

// OpenAICompatible talks to any /chat/completions endpoint. Configure it by
// base URL + model (+ optional API key). Local: BaseURL "http://localhost:11434/v1".
// Remote: BaseURL "https://api.deepseek.com" with an APIKey.
type OpenAICompatible struct {
	BaseURL string
	Model   string
	APIKey  string
	Referer string       // optional, sent as HTTP-Referer (OpenRouter attribution)
	Client  *http.Client // optional; a 60s client is used if nil
}

func (o *OpenAICompatible) Available() bool { return o.BaseURL != "" && o.Model != "" }
func (o *OpenAICompatible) Name() string    { return o.Model + " @ " + o.BaseURL }

func (o *OpenAICompatible) Complete(ctx context.Context, prompt string) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"model":       o.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0,
		"stream":      false,
	})
	url := strings.TrimRight(o.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	// OpenRouter app attribution; harmless and ignored by other endpoints.
	req.Header.Set("X-Title", "COGO")
	if o.Referer != "" {
		req.Header.Set("HTTP-Referer", o.Referer)
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llm http %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm: empty response")
	}
	return out.Choices[0].Message.Content, nil
}

// FromEnv builds a provider from COGO_LLM_* env. Unconfigured -> Noop (off).
//
//	COGO_LLM_BASE_URL  e.g. http://localhost:11434/v1  or  https://api.deepseek.com
//	COGO_LLM_MODEL     e.g. qwen2.5:7b  or  deepseek-chat
//	COGO_LLM_API_KEY   optional (required by remote APIs, not by local Ollama)
func FromEnv() Provider {
	base := os.Getenv("COGO_LLM_BASE_URL")
	model := os.Getenv("COGO_LLM_MODEL")
	if base == "" || model == "" {
		return Noop{}
	}
	return &OpenAICompatible{BaseURL: base, Model: model, APIKey: os.Getenv("COGO_LLM_API_KEY"), Referer: os.Getenv("COGO_LLM_REFERER")}
}

// StrongFromEnv builds the independent "strong" provider (COGO_LLM_STRONG_*),
// used where a judge should not share a brain with the proposer (e.g. the
// suasion steelman). Unset, it returns the fallback.
func StrongFromEnv(fallback Provider) Provider {
	base := os.Getenv("COGO_LLM_STRONG_BASE_URL")
	model := os.Getenv("COGO_LLM_STRONG_MODEL")
	if base == "" || model == "" {
		return fallback
	}
	return &OpenAICompatible{BaseURL: base, Model: model, APIKey: os.Getenv("COGO_LLM_STRONG_API_KEY"), Referer: os.Getenv("COGO_LLM_REFERER")}
}
