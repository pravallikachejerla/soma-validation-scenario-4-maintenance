// Command seed populates the database with a deterministic synthetic
// dataset. It is intended to be run from docker-compose after the
// migrations complete.
//
// The PUBLIC seeder does NOT register the overlap fixture used by the
// private condition test. A separate private seeder (in package private)
// is responsible for inserting that fixture. The overlap row in
// testdata/fixtures/overlap.json is read by the private seed path only.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/storage"
)

type overlapFixture struct {
	Promotion domain.Promotion `json:"promotion"`
	// SKURule is the SKU-specific rule row for the same promotion id.
	SKURule domain.Promotion `json:"sku_rule"`
	// WildcardRule is the channel-wildcard rule row for the same id.
	WildcardRule domain.Promotion `json:"wildcard_rule"`
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://pricing:pricing@localhost:5432/pricing?sslmode=disable"
	}
	profile := os.Getenv("SEED_PROFILE")
	withOverlap := os.Getenv("SEED_WITH_OVERLAP") == "1"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var store storage.Store
	ps, err := storage.NewPostgresStore(ctx, dsn)
	if err != nil {
		log.Printf("postgres init failed, falling back to memory: %v", err)
		store = storage.NewMemoryStore()
	} else {
		store = ps
	}
	defer store.Close(context.Background())

	logger := observability.Default("seed")
	_ = logger
	as := application.NewAdminService(store, 42)
	prof := application.DefaultProfile()
	if profile == "small" {
		prof = application.SeedProfile{Tenants: 1, Products: 5, Customers: 2, Promotions: 4, Channels: []string{"web"}}
	}
	if err := as.Seed(ctx, prof); err != nil {
		log.Fatalf("seed: %v", err)
	}

	if withOverlap {
		fixture, err := loadOverlap("testdata/fixtures/overlap.json")
		if err != nil {
			log.Fatalf("load overlap: %v", err)
		}
		if err := store.UpsertPromotion(ctx, fixture.Promotion); err != nil {
			log.Fatalf("upsert overlap promo: %v", err)
		}
		if err := store.UpsertPromotion(ctx, fixture.SKURule); err != nil {
			log.Fatalf("upsert overlap sku rule: %v", err)
		}
		if err := store.UpsertPromotion(ctx, fixture.WildcardRule); err != nil {
			log.Fatalf("upsert overlap wildcard rule: %v", err)
		}
		_, _ = store.BumpConfigVersion(ctx, fixture.Promotion.TenantID)
		fmt.Println("seed: overlap fixture installed")
	}
	fmt.Println("seed: complete")
}

func loadOverlap(path string) (overlapFixture, error) {
	var f overlapFixture
	b, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	return f, nil
}
