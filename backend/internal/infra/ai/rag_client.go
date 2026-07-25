// Package ai provides the client for communicating with the Python FINIX RAG service.
// This follows the provider-seam pattern (like bank_adapter.go, kyc/mock.go).
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"FINIX/backend/internal/config"
)

// RagClient is the HTTP client for the Python FINIX RAG service.
// It implements the same provider-seam pattern as BankAdapter/KYCProvider.
type RagClient struct {
	baseURL      string
	internalToken string
	httpClient   *http.Client
}

// ChatRequest is the request sent to the RAG service /v1/rag/chat endpoint.
type ChatRequest struct {
	Query       string         `json:"query"`
	History     []ChatMessage  `json:"history,omitempty"`
	UserID      string         `json:"user_id"`
	DeviceFP    string         `json:"device_fp,omitempty"`
	Scope       string         `json:"scope,omitempty"` // wealth|fraud|general
	MaxSources  int            `json:"max_sources,omitempty"`
}

// ChatMessage represents a chat history message.
type ChatMessage struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// ChatResponse is the response from the RAG service.
type ChatResponse struct {
	Answer           string                 `json:"answer"`
	Citations        []Citation             `json:"citations"`
	Confidence       float64                `json:"confidence"`
	ToolsUsed        []string               `json:"tools_used"`
	ValidationSummary *ValidationSummary    `json:"validation_summary,omitempty"`
	Recommendations  *Recommendations       `json:"recommendations,omitempty"`
	Disclaimer       string                 `json:"disclaimer"`
}

// Citation represents a source citation.
type Citation struct {
	OKFID       string  `json:"okf_id"`
	Title       string  `json:"title"`
	Section     string  `json:"section,omitempty"`
	Relevance   float64 `json:"relevance"`
	DocumentURL string  `json:"document_url,omitempty"`
}

// ValidationSummary contains the ML validation verdict.
type ValidationSummary struct {
	TxID              string  `json:"tx_id,omitempty"`
	AgreesWithML      bool    `json:"agrees_with_ml"`
	SuggestedLevel    string  `json:"suggested_level,omitempty"`
	Confidence        float64 `json:"confidence"`
	Rationale         string  `json:"rationale"`
	RecommendedAction string  `json:"recommended_action"`
	Citations         []string `json:"citations"`
	XAIExplanation    string  `json:"xai_explanation"`
}

// Recommendations contains personalized recommendations.
type Recommendations struct {
	Security   []SecurityRec   `json:"security"`
	Spending   []SpendingRec   `json:"spending"`
	Investing  []InvestingRec  `json:"investing"`
	XAI        []XAIExplanation `json:"xai_explanations"`
}

type SecurityRec struct {
	Type        string `json:"type"`         // step_up|freeze|beneficiary_review|limit
	Priority    string `json:"priority"`     // high|medium|low
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

type SpendingRec struct {
	Category    string  `json:"category"`
	Anomaly     string  `json:"anomaly,omitempty"`
	Description string  `json:"description"`
	AmountPaise int64   `json:"amount_paise,omitempty"`
	Confidence  float64 `json:"confidence"`
}

type InvestingRec struct {
	Type        string  `json:"type"`         // sip_adjust|rebalance|goal_funding|asset_allocation
	Description string  `json:"description"`
	AmountPaise int64   `json:"amount_paise,omitempty"`
	GoalID      string  `json:"goal_id,omitempty"`
	Confidence  float64 `json:"confidence"`
}

type XAIExplanation struct {
	DecisionType string `json:"decision_type"` // risk_score|fraud_alert|goal_adjustment
	MLOutput     string `json:"ml_output"`
	Explanation  string `json:"explanation"`
	Confidence   float64 `json:"confidence"`
}

// ScanSMSRequest is the request for SMS scanning.
type ScanSMSRequest struct {
	Sender  string `json:"sender"`
	Message string `json:"message"`
}

// ScanSMSResponse is the response from SMS scanning.
type ScanSMSResponse struct {
	Badge       string   `json:"badge"`        // green|amber|red
	Reason      string   `json:"reason"`
	Indicators  []string `json:"indicators"`
	Model       string   `json:"model"`
	Confidence  float64  `json:"confidence"`
}

// RiskScoreRequest is the request for ML risk scoring.
type RiskScoreRequest struct {
	Features map[string]float64 `json:"features"`
}

// RiskScoreResponse is the response from ML risk scoring.
type RiskScoreResponse struct {
	Score     float64 `json:"score"`
	Tier      string  `json:"tier"`      // low|medium|high
	Factors   []string `json:"factors"`
	ModelVer  string  `json:"model_version"`
}

// NewRagClient creates a new RAG client.
func NewRagClient(cfg config.AIConfig) *RagClient {
	return &RagClient{
		baseURL:       cfg.RagBaseURL,
		internalToken: cfg.InternalToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BaseURL returns the base URL of the RAG service.
func (c *RagClient) BaseURL() string {
	return c.baseURL
}

// Chat sends a chat query to the RAG service.
func (c *RagClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	url := c.baseURL + "/v1/rag/chat"
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rag request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rag API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ScanSMS sends an SMS to the RAG service for BERT-based classification.
func (c *RagClient) ScanSMS(ctx context.Context, sender, message string) (*ScanSMSResponse, error) {
	url := c.baseURL + "/v1/rag/sms"
	req := ScanSMSRequest{Sender: sender, Message: message}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rag SMS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rag SMS API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ScanSMSResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// RiskScore sends features to the RAG service for ML risk scoring.
func (c *RagClient) RiskScore(ctx context.Context, features map[string]float64) (*RiskScoreResponse, error) {
	url := c.baseURL + "/v1/rag/tool/risk"
	req := RiskScoreRequest{Features: features}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rag risk request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("rag risk API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result RiskScoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// Health checks the RAG service health.
func (c *RagClient) Health(ctx context.Context) error {
	url := c.baseURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Token", c.internalToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("rag service unhealthy")
	}
	return nil
}