# Common tasks. `make help` lists them.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/giovannirco/tetra-go/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildDate=$(DATE)
IMAGE   ?= tetra-go:local

.DEFAULT_GOAL := help
.PHONY: help run build test cover lint image up down logs smoke clean vendor-three

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  \033[36m%-13s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: ## Run locally on :8000 (no drain delay)
	DRAIN_DELAY_SECONDS=0 LOG_LEVEL=debug go run -ldflags "$(LDFLAGS)" ./cmd/tetra

build: ## Build bin/tetra
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/tetra ./cmd/tetra

test: ## Unit tests with the race detector
	go test -race ./...

cover: ## Coverage report: coverage.out, coverage.html and the total
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1

lint: ## gofmt and go vet
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

image: ## Build the container image ($(IMAGE))
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(DATE) -t $(IMAGE) .

up: ## Start the app, Prometheus and Grafana with Docker Compose
	VERSION=$(VERSION) COMMIT=$(COMMIT) docker compose up -d --build --wait app prometheus grafana
	@echo "app http://localhost:$${APP_PORT:-8000}  prometheus http://localhost:$${PROMETHEUS_PORT:-9090}  grafana http://localhost:$${GRAFANA_PORT:-3000}"

down: ## Stop the Compose stack
	docker compose --profile test down --remove-orphans

logs: ## Follow the app logs
	docker compose logs -f app

smoke: ## Contract test against the Compose stack
	docker compose --profile test run --rm smoke

vendor-three: ## Re-vendor three.js into web/vendor
	./scripts/vendor-three.sh

clean: ## Remove build and coverage output
	rm -rf bin coverage.out coverage.html
