// Command migrate runs the SQL migrations against the configured
// database. Direction "up" applies every NNNN_*.sql file in sorted
// order (so 0001 then 0002 then ...); direction "down" applies every
// NNNN_*.down.sql file in REVERSE sorted order.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ps, err := storage.NewPostgresStore(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer ps.Close(context.Background())

	abs, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatalf("resolve dir: %v", err)
	}

	var files []string
	if *direction == "down" {
		// Apply down files in reverse sorted order.
		pattern := filepath.Join(abs, "*.down.sql")
		files, err = filepath.Glob(pattern)
		if err != nil {
			log.Fatalf("glob: %v", err)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(files)))
	} else {
		// Apply up files in sorted order, skipping the .down.sql
		// siblings (which are paired with their up file).
		pattern := filepath.Join(abs, "*.sql")
		all, err := filepath.Glob(pattern)
		if err != nil {
			log.Fatalf("glob: %v", err)
		}
		files = all[:0]
		for _, f := range all {
			if strings.HasSuffix(f, ".down.sql") {
				continue
			}
			files = append(files, f)
		}
		sort.Strings(files)
	}

	if len(files) == 0 {
		log.Fatalf("no migration files found in %s (direction=%s)", abs, *direction)
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
	fmt.Printf("migrate: complete (%d file(s), direction=%s)\n", len(files), *direction)
}
