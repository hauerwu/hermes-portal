# Hermes Portal — development / build helpers

.PHONY: help build image up down dev-backend dev-frontend test clean

help:
	@echo "Targets:"
	@echo "  build          build the Go backend binary"
	@echo "  image          build the portal docker image"
	@echo "  up             docker compose up -d"
	@echo "  down           docker compose down"
	@echo "  dev-backend    run the backend locally (Go, port 8080)"
	@echo "  dev-frontend   run the frontend dev server (Vite, port 5175)"
	@echo "  test           run all Go tests"
	@echo "  clean          remove build artifacts"

build:
	cd backend && go build -buildvcs=false -o bin/portal ./cmd/portal

test:
	cd backend && go test -buildvcs=false ./...

image:
	docker build -t hermes-portal .

up:
	docker compose up -d

down:
	docker compose down

dev-backend:
	cd backend && PORTAL_STATIC_DIR=../frontend/dist PORTAL_ADMIN_PASSWORD=admin go run -buildvcs=false ./cmd/portal

dev-frontend:
	cd frontend && npm run dev
