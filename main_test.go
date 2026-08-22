package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandlerRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if body["message"] != "Hello, World!" {
		t.Fatalf("expected message=Hello, World!, got %q", body["message"])
	}
	if body["service"] != "testRepo" {
		t.Fatalf("expected service=testRepo, got %q", body["service"])
	}
	if body["version"] != "1.0.0" {
		t.Fatalf("expected version=1.0.0, got %q", body["version"])
	}
}

func TestHandlerUnknownPathReturns404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestTimeHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/time", nil)
	w := httptest.NewRecorder()

	timeHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := body["current_time"]; !ok {
		t.Fatalf("expected current_time field, got %v", body)
	}
}

func TestEchoHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/echo?msg=hello", nil)
	w := httptest.NewRecorder()

	echoHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["message"] != "hello" {
		t.Fatalf("expected message=hello, got %v", body)
	}
}

func TestEchoHandlerMissingMsg(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/echo", nil)
	w := httptest.NewRecorder()

	echoHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHealthHandler(t *testing.T) {
	// Save and restore startTime
	original := startTime
	startTime = time.Now().Add(-2 * time.Hour)
	defer func() { startTime = original }()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", body.Status)
	}
	if body.Uptime == "" {
		t.Fatalf("expected non-empty uptime, got %q", body.Uptime)
	}
	if body.Timestamp == "" {
		t.Fatalf("expected non-empty timestamp, got %q", body.Timestamp)
	}
	// Verify timestamp is parseable as RFC 3339
	if _, err := time.Parse(time.RFC3339, body.Timestamp); err != nil {
		t.Fatalf("timestamp not in RFC 3339 format: %v", err)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	wrapped := loggingMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTeapot {
		t.Fatalf("expected status %d, got %d", http.StatusTeapot, w.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := corsMiddleware(inner)

	// Test regular GET request sets CORS headers
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if origin := w.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin=*, got %q", origin)
	}
	if methods := w.Header().Get("Access-Control-Allow-Methods"); methods != "GET, POST, OPTIONS" {
		t.Fatalf("expected Access-Control-Allow-Methods=GET, POST, OPTIONS, got %q", methods)
	}
	if headers := w.Header().Get("Access-Control-Allow-Headers"); headers != "Content-Type, Authorization" {
		t.Fatalf("expected Access-Control-Allow-Headers=Content-Type, Authorization, got %q", headers)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Test OPTIONS preflight short-circuits with 204
	reqOptions := httptest.NewRequest(http.MethodOptions, "/test", nil)
	wOptions := httptest.NewRecorder()
	wrapped.ServeHTTP(wOptions, reqOptions)

	if wOptions.Code != http.StatusNoContent {
		t.Fatalf("expected status %d for OPTIONS preflight, got %d", http.StatusNoContent, wOptions.Code)
	}
	if origin := wOptions.Header().Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin=* on preflight, got %q", origin)
	}
}
