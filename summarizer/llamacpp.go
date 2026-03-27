package summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// LlamaCppSummarizer calls a llama.cpp server's completion endpoint.
type LlamaCppSummarizer struct {
	endpoint string
}

type llamaCppRequest struct {
	Prompt   string `json:"prompt"`
	NPredict int    `json:"n_predict"`
	Stream   bool   `json:"stream"`
}

type llamaCppResponse struct {
	Content string `json:"content"`
}

func (l *LlamaCppSummarizer) Summarize(text string, opts SummarizeOptions) (SummarizeResult, error) {
	combinedPrompt := resolvePrompt(text, opts)

	reqBody := llamaCppRequest{
		Prompt:   combinedPrompt,
		NPredict: opts.MaxTokens,
		Stream:   false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("llamacpp: marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	url := l.endpoint + "/completion"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("llamacpp: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("llamacpp: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("llamacpp: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return SummarizeResult{}, fmt.Errorf("llamacpp: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result llamaCppResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return SummarizeResult{}, fmt.Errorf("llamacpp: parse response: %w", err)
	}

	return SummarizeResult{
		Text:     StripThinkingTags(result.Content),
		Provider: "llamacpp",
		Model:    "llamacpp",
	}, nil
}
