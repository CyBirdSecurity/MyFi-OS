.PHONY: dev-api dev-web build test migrate-up migrate-down docker-up docker-down seed reseed-demo reset-password backup restore

# Run the Go API server in development mode (requires .env or env vars)
dev-api:
	ENV=development go run ./cmd/server

# Run the Vite dev server for the frontend
dev-web:
	cd web && npm run dev

# Build everything (frontend + Go binary)
build:
	cd web && npm run build
	go build -o bin/myfi ./cmd/server

# Run all Go tests
test:
	go test ./...

# Apply all pending migrations (requires DATABASE_URL)
migrate-up:
	@echo "Migrations run automatically on server startup."
	@echo "Start the server to apply migrations, or use migrate CLI directly:"
	@echo "  migrate -source file://migrations -database $$DATABASE_URL up"

# Roll back one migration (requires migrate CLI)
migrate-down:
	migrate -source file://migrations -database "$$DATABASE_URL" down 1

# Start the full stack with docker compose
docker-up:
	docker compose up --build

# Stop the docker compose services
docker-down:
	docker compose down

# Install frontend dependencies
install:
	cd web && npm install

# Seed a fresh (not yet set up) instance with demo data (requires the docker compose stack to be running)
seed: ## Login: demo@myfi.local / demo1234
	docker compose exec -T db psql -U myfi myfi < scripts/seed.sql

# Drop the demo account and reseed it
reseed-demo:
	docker compose exec -T db psql -U myfi myfi -c "DELETE FROM users WHERE email = 'demo@myfi.local';"
	$(MAKE) seed

# Set a new password for the instance's account (prompts for it)
reset-password:
	docker compose exec -it app /bin/myfi reset-password

# Back up the database to backups/myfi_<timestamp>.sql.gz
backup:
	./scripts/backup.sh

# Restore the database from a backup file: make restore FILE=backups/myfi_20260101_120000.sql.gz
restore:
	./scripts/restore.sh $(FILE)
