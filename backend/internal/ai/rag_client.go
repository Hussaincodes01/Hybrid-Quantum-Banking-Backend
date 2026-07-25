package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// RagClient is the Go client for the Python FINIX RAG service.
type RagClient struct {
	baseURL     string
	internalToken string
	httpClient  *http.Client
}

// ChatRequest is the request payload for /v1/rag/chat
type ChatRequest struct {
	Query      string                 `json:"query"`
	UserID     string                 `json:"user_id"`
	DeviceFP   string                 `json:"device_fp,omitempty"`
	Scope      string                 `json:"scope,omitempty"`      // "wealth" | "fraud" | "general"
	History    []ChatMessage          `json:"history,omitempty"`
	Metadata   map[string]any         `json:"metadata,omitempty"`
	MaxSources int                    `json:"max_sources,omitempty"`
}

// ChatMessage represents a chat message.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the response from /v1/rag/chat
type ChatResponse struct {
	Answer            string                 `json:"answer"`
	Citations         []string               `json:"citations"`
	Confidence        float64                `json:"confidence"`
	ValidationSummary *ValidationSummary     `json:"validation_summary,omitempty"`
	Recommendations   *Recommendations       `json:"recommendations,omitempty"`
	RetrievalStats    map[string]any         `json:"retrieval_stats,omitempty"`
}

// ValidationSummary contains the ML validation verdict.
type ValidationSummary struct {
	TxID               string   `json:"tx_id"`
	AgreesWithML       bool     `json:"agrees_with_ml"`
	SuggestedLevel     string   `json:"suggested_level"`
	Confidence         float64  `json:"confidence"`
	Rationale          string   `json:"rationale"`
	Action             string   `json:"action"`
	Citations          []string `json:"citations"`
	XAIExplanation     string   `json:"xai_explanation"`
}

// Recommendations contains personalized recommendations.
type Recommendations struct {
	Security []RecommendationItem `json:"security"`
	Spending []RecommendationItem `json:"spending"`
	Investing []RecommendationItem `json:"investing"`
	XAIExplanations []XAIItem      `json:"xai_explanations"`
}

// RecommendationItem is a single recommendation.
type RecommendationItem struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Priority    string  `json:"priority"` // "high" | "medium" | "low"
	Action      string  `json:"action,omitempty"`
	Confidence  float64 `json:"confidence"`
}

// XAIItem is an explainable AI explanation item.
type XAIItem struct {
	DecisionID    string  `json:"decision_id"`
	DecisionType  string  `json:"decision_type"`
	MLLevel       string  `json:"ml_level"`
	ValidatedLevel string `json:"validated_level"`
	Explanation   string  `json:"explanation"`
	Confidence    float64 `json:"confidence"`
}

// SMSBadgeRequest is the request for /v1/rag/sms/scan
type SMSBadgeRequest struct {
	Sender  string `json:"sender"`
	Message string `json:"message"`
	UserID  string `json:"user_id"`
}

// SMSBadgeResponse is the response from /v1/rag/sms/scan
type SMSBadgeResponse struct {
	Badge       string   `json:"badge"`       // "safe" | "suspicious" | "malicious"
	Confidence  float64  `json:"confidence"`
	Reasons     []string `json:"reasons"`
	Indicators  []string `json:"indicators"`
	Explanation string   `json:"explanation"`
}

// NewRagClient creates a new RAG client.
func NewRagClient(baseURL, internalToken string) *RagClient {
	return &RagClient{
		baseURL:       baseURL,
		internalToken: internalToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BaseURL returns the base URL of the RAG service.
func (c *RagClient) BaseURL() string {
	return c.baseURL
}

// Chat sends a chat request to the RAG service.
func (c *RagClient) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	var resp ChatResponse
	err := c.doRequest(ctx, http.MethodPost, "/v1/rag/chat", req, &resp)
	return resp, err
}

// ScanSMS scans an SMS for fraud indicators.
func (c *RagClient) ScanSMS(ctx context.Context, req SMSBadgeRequest) (SMSBadgeResponse, error) {
	var resp SMSBadgeResponse
	err := c.doRequest(ctx, http.MethodPost, "/v1/rag/sms/scan", req, &resp)
	return resp, err
}

// doRequest performs the HTTP request with auth headers.
func (c *RagClient) doRequest(ctx context.Context, method, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.internalToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("rag service error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	return nil
}

// ValidationVerdict is the verdict sent from Python validation worker to Go backend.
type ValidationVerdict struct {
	TxID           string   `json:"tx_id"`
	AgreesWithML   bool     `json:"agrees_with_ml"`
	SuggestedLevel string   `json:"suggested_level"`
	Confidence     float64  `json:"confidence"`
	Rationale      string   `json:"rationale"`
	Action         string   `json:"action"`
	Citations      []string `json:"citations"`
	XAIExplanation string   `json:"xai_explanation"`
}

// RecommendationsPayload is the recommendations payload from Python.
type RecommendationsPayload struct {
	Security        []RecommendationItem `json:"security"`
	Spending        []RecommendationItem `json:"spending"`
	Investing       []RecommendationItem `json:"investing"`
	XAIExplanations []XAIItem            `json:"xai_explanations"`
}

// ValidationCallbackPayload is the full payload sent to /v1/internal/risk-validation
type ValidationCallbackPayload struct {
	TxID             string                `json:"tx_id"`
	Verdict          ValidationVerdict     `json:"verdict"`
	Recommendations  RecommendationsPayload `json:"recommendations"`
	ValidatedAt      string                `json:"validated_at"`
}