package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/somagen/scenario4/internal/domain"
)

// PostgresStore is the PostgreSQL implementation of Store. All queries use
// positional placeholders and NEVER concatenate user input into SQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore opens a pgx pool against the given DSN.
func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 16
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &PostgresStore{pool: pool}, nil
}

// Close releases the pool.
func (s *PostgresStore) Close(ctx context.Context) error {
	s.pool.Close()
	return nil
}

// Pool exposes the underlying pool for the migrate command.
func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

func (s *PostgresStore) UpsertTenant(ctx context.Context, t domain.Tenant) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenants (id, name, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name
	`, t.ID, t.Name, t.CreatedAt)
	return err
}

func (s *PostgresStore) GetTenant(ctx context.Context, id string) (domain.Tenant, error) {
	var t domain.Tenant
	err := s.pool.QueryRow(ctx, `SELECT id, name, created_at FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.Name, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tenant{}, ErrNotFound
	}
	return t, err
}

func (s *PostgresStore) ListTenants(ctx context.Context) ([]domain.Tenant, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, created_at FROM tenants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Tenant, 0)
	for rows.Next() {
		var t domain.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpsertUser(ctx context.Context, u domain.User) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, tenant_id, name, role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, role = EXCLUDED.role
	`, u.ID, u.TenantID, u.Name, u.Role)
	return err
}

func (s *PostgresStore) UpsertProduct(ctx context.Context, p domain.Product) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO products (id, tenant_id, sku, name, base_yen)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, base_yen = EXCLUDED.base_yen
	`, p.ID, p.TenantID, p.SKU, p.Name, p.BaseYen)
	return err
}

func (s *PostgresStore) ListProducts(ctx context.Context, tenantID string) ([]domain.Product, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, sku, name, base_yen
		FROM products WHERE tenant_id = $1 ORDER BY sku
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Product, 0)
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.TenantID, &p.SKU, &p.Name, &p.BaseYen); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetProduct(ctx context.Context, tenantID, sku string) (domain.Product, error) {
	var p domain.Product
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, sku, name, base_yen
		FROM products WHERE tenant_id = $1 AND sku = $2
	`, tenantID, sku).Scan(&p.ID, &p.TenantID, &p.SKU, &p.Name, &p.BaseYen)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Product{}, ErrNotFound
	}
	return p, err
}

func (s *PostgresStore) UpsertCustomer(ctx context.Context, c domain.Customer) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO customers (id, tenant_id, name, segment)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, segment = EXCLUDED.segment
	`, c.ID, c.TenantID, c.Name, c.Segment)
	return err
}

func (s *PostgresStore) GetCustomer(ctx context.Context, tenantID, id string) (domain.Customer, error) {
	var c domain.Customer
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, segment FROM customers WHERE id = $1 AND tenant_id = $2
	`, id, tenantID).Scan(&c.ID, &c.TenantID, &c.Name, &c.Segment)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Customer{}, ErrNotFound
	}
	return c, err
}

func (s *PostgresStore) UpsertPromotion(ctx context.Context, p domain.Promotion) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO promotions (id, tenant_id, name, type, value, channel, sku, priority, valid_from, valid_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id, sku) DO UPDATE SET
			name = EXCLUDED.name, type = EXCLUDED.type, value = EXCLUDED.value,
			channel = EXCLUDED.channel, priority = EXCLUDED.priority,
			valid_from = EXCLUDED.valid_from, valid_until = EXCLUDED.valid_until
	`, p.ID, p.TenantID, p.Name, string(p.Type), p.Value, p.Channel, p.SKU, p.Priority, p.ValidFrom, p.ValidUntil)
	return err
}

func (s *PostgresStore) UpdatePromotion(ctx context.Context, p domain.Promotion) error {
	res, err := s.pool.Exec(ctx, `
		UPDATE promotions SET name = $2, type = $3, value = $4, channel = $5,
			priority = $6, valid_from = $7, valid_until = $8
		WHERE id = $1 AND sku = $9
	`, p.ID, p.Name, string(p.Type), p.Value, p.Channel, p.Priority, p.ValidFrom, p.ValidUntil, p.SKU)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListPromotions(ctx context.Context, tenantID, channel string) ([]domain.Promotion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, type, value, channel, COALESCE(sku, ''), priority, valid_from, valid_until
		FROM promotions
		WHERE tenant_id = $1 AND ($2 = '' OR channel = $2)
		ORDER BY priority DESC, id ASC
	`, tenantID, channel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Promotion, 0)
	for rows.Next() {
		var p domain.Promotion
		var t string
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &t, &p.Value, &p.Channel, &p.SKU, &p.Priority, &p.ValidFrom, &p.ValidUntil); err != nil {
			return nil, err
		}
		p.Type = domain.PromotionType(t)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetPromotion(ctx context.Context, id string) (domain.Promotion, error) {
	var p domain.Promotion
	var t string
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, type, value, channel, sku, priority, valid_from, valid_until
		FROM promotions WHERE id = $1 ORDER BY sku LIMIT 1
	`, id).Scan(&p.ID, &p.TenantID, &p.Name, &t, &p.Value, &p.Channel, &p.SKU, &p.Priority, &p.ValidFrom, &p.ValidUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Promotion{}, ErrNotFound
	}
	if err != nil {
		return domain.Promotion{}, err
	}
	p.Type = domain.PromotionType(t)
	return p, nil
}

// SelectCandidates returns every active promotion for the tenant on
// `channel` whose valid window covers `at`. The end-of-window comparison
// is INCLUSIVE (<=).
func (s *PostgresStore) SelectCandidates(ctx context.Context, tenantID, sku, channel string, at time.Time) ([]domain.Promotion, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, name, type, value, channel, COALESCE(sku, ''), priority, valid_from, valid_until
		FROM promotions
		WHERE tenant_id = $1
		  AND channel = $2
		  AND valid_from <= $3
		  AND valid_until >= $3
		ORDER BY priority DESC, id ASC
	`, tenantID, channel, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Promotion, 0)
	for rows.Next() {
		var p domain.Promotion
		var t string
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &t, &p.Value, &p.Channel, &p.SKU, &p.Priority, &p.ValidFrom, &p.ValidUntil); err != nil {
			return nil, err
		}
		p.Type = domain.PromotionType(t)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) SaveDecision(ctx context.Context, d domain.PricingDecision) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO pricing_decisions (request_hash, base_yen, final_yen, applied_ids, audit_ids, explanation, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (request_hash) DO UPDATE SET final_yen = EXCLUDED.final_yen
	`, d.RequestHash, d.BaseYen, d.FinalYen, d.AppliedIDs, d.AuditIDs, d.ExplanationLines)
	return err
}

func (s *PostgresStore) GetDecision(ctx context.Context, requestHash string) (domain.PricingDecision, bool, error) {
	var d domain.PricingDecision
	err := s.pool.QueryRow(ctx, `
		SELECT request_hash, base_yen, final_yen, applied_ids, audit_ids, explanation
		FROM pricing_decisions WHERE request_hash = $1
	`, requestHash).Scan(&d.RequestHash, &d.BaseYen, &d.FinalYen, &d.AppliedIDs, &d.AuditIDs, &d.ExplanationLines)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PricingDecision{}, false, nil
	}
	if err != nil {
		return domain.PricingDecision{}, false, err
	}
	return d, true, nil
}

func (s *PostgresStore) CreateBatchJob(ctx context.Context, j domain.BatchJob) error {
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO batch_jobs (id, tenant_id, created_at, total_rows, done_rows, status, result_hash)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
	`, j.ID, j.TenantID, j.CreatedAt, j.TotalRows, j.DoneRows, j.Status, j.ResultHash)
	return err
}

func (s *PostgresStore) UpdateBatchJob(ctx context.Context, j domain.BatchJob) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE batch_jobs SET done_rows = $2, status = $3, result_hash = NULLIF($4, '')
		WHERE id = $1
	`, j.ID, j.DoneRows, j.Status, j.ResultHash)
	return err
}

func (s *PostgresStore) GetBatchJob(ctx context.Context, id string) (domain.BatchJob, error) {
	var j domain.BatchJob
	err := s.pool.QueryRow(ctx, `
		SELECT id, tenant_id, created_at, total_rows, done_rows, status, COALESCE(result_hash, '')
		FROM batch_jobs WHERE id = $1
	`, id).Scan(&j.ID, &j.TenantID, &j.CreatedAt, &j.TotalRows, &j.DoneRows, &j.Status, &j.ResultHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BatchJob{}, ErrNotFound
	}
	return j, err
}

func (s *PostgresStore) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_events (id, tenant_id, action, subject, detail, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, e.ID, e.TenantID, e.Action, e.Subject, e.Detail, e.CreatedAt)
	return err
}

func (s *PostgresStore) ListAudit(ctx context.Context, tenantID string, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, tenant_id, action, subject, detail, created_at
		FROM audit_events
		WHERE $1 = '' OR tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var e domain.AuditEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Action, &e.Subject, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetConfigVersion(ctx context.Context, tenantID string) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `
		SELECT version FROM config_versions WHERE tenant_id = $1
	`, tenantID).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		// Auto-seed version 1 on first read.
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO config_versions (tenant_id, version) VALUES ($1, 1)
			ON CONFLICT (tenant_id) DO NOTHING
		`, tenantID); err != nil {
			return 0, err
		}
		return 1, nil
	}
	return v, err
}

func (s *PostgresStore) BumpConfigVersion(ctx context.Context, tenantID string) (int64, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO config_versions (tenant_id, version) VALUES ($1, 1)
		ON CONFLICT (tenant_id) DO UPDATE SET version = config_versions.version + 1
	`, tenantID)
	if err != nil {
		return 0, err
	}
	return s.GetConfigVersion(ctx, tenantID)
}

// helper for the seeder to run ad-hoc parameterised SQL.
func (s *PostgresStore) Exec(ctx context.Context, sql string, args ...any) error {
	if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}
