// Package httpapi — override_handlers.go
//
// HTTP handlers for the override workflow. All routes are additive
// under /api/v1/overrides/*.
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/somagen/scenario4/internal/application"
)

type OverrideHandler struct {
	svc *application.OverrideService
}

func NewOverrideHandler(svc *application.OverrideService) *OverrideHandler {
	return &OverrideHandler{svc: svc}
}

type createOverrideRequest struct {
	TenantID     string `json:"tenant_id"`
	RequesterID  string `json:"requester_id"`
	SKU          string `json:"sku"`
	Channel      string `json:"channel"`
	BasePriceYen int64  `json:"base_price_yen"`
	OverrideYen  int64  `json:"override_yen"`
	Reason       string `json:"reason"`
}

type approvalRequest struct {
	ApproverID string `json:"approver_id"`
	Reason     string `json:"reason"`
}

func (h *OverrideHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.svc.CreateRequest(req.TenantID, req.RequesterID, req.SKU, req.Channel, req.BasePriceYen, req.OverrideYen, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *OverrideHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path
	if len(id) > len("/api/v1/overrides/") {
		id = id[len("/api/v1/overrides/"):]
	}
	result, err := h.svc.GetRequest(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *OverrideHandler) Approve(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path
	if len(id) > len("/api/v1/overrides/") {
		id = id[len("/api/v1/overrides/"):]
	}
	if len(id) > len("/approve") {
		id = id[:len(id)-len("/approve")]
	}
	var req approvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.svc.Approve(id, req.ApproverID, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *OverrideHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path
	if len(id) > len("/api/v1/overrides/") {
		id = id[len("/api/v1/overrides/"):]
	}
	if len(id) > len("/reject") {
		id = id[:len(id)-len("/reject")]
	}
	var req approvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := h.svc.Reject(id, req.ApproverID, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *OverrideHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant_id")
	results, err := h.svc.ListRequests(tenantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
