# Lapin development guide

Lapin is a Go and Vue 3 learning platform. Keep changes MVP-sized and preserve the existing modular-monolith layout.

## Architecture

- `cmd/lapin`: executable entry point.
- `internal/httpapi/handler`: one focused Go handler file per HTTP area; keep route-to-handler mappings visible in `internal/httpapi/server.go`.
- `internal/database`: keep each table's operations in its own Go file and use parameterized PostgreSQL queries.
- `migrations`: ordered SQL migrations embedded by `migrations/migrations.go`.
- `web`: Vue 3, TypeScript and PrimeVue application embedded by `web/web.go` after `npm run build`.
- Chapter bodies are stored as Markdown. Tiptap converts between Markdown and editor state in the browser; Go stores the string unchanged.
- Whiteboards use Excalidraw in a transparent React island. Persist the anchored document contract, never viewport or camera state.

## Data and API conventions

- Database primary keys are identity `BIGINT` values. HTTP and Web clients only receive HashID strings.
- Chapters form a tree through `parent_id`.
- Session-authenticated writes require the CSRF header. OpenAPI imports require a bearer access token.
- HTTP routes may use only `GET` and `POST`. Model updates and revocations as explicit `POST` action paths; never add `PUT`, `PATCH`, or `DELETE` routes.
- Use the existing JSON response envelope and validate all input at the handler boundary.
- Preserve user-owned work and interaction records during repeated OpenAPI imports.

## Verification

Use the PostgreSQL test database at `127.0.0.1:5433` without resetting the user's `postgres` database.

```sh
cd web && npm test && npm run typecheck && npm run build
TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5433/lapin_test?sslmode=disable' go test -race -coverpkg=./... ./...
go vet ./...
go build ./cmd/lapin
```

Add or update behavior tests before implementation. For UI changes, also verify the registration-to-study happy path in a real browser.
