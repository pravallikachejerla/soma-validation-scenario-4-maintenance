package application

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/storage"
)

// AdminService is the deterministic seeder.
type AdminService struct {
	Store    storage.Store
	SeedRand int64
}

// NewAdminService returns a service with a fixed seed.
func NewAdminService(s storage.Store, seed int64) *AdminService {
	return &AdminService{Store: s, SeedRand: seed}
}

// SeedProfile describes the volume of synthetic data to generate.
type SeedProfile struct {
	Tenants    int
	Products   int
	Customers  int
	Promotions int
	Channels   []string
}

// DefaultProfile matches the brief: 3 tenants, 200 products, 50 customers,
// 300 promotions.
func DefaultProfile() SeedProfile {
	return SeedProfile{
		Tenants:    3,
		Products:   200,
		Customers:  50,
		Promotions: 300,
		Channels:   []string{"web", "store", "mobile"},
	}
}

// Seed populates the store with a deterministic synthetic dataset. The
// generator is intentionally simple: it does NOT register the overlap
// fixture used by the private condition test; the private seed path is
// responsible for that.
func (a *AdminService) Seed(ctx context.Context, prof SeedProfile) error {
	r := rand.New(rand.NewSource(a.SeedRand))
	tenantIDs := make([]string, 0, prof.Tenants)
	for i := 0; i < prof.Tenants; i++ {
		tid := fmt.Sprintf("tenant-%c", 'a'+i)
		tenantIDs = append(tenantIDs, tid)
		if err := a.Store.UpsertTenant(ctx, domain.Tenant{ID: tid, Name: "Synthetic " + tid}); err != nil {
			return err
		}
	}
	for _, tid := range tenantIDs {
		for i := 0; i < prof.Products; i++ {
			sku := fmt.Sprintf("SKU-%05d", i+1)
			if err := a.Store.UpsertProduct(ctx, domain.Product{
				ID:       tid + "/" + sku,
				TenantID: tid,
				SKU:      sku,
				Name:     "Product " + sku,
				BaseYen:  int64(1000 + r.Intn(9000)),
			}); err != nil {
				return err
			}
		}
		for i := 0; i < prof.Customers; i++ {
			cid := fmt.Sprintf("cust-%03d", i+1)
			if err := a.Store.UpsertCustomer(ctx, domain.Customer{
				ID:       cid,
				TenantID: tid,
				Name:     "Customer " + cid,
				Segment:  pickSegment(r),
			}); err != nil {
				return err
			}
		}
		validFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		validUntil := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < prof.Promotions; i++ {
			pid := fmt.Sprintf("promo-%04d", i+1)
			ch := prof.Channels[i%len(prof.Channels)]
			var sku string
			// 70% of promotions are channel-wildcard (no SKU), 30%
			// are SKU-specific. This mirrors a realistic mix and
			// keeps the dataset rich without overlapping the same
			// promotion across both paths during public seeding.
			if r.Float64() < 0.3 {
				sku = fmt.Sprintf("SKU-%05d", 1+r.Intn(prof.Products))
			}
			typ := domain.PromotionPercent
			val := 5.0 + r.Float64()*15.0
			if r.Float64() < 0.2 {
				typ = domain.PromotionAmount
				val = 100 + float64(r.Intn(900))
			}
			if err := a.Store.UpsertPromotion(ctx, domain.Promotion{
				ID:         pid,
				TenantID:   tid,
				Name:       "Promotion " + pid,
				Type:       typ,
				Value:      val,
				Channel:    ch,
				SKU:        sku,
				Priority:   r.Intn(10),
				ValidFrom:  validFrom,
				ValidUntil: validUntil,
			}); err != nil {
				return err
			}
		}
		if _, err := a.Store.BumpConfigVersion(ctx, tid); err != nil {
			return err
		}
	}
	return nil
}

func pickSegment(r *rand.Rand) string {
	segs := []string{"retail", "wholesale", "vip"}
	return segs[r.Intn(len(segs))]
}
