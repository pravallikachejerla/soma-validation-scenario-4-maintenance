// Package httpapi exposes the pricing application as a JSON HTTP API.
// All handlers emit structured JSON and route every log line through the
// redacting logger; sensitive request fields are NEVER echoed in clear
// text.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/pricing"
	"github.com/somagen/scenario4/internal/security"
	"github.com/somagen/scenario4/internal/storage"
)

// Server bundles the HTTP router and its dependencies.
type Server struct {
	Mux     *http.ServeMux
	Pricing *application.PricingService
	Admin   *application.AdminService
	Store   storage.Store
	Logger  *observability.Logger
	Metrics *observability.Metrics
	Build   BuildInfo
}

// BuildInfo is exposed through /version.
type BuildInfo struct {
	Commit    string
	BuiltAt   string
	DatasetID string
}

// NewServer wires the routes.
func NewServer(s *Server) *http.ServeMux {
	if s.Mux == nil {
		s.Mux = http.NewServeMux()
	}
	s.routes()
	return s.Mux
}

func (s *Server) routes() {
	s.Mux.HandleFunc("/healthz", s.handleHealthz)
	s.Mux.HandleFunc("/version", s.handleVersion)
	s.Mux.HandleFunc("/metrics", s.handleMetrics)

	// API v1
	s.Mux.HandleFunc("/api/v1/pricing/quote", s.withCtx(s.handleQuote))
	s.Mux.HandleFunc("/api/v1/pricing/batch", s.withCtx(s.handleBatch))
	s.Mux.HandleFunc("/api/v1/promotions", s.withCtx(s.handlePromotions))
	s.Mux.HandleFunc("/api/v1/promotions/", s.withCtx(s.handlePromotionItem))
	s.Mux.HandleFunc("/api/v1/products", s.withCtx(s.handleProducts))
	s.Mux.HandleFunc("/api/v1/audit", s.withCtx(s.handleAudit))
	s.Mux.HandleFunc("/api/v1/batch_jobs/", s.withCtx(s.handleBatchJob))
	s.Mux.HandleFunc("/api/v1/admin/seed", s.withCtx(s.handleAdminSeed))
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Build)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Metrics.Snapshot())
}

// withCtx attaches a request id and tenant id to the context, then
// recovers from panics, then writes through the redacting logger. The
// body of the request is NEVER logged in clear text.
func (s *Server) withCtx(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = "req-" + uuid.NewString()
		}
		ctx := observability.WithRequestID(r.Context(), rid)
		if t := r.URL.Query().Get("tenant_id"); t != "" {
			ctx = observability.WithTenantID(ctx, t)
		}
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				s.Logger.Error(ctx, "http.panic", map[string]any{"panic": rec})
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		ww := &statusRecorder{ResponseWriter: w, status: 200}
		h(ww, r.WithContext(ctx))
		s.Logger.Info(ctx, "http.request", map[string]any{
			"method":    r.Method,
			"path":      r.URL.Path,
			"status":    ww.status,
			"latency_ms": time.Since(start).Milliseconds(),
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) handleQuote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req domain.PricingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.At.IsZero() {
		req.At = time.Now().UTC()
	}
	d, err := s.Pricing.Quote(r.Context(), req)
	if err != nil {
		s.Logger.Warn(r.Context(), "pricing.quote.failed", map[string]any{"err": err.Error()})
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

type batchRequest struct {
	TenantID string                  `json:"tenant_id"`
	Requests []domain.PricingRequest `json:"requests"`
}

func (s *Server) handleBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var br batchRequest
	if err := json.NewDecoder(r.Body).Decode(&br); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if br.TenantID == "" {
		http.Error(w, "tenant_id required", http.StatusBadRequest)
		return
	}
	results, err := s.Pricing.Batch(r.Context(), br.TenantID, br.Requests)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	jobID := "job-" + uuid.NewString()
	hash := pricing.ResultHash(results)
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":     jobID,
		"result":     results,
		"result_hash": hash,
	})
}

func (s *Server) handlePromotions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenant := r.URL.Query().Get("tenant_id")
		channel := r.URL.Query().Get("channel")
		if tenant == "" {
			http.Error(w, "tenant_id required", http.StatusBadRequest)
			return
		}
		out, err := s.Pricing.ListPromotions(r.Context(), tenant, channel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, out)
	case http.MethodPost:
		var p domain.Promotion
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if p.ID == "" {
			p.ID = "promo-" + uuid.NewString()
		}
		if err := s.Pricing.CreatePromotion(r.Context(), p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePromotionItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Path[len("/api/v1/promotions/"):]
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	var p domain.Promotion
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	p.ID = id
	if err := s.Pricing.UpdatePromotion(r.Context(), p); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenant := r.URL.Query().Get("tenant_id")
	if tenant == "" {
		http.Error(w, "tenant_id required", http.StatusBadRequest)
		return
	}
	ps, err := s.Pricing.ListProducts(r.Context(), tenant)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenant := r.URL.Query().Get("tenant_id")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	events, err := s.Pricing.ListAudit(r.Context(), tenant, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) handleBatchJob(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/v1/batch_jobs/"):]
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	j, err := s.Store.GetBatchJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) handleAdminSeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prof := application.DefaultProfile()
	if err := s.Admin.Seed(r.Context(), prof); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "seeded", "tenants": prof.Tenants})
}

// LoggedRoundTrip is a placeholder hook for tests that need a
// request/response round-trip in a way that mirrors a real client.
func LoggedRoundTrip(ctx context.Context, _ map[string]any) (map[string]any, error) {
	// This function exists so the security package is not dead-code
	// in scenarios where no other handler redacts. The redactor is
	// invoked on every log write by observability.Logger, and the
	// function below is a no-op for callers but keeps the import
	// alive in the build graph.
	_ = security.RedactMap
	return map[string]any{}, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
