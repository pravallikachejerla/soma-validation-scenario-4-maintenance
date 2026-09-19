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
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/override"
	"github.com/somagen/scenario4/internal/pricing"
	"github.com/somagen/scenario4/internal/security"
	"github.com/somagen/scenario4/internal/storage"
)

// Server bundles the HTTP router and its dependencies.
type Server struct {
	Mux      *http.ServeMux
	Pricing  *application.PricingService
	Admin    *application.AdminService
	Override *application.OverrideService
	Store    storage.Store
	Logger   *observability.Logger
	Metrics  *observability.Metrics
	Build    BuildInfo
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

	// Override workflow routes. The actor is identified by a plain
	// header (X-Actor-Id) — no real credentials — and the tenant by
	// X-Tenant-Id. The actor-identifier selector is the same test
	// pattern used by the existing endpoints.
	s.Mux.HandleFunc("/api/v1/overrides", s.withCtx(s.handleOverrides))
	s.Mux.HandleFunc("/api/v1/overrides/", s.withCtx(s.handleOverrideItem))
	s.Mux.HandleFunc("/api/v1/overrides-threshold", s.withCtx(s.handleOverrideThreshold))
	s.Mux.HandleFunc("/api/v1/overrides-apply/", s.withCtx(s.handleOverrideApply))
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
			"method":     r.Method,
			"path":       r.URL.Path,
			"status":     ww.status,
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
		"job_id":      jobID,
		"result":      results,
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

// ---- Override workflow HTTP handlers ----

// actorOrError reads the plain X-Actor-Id header used by the test
// selector (u-pricing-1, u-admin-1, u-admin-2, etc.). It returns an
// empty string + ok=false when the header is missing.
func (s *Server) actorOrError(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	actor := r.Header.Get("X-Actor-Id")
	tenant := r.Header.Get("X-Tenant-Id")
	if tenant == "" {
		http.Error(w, "X-Tenant-Id required", http.StatusBadRequest)
		return "", "", false
	}
	if actor == "" {
		http.Error(w, "X-Actor-Id required", http.StatusBadRequest)
		return "", "", false
	}
	return actor, tenant, true
}

// overrideRequestBody is the JSON body posted to /api/v1/overrides.
type overrideRequestBody struct {
	SKU         string    `json:"sku"`
	Channel     string    `json:"channel"`
	Quantity    int64     `json:"quantity"`
	OverrideYen int64     `json:"override_yen"`
	Reason      string    `json:"reason"`
	ValidFrom   time.Time `json:"valid_from"`
	ValidUntil  time.Time `json:"valid_until"`
}

// thresholdBody is the JSON body for PUT /api/v1/overrides-threshold.
type thresholdBody struct {
	Enabled      bool  `json:"enabled"`
	ThresholdYen int64 `json:"threshold_yen"`
}

// handleOverrides handles the collection endpoint:
//   - GET  /api/v1/overrides?tenant_id=&state=  → list
//   - POST /api/v1/overrides                    → create request
func (s *Server) handleOverrides(w http.ResponseWriter, r *http.Request) {
	if s.Override == nil {
		http.Error(w, "override service not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		tenant := r.Header.Get("X-Tenant-Id")
		if tenant == "" {
			tenant = r.URL.Query().Get("tenant_id")
		}
		if tenant == "" {
			http.Error(w, "X-Tenant-Id required", http.StatusBadRequest)
			return
		}
		state := domain.OverrideState(r.URL.Query().Get("state"))
		var items []domain.Override
		var err error
		if state != "" {
			items, err = s.Override.ListByTenantAndState(r.Context(), tenant, state)
		} else {
			items, err = s.Override.ListByTenant(r.Context(), tenant)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if items == nil {
			items = []domain.Override{}
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		actor, tenant, ok := s.actorOrError(w, r)
		if !ok {
			return
		}
		var body overrideRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if body.SKU == "" || body.Channel == "" {
			http.Error(w, "sku and channel are required", http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		if body.ValidFrom.IsZero() {
			body.ValidFrom = now
		}
		if body.ValidUntil.IsZero() {
			body.ValidUntil = now.Add(30 * 24 * time.Hour)
		}
		o, err := s.Override.Request(r.Context(), override.RequestInput{
			TenantID:    tenant,
			SKU:         body.SKU,
			Channel:     body.Channel,
			Quantity:    body.Quantity,
			OverrideYen: body.OverrideYen,
			RequesterID: actor,
			Reason:      body.Reason,
		})
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, override.ErrOverrideNotEnabled) {
				code = http.StatusConflict
			}
			if errors.Is(err, override.ErrBelowThreshold) {
				code = http.StatusUnprocessableEntity
			}
			if errors.Is(err, override.ErrAlreadyTerminal) {
				code = http.StatusConflict
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, http.StatusCreated, o)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleOverrideItem handles a single override row.
//   - GET    /api/v1/overrides/{id}      → detail
//   - POST   /api/v1/overrides/{id}/approve  → first or second approval
//   - POST   /api/v1/overrides/{id}/reject   → rejection
func (s *Server) handleOverrideItem(w http.ResponseWriter, r *http.Request) {
	if s.Override == nil {
		http.Error(w, "override service not configured", http.StatusServiceUnavailable)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/overrides/")
	if rest == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	if r.Method == http.MethodGet && sub == "" {
		o, err := s.Override.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, o)
		return
	}

	if r.Method == http.MethodPost {
		actor, _, ok := s.actorOrError(w, r)
		if !ok {
			return
		}
		switch sub {
		case "approve":
			var o domain.Override
			var err error
			// The current state determines whether this is the first
			// or the second approval.
			cur, gerr := s.Override.Get(r.Context(), id)
			if gerr != nil {
				if errors.Is(gerr, storage.ErrNotFound) {
					http.Error(w, "not found", http.StatusNotFound)
					return
				}
				http.Error(w, gerr.Error(), http.StatusInternalServerError)
				return
			}
			if cur.State == domain.OverrideRequested {
				o, err = s.Override.FirstApprove(r.Context(), id, actor)
			} else if cur.State == domain.OverrideAwaitingSecondApproval {
				o, err = s.Override.SecondApprove(r.Context(), id, actor)
			} else {
				http.Error(w, "override is not in an approvable state", http.StatusConflict)
				return
			}
			if err != nil {
				code := http.StatusConflict
				if errors.Is(err, storage.ErrNotFound) {
					code = http.StatusNotFound
				}
				http.Error(w, err.Error(), code)
				return
			}
			writeJSON(w, http.StatusOK, o)
			return
		case "reject":
			var body struct {
				Reason string `json:"reason"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			o, err := s.Override.Reject(r.Context(), id, actor, body.Reason)
			if err != nil {
				code := http.StatusConflict
				if errors.Is(err, storage.ErrNotFound) {
					code = http.StatusNotFound
				}
				http.Error(w, err.Error(), code)
				return
			}
			writeJSON(w, http.StatusOK, o)
			return
		default:
			http.Error(w, "unsupported sub-action", http.StatusNotFound)
			return
		}
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// handleOverrideApply marks an approved override as applied.
// POST /api/v1/overrides-apply/{id}
func (s *Server) handleOverrideApply(w http.ResponseWriter, r *http.Request) {
	if s.Override == nil {
		http.Error(w, "override service not configured", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/overrides-apply/")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	actor, _, ok := s.actorOrError(w, r)
	if !ok {
		return
	}
	o, err := s.Override.Apply(r.Context(), id, actor)
	if err != nil {
		code := http.StatusConflict
		if errors.Is(err, storage.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleOverrideThreshold manages the per-tenant override threshold.
// GET  /api/v1/overrides-threshold      → read
// PUT  /api/v1/overrides-threshold      → update (and implicitly enables)
func (s *Server) handleOverrideThreshold(w http.ResponseWriter, r *http.Request) {
	if s.Override == nil {
		http.Error(w, "override service not configured", http.StatusServiceUnavailable)
		return
	}
	tenant := r.Header.Get("X-Tenant-Id")
	if tenant == "" {
		http.Error(w, "X-Tenant-Id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		t, err := s.Override.GetThreshold(r.Context(), tenant)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, t)
	case http.MethodPut:
		actor, _, ok := s.actorOrError(w, r)
		if !ok {
			return
		}
		var body thresholdBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		t, err := s.Override.EnableForTenant(r.Context(), tenant, body.ThresholdYen, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// PUT body may explicitly disable.
		if !body.Enabled {
			t.Enabled = false
			if err := s.Store.UpsertThreshold(r.Context(), t); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusOK, t)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
