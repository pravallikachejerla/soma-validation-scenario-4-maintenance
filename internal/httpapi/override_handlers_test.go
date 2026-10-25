// Package httpapi — override_handlers_test.go
//
// HTTP-level tests for the override approval workflow routes.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/storage"
)

// auditCapture is a thread-safe in-memory audit writer for tests.
type auditCapture struct {
	mu     sync.Mutex
	events []domain.AuditEvent
}

func (a *auditCapture) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
	return nil
}

func newTestServer() (*Server, *auditCapture) {
	store := storage.NewMemoryStore()
	audit := &auditCapture{}
	s := &Server{
		Store:  store,
		Logger: observability.Default("test"),
	}
	s.Mux = http.NewServeMux()
	s.registerOverrideRoutes()
	return s, audit
}

func doRequest(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestOverrideHTTP_SettingsGetDefault(t *testing.T) {
	s, _ := newTestServer()
	rr := doRequest(t, s.Mux, http.MethodGet, "/api/v1/overrides/settings?tenant_id=tenant-a", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var cfg domain.TenantOverrideSettings
	if err := json.NewDecoder(rr.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.OverrideEnabled {
		t.Fatalf("should be disabled by default")
	}
	if cfg.ThresholdYen != domain.DefaultThresholdYen {
		t.Fatalf("threshold = %d, want %d", cfg.ThresholdYen, domain.DefaultThresholdYen)
	}
}

func TestOverrideHTTP_SettingsPutAndGet(t *testing.T) {
	s, _ := newTestServer()
	cfg := domain.TenantOverrideSettings{
		TenantID:          "tenant-a",
		OverrideEnabled:   true,
		ThresholdYen:      3_000_000,
		RequiredApprovals: 2,
	}
	rr := doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", cfg)
	if rr.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200", rr.Code)
	}

	rr = doRequest(t, s.Mux, http.MethodGet, "/api/v1/overrides/settings?tenant_id=tenant-a", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rr.Code)
	}
	var got domain.TenantOverrideSettings
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ThresholdYen != 3_000_000 {
		t.Fatalf("threshold = %d, want 3,000,000", got.ThresholdYen)
	}
}

func TestOverrideHTTP_CreateRequest_FeatureDisabled(t *testing.T) {
	s, _ := newTestServer()
	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "test",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "not enabled") {
		t.Fatalf("body = %q, want 'not enabled'", rr.Body.String())
	}
}

func TestOverrideHTTP_CreateRequest_AutoApproved(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 10_000_000, RequiredApprovals: 2,
	})

	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 1_000, BaseYen: 2_000, Reason: "small",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
	var req domain.OverrideRequest
	if err := json.NewDecoder(rr.Body).Decode(&req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.Status != domain.OverrideApplied {
		t.Fatalf("status = %s, want applied", req.Status)
	}
}

func TestOverrideHTTP_CreateRequest_Pending(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 1_000_000, RequiredApprovals: 2,
	})

	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "big",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rr.Code)
	}
	var req domain.OverrideRequest
	if err := json.NewDecoder(rr.Body).Decode(&req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.Status != domain.OverridePending {
		t.Fatalf("status = %s, want pending", req.Status)
	}
	if req.ID == "" {
		t.Fatalf("ID should be set")
	}
}

func TestOverrideHTTP_ApproveFlow(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 1_000_000, RequiredApprovals: 2,
	})

	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "big",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	var req domain.OverrideRequest
	json.NewDecoder(rr.Body).Decode(&req)

	rr = doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests/"+req.ID+"/approve", map[string]string{
		"approver_id": "user-2", "reason": "ok",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("first approval status = %d, want 200", rr.Code)
	}
	json.NewDecoder(rr.Body).Decode(&req)
	if req.Status != domain.OverridePending {
		t.Fatalf("after 1 approval: status = %s, want pending", req.Status)
	}

	rr = doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests/"+req.ID+"/approve", map[string]string{
		"approver_id": "user-3", "reason": "confirmed",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("second approval status = %d, want 200", rr.Code)
	}
	json.NewDecoder(rr.Body).Decode(&req)
	if req.Status != domain.OverrideApproved {
		t.Fatalf("after 2 approvals: status = %s, want approved", req.Status)
	}
}

func TestOverrideHTTP_SelfApprovalRejected(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 1_000_000, RequiredApprovals: 2,
	})

	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "big",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	var req domain.OverrideRequest
	json.NewDecoder(rr.Body).Decode(&req)

	rr = doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests/"+req.ID+"/approve", map[string]string{
		"approver_id": "user-1", "reason": "self",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "cannot approve") {
		t.Fatalf("body = %q, want 'cannot approve'", rr.Body.String())
	}
}

func TestOverrideHTTP_ReasonRequired(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 1_000_000, RequiredApprovals: 2,
	})

	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestOverrideHTTP_GetRequest(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 1_000_000, RequiredApprovals: 2,
	})

	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "big",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	var req domain.OverrideRequest
	json.NewDecoder(rr.Body).Decode(&req)

	rr = doRequest(t, s.Mux, http.MethodGet, "/api/v1/overrides/requests/"+req.ID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestOverrideHTTP_ListRequests(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 1_000_000, RequiredApprovals: 2,
	})

	for i := 0; i < 2; i++ {
		body := domain.OverrideRequest{
			TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
			OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "big",
		}
		doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	}

	rr := doRequest(t, s.Mux, http.MethodGet, "/api/v1/overrides/requests?tenant_id=tenant-a", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var list []domain.OverrideRequest
	if err := json.NewDecoder(rr.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d requests, want 2", len(list))
	}
}

func TestOverrideHTTP_RejectFlow(t *testing.T) {
	s, _ := newTestServer()
	doRequest(t, s.Mux, http.MethodPut, "/api/v1/overrides/settings", domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 1_000_000, RequiredApprovals: 2,
	})

	body := domain.OverrideRequest{
		TenantID: "tenant-a", RequesterID: "user-1", SKU: "SKU-1", Channel: "web",
		OverrideYen: 5_000_000, BaseYen: 6_000_000, Reason: "big",
	}
	rr := doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests", body)
	var req domain.OverrideRequest
	json.NewDecoder(rr.Body).Decode(&req)

	rr = doRequest(t, s.Mux, http.MethodPost, "/api/v1/overrides/requests/"+req.ID+"/reject", map[string]string{
		"approver_id": "user-2", "reason": "not justified",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	json.NewDecoder(rr.Body).Decode(&req)
	if req.Status != domain.OverrideRejected {
		t.Fatalf("status = %s, want rejected", req.Status)
	}
}
