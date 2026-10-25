// Package storage provides the persistence boundary for the pricing
// application. The same interface is satisfied by two backends: an
// in-process memory store (used by tests and by the docker-compose `seed`
// step) and a PostgreSQL store (used by the production-shaped deployment).
//
// The two implementations MUST agree on observable behaviour: the same
// query against the same logical state must return the same logical
// result. Both use INCLUSIVE end-of-window comparisons (`<= end`).
package storage

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/somagen/scenario4/internal/domain"
)

// Store is the storage interface used by the application layer.
type Store interface {
	TenantRepo
	UserRepo
	ProductRepo
	CustomerRepo
	PromotionRepo
	PricingRepo
	BatchJobRepo
	AuditRepo
	ConfigRepo
	OverrideRepo
	Close(ctx context.Context) error
}

type TenantRepo interface {
	UpsertTenant(ctx context.Context, t domain.Tenant) error
	GetTenant(ctx context.Context, id string) (domain.Tenant, error)
	ListTenants(ctx context.Context) ([]domain.Tenant, error)
}

type UserRepo interface {
	UpsertUser(ctx context.Context, u domain.User) error
}

type ProductRepo interface {
	UpsertProduct(ctx context.Context, p domain.Product) error
	ListProducts(ctx context.Context, tenantID string) ([]domain.Product, error)
	GetProduct(ctx context.Context, tenantID, sku string) (domain.Product, error)
}

type CustomerRepo interface {
	UpsertCustomer(ctx context.Context, c domain.Customer) error
	GetCustomer(ctx context.Context, tenantID, id string) (domain.Customer, error)
}

type PromotionRepo interface {
	UpsertPromotion(ctx context.Context, p domain.Promotion) error
	UpdatePromotion(ctx context.Context, p domain.Promotion) error
	ListPromotions(ctx context.Context, tenantID, channel string) ([]domain.Promotion, error)
	GetPromotion(ctx context.Context, id string) (domain.Promotion, error)
}

type PricingRepo interface {
	SelectCandidates(ctx context.Context, tenantID, sku, channel string, at time.Time) ([]domain.Promotion, error)
	SaveDecision(ctx context.Context, d domain.PricingDecision) error
	GetDecision(ctx context.Context, requestHash string) (domain.PricingDecision, bool, error)
}

type BatchJobRepo interface {
	CreateBatchJob(ctx context.Context, j domain.BatchJob) error
	UpdateBatchJob(ctx context.Context, j domain.BatchJob) error
	GetBatchJob(ctx context.Context, id string) (domain.BatchJob, error)
}

type AuditRepo interface {
	AppendAudit(ctx context.Context, e domain.AuditEvent) error
	ListAudit(ctx context.Context, tenantID string, limit int) ([]domain.AuditEvent, error)
}

type ConfigRepo interface {
	GetConfigVersion(ctx context.Context, tenantID string) (int64, error)
	BumpConfigVersion(ctx context.Context, tenantID string) (int64, error)
}

type OverrideRepo interface {
	GetOverrideSettings(ctx context.Context, tenantID string) (domain.TenantOverrideSettings, error)
	UpsertOverrideSettings(ctx context.Context, s domain.TenantOverrideSettings) error
	CreateOverrideRequest(ctx context.Context, r domain.OverrideRequest) error
	GetOverrideRequest(ctx context.Context, id string) (domain.OverrideRequest, error)
	ListOverrideRequests(ctx context.Context, tenantID string) ([]domain.OverrideRequest, error)
	UpdateOverrideRequest(ctx context.Context, r domain.OverrideRequest) error
	AppendOverrideAudit(ctx context.Context, requestID, action, actorID, reason string) error
	ListOverrideAudit(ctx context.Context, requestID string) ([]OverrideAuditEntry, error)
}

// OverrideAuditEntry is a single audit record for an override action.
type OverrideAuditEntry struct {
	RequestID string
	Action    string
	ActorID   string
	Reason    string
	At        time.Time
}

var (
	ErrOverrideNotFound  = errors.New("override not found")
	ErrDuplicateApprover = errors.New("duplicate approver")
	ErrSelfApproval      = errors.New("requester cannot approve own request")
)

// MemoryStore is an in-process implementation of Store. All access is
// guarded by a single RWMutex; this is fine for unit tests but not for
// real concurrent throughput (the production backend is Postgres).
type MemoryStore struct {
	mu sync.RWMutex

	tenants    map[string]domain.Tenant
	users      map[string]domain.User
	products   map[string]domain.Product    // key = tenantID + "|" + sku
	customers  map[string]domain.Customer   // key = tenantID + "|" + id
	promotions map[string]domain.Promotion  // key = id
	decisions  map[string]domain.PricingDecision
	batchJobs  map[string]domain.BatchJob
	audits     []domain.AuditEvent
	configs    map[string]int64 // tenantID -> version

	overrideSettings  map[string]domain.TenantOverrideSettings
	overrideRequests  map[string]domain.OverrideRequest
	overrideAudit     map[string][]OverrideAuditEntry
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tenants:    make(map[string]domain.Tenant),
		users:      make(map[string]domain.User),
		products:   make(map[string]domain.Product),
		customers:  make(map[string]domain.Customer),
		promotions: make(map[string]domain.Promotion),
		decisions:  make(map[string]domain.PricingDecision),
		batchJobs:  make(map[string]domain.BatchJob),
		configs:    make(map[string]int64),

		overrideSettings: make(map[string]domain.TenantOverrideSettings),
		overrideRequests: make(map[string]domain.OverrideRequest),
		overrideAudit:    make(map[string][]OverrideAuditEntry),
	}
}

// GetOverrideSettings returns the settings for a tenant, or defaults if unset.
func (m *MemoryStore) GetOverrideSettings(ctx context.Context, tenantID string) (domain.TenantOverrideSettings, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.overrideSettings[tenantID]; ok {
		return s, nil
	}
	return domain.TenantOverrideSettings{
		TenantID:          tenantID,
		ThresholdYen:      domain.DefaultThresholdYen,
		RequiredApprovals: domain.DefaultRequiredApprovals,
		OverrideEnabled:   false,
	}, nil
}

// UpsertOverrideSettings creates or updates the settings for a tenant.
func (m *MemoryStore) UpsertOverrideSettings(ctx context.Context, s domain.TenantOverrideSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overrideSettings[s.TenantID] = s
	return nil
}

// CreateOverrideRequest stores a new override request.
func (m *MemoryStore) CreateOverrideRequest(ctx context.Context, r domain.OverrideRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overrideRequests[r.ID] = r
	return nil
}

// GetOverrideRequest returns an override request by id.
func (m *MemoryStore) GetOverrideRequest(ctx context.Context, id string) (domain.OverrideRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.overrideRequests[id]; ok {
		return r, nil
	}
	return domain.OverrideRequest{}, ErrOverrideNotFound
}

// ListOverrideRequests returns all override requests for a tenant.
func (m *MemoryStore) ListOverrideRequests(ctx context.Context, tenantID string) ([]domain.OverrideRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var results []domain.OverrideRequest
	for _, r := range m.overrideRequests {
		if r.TenantID == tenantID {
			results = append(results, r)
		}
	}
	return results, nil
}

// UpdateOverrideRequest updates an existing override request.
func (m *MemoryStore) UpdateOverrideRequest(ctx context.Context, r domain.OverrideRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overrideRequests[r.ID] = r
	return nil
}

// AppendOverrideAudit appends an audit entry for an override action.
func (m *MemoryStore) AppendOverrideAudit(ctx context.Context, requestID, action, actorID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.overrideAudit[requestID] = append(m.overrideAudit[requestID], OverrideAuditEntry{
		RequestID: requestID,
		Action:    action,
		ActorID:   actorID,
		Reason:    reason,
		At:        time.Now(),
	})
	return nil
}

// ListOverrideAudit returns all audit entries for an override request.
func (m *MemoryStore) ListOverrideAudit(ctx context.Context, requestID string) ([]OverrideAuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.overrideAudit[requestID], nil
}

// Close is a no-op for the in-memory store.
func (s *MemoryStore) Close(ctx context.Context) error { return nil }

func (s *MemoryStore) UpsertTenant(ctx context.Context, t domain.Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	s.tenants[t.ID] = t
	return nil
}

func (s *MemoryStore) GetTenant(ctx context.Context, id string) (domain.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tenants[id]
	if !ok {
		return domain.Tenant{}, ErrNotFound
	}
	return t, nil
}

func (s *MemoryStore) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemoryStore) UpsertUser(ctx context.Context, u domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = u
	return nil
}

func (s *MemoryStore) productKey(tenant, sku string) string { return tenant + "|" + sku }

func (s *MemoryStore) UpsertProduct(ctx context.Context, p domain.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.products[s.productKey(p.TenantID, p.SKU)] = p
	return nil
}

func (s *MemoryStore) ListProducts(ctx context.Context, tenantID string) ([]domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Product, 0)
	for _, p := range s.products {
		if p.TenantID == tenantID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out, nil
}

func (s *MemoryStore) GetProduct(ctx context.Context, tenantID, sku string) (domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.products[s.productKey(tenantID, sku)]
	if !ok {
		return domain.Product{}, ErrNotFound
	}
	return p, nil
}

func (s *MemoryStore) customerKey(tenant, id string) string { return tenant + "|" + id }

func (s *MemoryStore) UpsertCustomer(ctx context.Context, c domain.Customer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.customers[s.customerKey(c.TenantID, c.ID)] = c
	return nil
}

func (s *MemoryStore) GetCustomer(ctx context.Context, tenantID, id string) (domain.Customer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.customers[s.customerKey(tenantID, id)]
	if !ok {
		return domain.Customer{}, ErrNotFound
	}
	return c, nil
}

// promotionKey is the (id, sku) composite key. The same promotion id can
// be registered with multiple SKUs (e.g. one SKU-specific rule and one
// channel-wildcard rule); the storage layer treats each (id, sku)
// combination as a separate rule row.
func (s *MemoryStore) promotionKey(id, sku string) string { return id + "|" + sku }

func (s *MemoryStore) UpsertPromotion(ctx context.Context, p domain.Promotion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.promotions[s.promotionKey(p.ID, p.SKU)] = p
	return nil
}

func (s *MemoryStore) UpdatePromotion(ctx context.Context, p domain.Promotion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.promotions[s.promotionKey(p.ID, p.SKU)]; !ok {
		return ErrNotFound
	}
	s.promotions[s.promotionKey(p.ID, p.SKU)] = p
	return nil
}

func (s *MemoryStore) ListPromotions(ctx context.Context, tenantID, channel string) ([]domain.Promotion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Promotion, 0)
	for _, p := range s.promotions {
		if p.TenantID != tenantID {
			continue
		}
		if channel != "" && p.Channel != channel {
			continue
		}
		out = append(out, p)
	}
	// Stable order for callers: priority DESC, then ID ASC.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// GetPromotion returns the first promotion rule row matching id. The
// (id, sku) composite is the storage key; callers that registered the
// same id with multiple SKUs receive whichever rule row is iterated
// first.
func (s *MemoryStore) GetPromotion(ctx context.Context, id string) (domain.Promotion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, p := range s.promotions {
		if p.ID == id {
			_ = k
			return p, nil
		}
	}
	return domain.Promotion{}, ErrNotFound
}

// SelectCandidates returns every active promotion for the tenant on
// `channel` whose valid window covers `at`. The end-of-window comparison
// is INCLUSIVE (use <= end).
func (s *MemoryStore) SelectCandidates(ctx context.Context, tenantID, sku, channel string, at time.Time) ([]domain.Promotion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Promotion, 0)
	for _, p := range s.promotions {
		if p.TenantID != tenantID {
			continue
		}
		if p.Channel != channel {
			continue
		}
		// INCLUSIVE on both ends: a promotion is active for the entire
		// half-open interval [valid_from, valid_until].
		if at.Before(p.ValidFrom) || at.After(p.ValidUntil) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *MemoryStore) SaveDecision(ctx context.Context, d domain.PricingDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions[d.RequestHash] = d
	return nil
}

func (s *MemoryStore) GetDecision(ctx context.Context, requestHash string) (domain.PricingDecision, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.decisions[requestHash]
	return d, ok, nil
}

func (s *MemoryStore) CreateBatchJob(ctx context.Context, j domain.BatchJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
	s.batchJobs[j.ID] = j
	return nil
}

func (s *MemoryStore) UpdateBatchJob(ctx context.Context, j domain.BatchJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchJobs[j.ID] = j
	return nil
}

func (s *MemoryStore) GetBatchJob(ctx context.Context, id string) (domain.BatchJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.batchJobs[id]
	if !ok {
		return domain.BatchJob{}, ErrNotFound
	}
	return j, nil
}

func (s *MemoryStore) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	s.audits = append(s.audits, e)
	return nil
}

func (s *MemoryStore) ListAudit(ctx context.Context, tenantID string, limit int) ([]domain.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.AuditEvent, 0)
	for i := len(s.audits) - 1; i >= 0 && len(out) < limit; i-- {
		if tenantID == "" || s.audits[i].TenantID == tenantID {
			out = append(out, s.audits[i])
		}
	}
	return out, nil
}

func (s *MemoryStore) GetConfigVersion(ctx context.Context, tenantID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.configs[tenantID]
	if !ok {
		return 1, nil
	}
	return v, nil
}

func (s *MemoryStore) BumpConfigVersion(ctx context.Context, tenantID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.configs[tenantID]++
	return s.configs[tenantID], nil
}

// ErrNotFound is returned by memory store lookups that miss.
var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
