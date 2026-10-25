// Command migrate runs the SQL migrations against the configured
// database. Direction "up" applies 0001_init.sql; direction "down"
// applies 0001_init.down.sql.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/somagen/scenario4/internal/storage"
)

func main() {
	dir := flag.String("dir", "migrations", "directory holding the SQL migration files")
	direction := flag.String("direction", "up", "migration direction: up or down")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://pricing:pricing@localhost:5432/pricing?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ps, err := storage.NewPostgresStore(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer ps.Close(context.Background())

	pattern := "0001_init"
	if *direction == "down" {
		pattern = "0001_init.down"
	}
	// Resolve the absolute path so this works regardless of CWD.
	abs, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("resolve dir: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(abs, pattern+"*.sql"))
	if err != nil {
		log.Fatalf("glob: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		log.Fatalf("no migration files matched %s in %s", pattern, abs)
	}
	for _, f := range files {
		fmt.Printf("applying %s ...\n", f)
		body, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("read %s: %v", f, err)
		}
		if _, err := ps.Pool().Exec(ctx, string(body)); err != nil {
			log.Fatalf("apply %s: %v", f, err)
		}
	}
	fmt.Println("migrate: complete")
}
