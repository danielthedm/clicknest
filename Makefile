.PHONY: build sdk web dev clean test embed docker

# Build the full binary (SDK + Web + Go)
build: sdk web embed
	go build -o clicknest ./cmd/clicknest/

# Build SDK
sdk:
	cd sdk && npm install && npm run build

# Build SvelteKit frontend
web:
	cd web && npm install && npm run build

# Copy build artifacts into cmd/clicknest/ for go:embed
embed: sdk web
	rm -rf cmd/clicknest/web_build cmd/clicknest/sdk_dist
	cp -r web/build cmd/clicknest/web_build
	cp -r sdk/dist cmd/clicknest/sdk_dist

# Run in development mode (no frontend build needed)
dev:
	go run ./cmd/clicknest/ -dev -addr :8080

# Run backend + frontend dev servers together (with hot reload)
dev-all:
	@which air > /dev/null 2>&1 || go install github.com/air-verse/air@latest
	$(shell which air || echo ~/go/bin/air) & cd web && npm run dev; kill %1 2>/dev/null

# Run tests
test:
	go test ./...

# Clean build artifacts
clean:
	rm -f clicknest
	rm -rf sdk/dist web/build data/
	rm -rf cmd/clicknest/web_build cmd/clicknest/sdk_dist

# Docker build
docker:
	docker build -t clicknest .
