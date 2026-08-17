package public

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/httpapi"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/storage"
)

func TestHealthz(t *testing.T) {
	store := storage.NewMemoryStore()
	ps := application.NewPricingService(store, observability.Default("test"), observability.NewMetrics())
	as := application.NewAdminService(store, 42)
	srv := &httpapi.Server{Pricing: ps, Admin: as, Store: store, Logger: observability.Default("test"), Metrics: observability.NewMetrics(), Build: httpapi.BuildInfo{Commit: "test"}}
	mux := httpapi.NewServer(srv)

	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected ok, got %q", body["status"])
	}
}

func TestVersion(t *testing.T) {
	store := storage.NewMemoryStore()
	ps := application.NewPricingService(store, observability.Default("test"), observability.NewMetrics())
	as := application.NewAdminService(store, 42)
	srv := &httpapi.Server{Pricing: ps, Admin: as, Store: store, Logger: observability.Default("test"), Metrics: observability.NewMetrics(), Build: httpapi.BuildInfo{Commit: "abc123"}}
	mux := httpapi.NewServer(srv)

	r := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body httpapi.BuildInfo
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Commit != "abc123" {
		t.Fatalf("commit mismatch: %s", body.Commit)
	}
}
