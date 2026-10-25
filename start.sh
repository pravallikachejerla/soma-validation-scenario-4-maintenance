#!/bin/sh
set -e

# Start PostgreSQL if not running.
if ! pg_isready -h /run/postgresql -q 2>/dev/null; then
  PGBIN=$(ls -d /usr/lib/postgresql/*/bin | head -1)
  mkdir -p /run/postgresql /var/lib/postgresql/data
  chown -R postgres:postgres /run/postgresql /var/lib/postgresql 2>/dev/null || true
  if [ ! -f /var/lib/postgresql/data/PG_VERSION ]; then
    su postgres -c "$PGBIN/initdb -D /var/lib/postgresql/data" >/dev/null 2>&1
  fi
  su postgres -c "$PGBIN/pg_ctl -D /var/lib/postgresql/data -l /tmp/pg.log -o '-k /run/postgresql' start" >/dev/null 2>&1
  sleep 2
  su postgres -c "psql -h /run/postgresql -c \"CREATE USER pricing WITH PASSWORD 'pricing' SUPERUSER;\"" >/dev/null 2>&1 || true
  su postgres -c "psql -h /run/postgresql -c \"CREATE DATABASE pricing OWNER pricing;\"" >/dev/null 2>&1 || true
fi

# Migrate and seed.
export DATABASE_URL="postgres://pricing:pricing@localhost:5432/pricing?host=/run/postgresql"
go run ./cmd/migrate -direction up >/dev/null 2>&1 || true
go run ./cmd/seed >/dev/null 2>&1 || true

# Start API in background.
export LISTEN_ADDR=":8080"
go run ./cmd/api &
API_PID=$!

# Start frontend preview.
cd frontend && npm run preview -- --port 5173 --host 0.0.0.0 &
WEB_PID=$!

# Wait for both.
wait $API_PID $WEB_PID
