.PHONY: build test lint cover ui gen run clean

BINARY := grove

build:
	go build -o $(BINARY) ./cmd/grove

# Release build with the embedded web UI (requires `make ui` first).
build-release: ui
	go build -tags embedui -o $(BINARY) ./cmd/grove

test:
	go test -race ./...

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run

ui:
	cd ui && npm ci && npm run build

# Regenerate derived code (sqlc queries, tygo TS types). CI verifies output is committed.
gen:
	@echo "gen: nothing to generate yet"

run: build
	./$(BINARY) serve

clean:
	rm -f $(BINARY) coverage.out coverage.html
