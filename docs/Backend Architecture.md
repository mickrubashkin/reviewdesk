# ReviewDesk Backend Architecture

This document provides a high-level overview of the ReviewDesk backend architecture.

## Tech Stack

The backend is built in Go as a high-performance, modular HTTP API:

- **Language**: Go (1.22+)
- **HTTP Router**: [chi v5](https://github.com/go-chi/chi/v5)
- **Database**: PostgreSQL with [pgx v5](https://github.com/jackc/pgx) (direct SQL queries, no ORM)
- **Migrations**: [Goose](https://github.com/pressly/goose)
- **Config**: `.env` loaded via `godotenv`
- **AI Integration**: OpenAI API with Server-Sent Events (SSE) streaming

---

## Directory Structure

The backend is organized into a modular **Feature-based architecture** (`internal/features/`) to decouple domains and keep entrypoints clean:

```text
apps/api/
├── cmd/
│   ├── demoseed/               # Demo data seeding entrypoint
│   ├── seed/                   # Development DB seeding script
│   ├── server/                 # Main HTTP server entrypoint
│   │   ├── main.go             # Wire-up: loads config, DB pool, mounts routers
│   │   ├── api_helpers.go      # Common API response helpers
│   │   ├── health_handler.go   # /health endpoint
│   │   └── operational_events.go # System operational logging
│   └── syncmeta/               # Metadata generation tool
├── internal/
│   ├── core/
│   │   └── auth/               # Core authentication primitives & context handling
│   ├── emailedit/              # Template parsing and editable fields logic
│   ├── emailreview/            # Review blocks, HTML diffing, and sanitization
│   ├── emailtext/              # Text extraction, links classification, content parsing
│   └── features/               # Feature domain modules
│       ├── ai/                 # OpenAI prompts, caching, SSE streaming & logs
│       ├── auth/               # OTP login, session handling, user management
│       ├── boards/             # Kanban boards, stages, approval areas config
│       ├── comments/           # Review comments, blocking status, replies
│       └── emails/             # Email CRUD, versioning, approvals, activity logs
db/
└── migrations/                 # Goose SQL database migrations
```

---

## Architectural Principles

### 1. Feature Isolation & Direct SQL
Each feature in `internal/features/<feature>` is self-contained:
- Contains its own HTTP handlers (`*_handlers.go`), database operations, and data models (`types.go`).
- Uses direct SQL via `pgxpool.Pool` for maximum transparency and efficiency without ORM abstractions.

### 2. Thin Server Entrypoint (`cmd/server/main.go`)
The `main.go` file acts purely as a composition root (wire-up):
- Loads environment configuration (`../../.env`).
- Initializes `*pgxpool.Pool` connection pool.
- Connects domain event loggers and email senders via interface implementations.
- Registers chi sub-routers for each feature (`auth.RegisterAuthRoutes`, `emails.RegisterEmailRoutes`, `boards.RegisterBoardRoutes`, `comments.RegisterCommentRoutes`, `ai.RegisterAIRoutes`).

### 3. Source of Truth & Safe Templating
- **Original HTML Immutability**: The original HTML of an email is never mutated directly.
- **Template Editing**: Admin/reviewers modify `data-edit-*` fields; the backend dynamically renders the final `review_html` template.
- **Sandboxed Rendering**: Untrusted client HTML is sanitized for preview without executing scripts.

### 4. Approval Gates
Approval state transitions enforce strict business integrity:
- An email **cannot be approved** if there are open `blocking` comments.
- An email **cannot be approved** if required approval areas (defined on board level) remain unapproved.

### 5. AI Streaming (SSE)
- Real-time AI analysis is delivered via Server-Sent Events (`EventSource`) on `/api/emails/{id}/ai-analysis-stream`.
- Results are cached in the database to prevent redundant LLM invocations.

### 6. Audit & Operational Telemetry
- **Email Events**: Track business actions (email creation, version restoration, stage moves, approvals) stored in `email_events`.
- **Operational Events**: System health, API server startup, and unexpected errors logged into `operational_events`.

---

## Key Commands

- `make setup-dev` — Start DB container → apply migrations → run seed.
- `make dev` — Run DB, API server, and web frontend concurrently.
- `make test-api` — Execute all unit and integration tests (`go test ./...`).
- `make db-migrate` / `make db-seed` / `make db-sync-meta` — Database management tasks.
