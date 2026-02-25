.PHONY: build test lint dev run deploy ui-install ui-build ui-dev

APP_ADDR ?= :8080
DATABASE_PATH ?= ./data/mini-atoms-gorm.db
SESSION_SECRET ?= dev-session-secret-change-me
APP_BASE_URL ?= http://localhost:8080

build:
	pnpm run build:css
	go mod tidy && go build ./...

test:
	go test ./...

lint:
	go vet ./...

dev:
	pnpm run build:css
	APP_ADDR='$(APP_ADDR)' DATABASE_PATH='$(DATABASE_PATH)' SESSION_SECRET='$(SESSION_SECRET)' APP_BASE_URL='$(APP_BASE_URL)' go run ./cmd/server

run: dev

ui-install:
	pnpm install

ui-build:
	pnpm run build:css

ui-dev:
	pnpm run dev:css

deploy:
	flyctl deploy
