# Contributing to ClickNest

Thanks for your interest in contributing. This document covers how to get the project running locally and the basics of how it's structured.

## Dev setup

**Requirements:** Go 1.23+, Node 20+

```bash
# Clone and install dependencies
git clone https://github.com/danielthedm/clicknest
cd clicknest

# Install frontend + SDK dependencies
cd web && npm install && cd ..
cd sdk && npm install && cd ..
```

**Run in development mode** (backend only, no frontend build needed):
```bash
make dev
```
This starts the Go server at `http://localhost:8080` with a placeholder frontend. Your API key is printed to the terminal on first run.

**Run with hot-reload frontend** (full stack):
```bash
make dev-all
```
This runs the Go backend and SvelteKit dev server together. The frontend is available at `http://localhost:5173` and proxies API calls to `:8080`.

## Project structure

```
cmd/clicknest/       # Main binary entrypoint
internal/
  auth/              # API key + session middleware
  ai/                # LLM provider abstraction, event naming, AI chat
  github/            # GitHub repo sync + source code matcher
  ingest/            # SDK event ingestion handler + validator
  query/             # Dashboard query handlers (events, sessions, funnels, etc.)
  server/            # HTTP server, routes, feature flag/alert/backup handlers
  storage/           # DuckDB (events) + SQLite (metadata) + encryption
sdk/                 # TypeScript browser SDK (<2KB gzipped)
web/                 # SvelteKit dashboard frontend
```

**Two databases:**
- `events.duckdb` — all captured events (DuckDB, columnar, fast aggregations)
- `clicknest.db` — metadata: projects, API keys, AI name cache, flags, alerts, config (SQLite)

**Adding a new API endpoint:**
1. Write the handler (in `internal/query/` for event queries, or directly on `*Server` in `internal/server/server.go` for everything else)
2. Register the route in `routes()` in `internal/server/server.go`
3. Add the API function to `web/src/lib/api.ts`
4. Add any new types to `web/src/lib/types.ts`

**Adding a SQLite migration:**
Create `internal/storage/migrations/sqlite/NNN_description.sql` — migrations run automatically on startup in filename order.

## Running tests

```bash
go test ./...
```

## Building a release binary

```bash
make build
./clicknest -data ./data
```

This compiles the SvelteKit frontend and SDK, embeds them into the Go binary, and produces a single self-contained `clicknest` executable.

## Pull requests

- Keep PRs focused — one feature or fix per PR
- Run `go test ./...` before opening
- For significant features, open an issue first to discuss the approach
- The dashboard uses Svelte 5 runes (`$state`, `$derived`, `$effect`) — not the legacy options API

## License

By contributing you agree that your contributions will be licensed under the [AGPL-3.0](LICENSE).
