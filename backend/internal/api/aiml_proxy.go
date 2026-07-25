package api

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	apimw "FINIX/backend/internal/api/middleware"
)

// aimlProxy migrates the LLM / generative-AI routes off the Go backend and onto
// the Python "AI ecosystem" (finix-rag, :8000) per AI_ECOSYSTEM_ARCHITECTURE.md.
//
// It is a staged, reversible seam — OFF by default. With AIML_PROXY_ENABLED unset,
// every wrapped route keeps its existing in-Go behaviour, so nothing breaks while
// the Python endpoints are still being built. Set AIML_PROXY_ENABLED=true and
// AIML_UPSTREAM_URL to move traffic to the AI ecosystem, one route at a time.
//
// NOTE (prerequisites before enabling): the Python service must expose a matching
// endpoint for each wrapped route, and an identity bridge is required — the AI
// ecosystem authenticates with its OWN TokenPayload, not the Go user JWT, so the
// composition root must mint/forward a service credential the Python side accepts.
type aimlProxy struct {
	enabled  bool
	upstream string
	client   *http.Client
}

func newAIMLProxy() *aimlProxy {
	enabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("AIML_PROXY_ENABLED")))
	upstream := strings.TrimRight(strings.TrimSpace(os.Getenv("AIML_UPSTREAM_URL")), "/")
	if upstream == "" {
		upstream = "http://localhost:8000"
	}
	return &aimlProxy{
		enabled:  enabled,
		upstream: upstream,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// route wraps a local handler. When proxying is enabled it forwards the request to
// upstream+targetPath; on any forward error — or when disabled — it serves the
// local handler, so an unreachable AI ecosystem degrades gracefully instead of
// failing the request (matches the repo's "runs with/without external systems").
func (p *aimlProxy) route(targetPath string, local http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !p.enabled {
			local(w, r)
			return
		}
		if err := p.forward(w, r, targetPath); err != nil {
			slog.Warn("aiml proxy forward failed, falling back to local handler",
				"target", targetPath, "error", err)
			local(w, r)
		}
	}
}

func (p *aimlProxy) forward(w http.ResponseWriter, r *http.Request, targetPath string) error {
	target := p.upstream + targetPath
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		return err
	}
	// Forward only the safe business headers (same allow-list as the expanded-
	// endpoint proxy, L-6). Auth/session headers are deliberately dropped — the
	// AI ecosystem authenticates with its own identity.
	for key := range forwardableHeaders {
		if v := r.Header.Get(key); v != "" {
			outReq.Header.Set(key, v)
		}
	}
	// Attribute the call to the authenticated user without leaking the Go JWT.
	if uid, ok := apimw.UserIDFromContext(r.Context()); ok {
		outReq.Header.Set("X-FINIX-User", uid)
	}

	resp, err := p.client.Do(outReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return nil
}
