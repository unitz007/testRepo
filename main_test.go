package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "root path", path: "/", wantStatus: http.StatusOK, wantBody: "Hello, World!"},
		{name: "unknown path", path: "/does-not-exist", wantStatus: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, w.Code)
			}
			if tc.wantBody != "" && w.Body.String() != tc.wantBody {
				t.Fatalf("unexpected body: %q", w.Body.String())
			}
		})
	}
}

func TestTimeHandler(t *testing.T) {
	tests := []struct {
		name       string
		wantStatus int
	}{
		{name: "returns current time in RFC3339", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/time", nil)
			w := httptest.NewRecorder()

			timeHandler(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, w.Code)
			}

			var body map[string]string
			if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			currentTime, ok := body["current_time"]
			if !ok {
				t.Fatalf("expected current_time field, got %v", body)
			}
			if _, err := time.Parse(time.RFC3339, currentTime); err != nil {
				t.Fatalf("current_time %q not in RFC3339 format: %v", currentTime, err)
			}
		})
	}
}

func TestEchoHandler(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantMessage string
		wantErrText string
	}{
		{name: "valid msg", path: "/api/echo?msg=hello", wantStatus: http.StatusOK, wantMessage: "hello"},
		{name: "missing msg param", path: "/api/echo", wantStatus: http.StatusBadRequest, wantErrText: "msg parameter is required"},
		{name: "empty msg param", path: "/api/echo?msg=", wantStatus: http.StatusBadRequest, wantErrText: "msg parameter is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()

			echoHandler(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, w.Code)
			}

			if tc.wantStatus == http.StatusOK {
				var body map[string]string
				if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if body["message"] != tc.wantMessage {
					t.Fatalf("expected message=%q, got %v", tc.wantMessage, body)
				}
				return
			}

			if !strings.Contains(w.Body.String(), tc.wantErrText) {
				t.Fatalf("expected body to contain %q, got %q", tc.wantErrText, w.Body.String())
			}
		})
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
