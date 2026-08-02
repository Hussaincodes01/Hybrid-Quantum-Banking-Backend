package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultSystemPrompt = `You are FINIX AI, a helpful financial assistant for Indian users. You help with budgeting, investments, tax planning, UPI transactions, insurance, and loans. Always give practical advice relevant to Indian financial regulations. Never recommend specific stocks. Always add a disclaimer that this is AI-generated advice.`

type ChatbotClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func NewChatbotClient() *ChatbotClient {
	apiKey := GetSecret("GROQ_API_KEY")
	if apiKey == "" {
		return nil
	}
	baseURL := GetSecret("GROQ_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	model := GetSecret("GROQ_MODEL")
	if model == "" {
		// llama3-8b-8192 has been decommissioned by Groq: the API rejects it,
		// the chat call errors, and the service quietly degrades to template
		// replies that look like a working chatbot. Default to a served model.
		model = "llama-3.3-70b-versatile"
	}
	return &ChatbotClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Model reports the model this client actually calls, so the audit trail and
// the explainability shown to the user name the real model rather than a
// hardcoded string that drifts whenever GROQ_MODEL is set.
func (c *ChatbotClient) Model() string {
	if c == nil {
		return ""
	}
	return c.model
}

func (c *ChatbotClient) Query(ctx context.Context, systemPrompt string, userPrompt string, history []ChatMessage) (string, error) {
	messages := make([]ChatMessage, 0, len(history)+2)
	messages = append(messages, ChatMessage{Role: "system", Content: systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, ChatMessage{Role: "user", Content: userPrompt})

	reqBody := ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		MaxTokens:   1024,
		Temperature: 0.7,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("groq request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("groq API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("groq API error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("groq returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
