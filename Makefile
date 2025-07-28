run:
	@go run *.go

dev:
	# Starts Overmind to mintor Procfile ..
	@pkgx overmind s

server:
	@go run ./cmd/server

build:
	@go build -o bin/timeline-server ./cmd/server

test:
	@go test ./pkg/timeline/ ./pkg/temporal/ -v

test-all:
	@go test ./... -v

check:
	@go mod tidy
	@go vet ./...
	@golangci-lint run || echo "golangci-lint not installed, skipping"

coverage:
	@go test ./pkg/timeline/ ./pkg/temporal/ -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean:
	@rm -f bin/timeline-server coverage.out coverage.html

watch:
	@pkgx watch --color jj --ignore-working-copy log --color=always

detailed:
	@jj log -T builtin_log_detailed

evolog:
	@jj evolog -p

squash:
	@jj squash

new:
	@jj new main

push:
	@echo "Random new branch for main .."
	@jj git push

.PHONY: run server build test test-all check coverage clean

