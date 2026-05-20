# Batch Processing Middleware

An enterprise-grade, highly resilient batch-processing middleware written in Go. This service acts as an intermediary layer, accepting large batches of jobs from multiple source systems, securely processing them concurrently against external systems, and delivering completion statuses via secure webhooks.

## Key Features

- **Robust Concurrency**: Leverages PostgreSQL 18's `FOR UPDATE SKIP LOCKED` for rock-solid, lock-free row-level concurrency across multiple horizontally-scaled worker nodes.
- **Idempotent API**: All batch creation endpoints require an Idempotency-Key header to safely handle network retries and prevent duplicate batch processing.
- **Resilient Webhooks (Outbox Pattern)**: Webhooks are dispatched using the transactional Outbox Pattern. If the destination is unreachable, exponential backoff is applied.
- **Dead Letter Queue (DLQ)**: Failed items that exhaust their maximum retry attempts are safely routed to a Dead Letter Queue for manual inspection and replay.
- **Stale Lock Recovery**: Automatically detects and recovers items that were left in a "processing" state by a crashed or unresponsive worker node.
- **Modern Identifiers**: Utilizes PostgreSQL 18's native `uuidv7()` for all primary keys, ensuring time-ordered locality and massive scalability.
- **Observability**: Built-in Prometheus metrics and structured JSON logging using `go.uber.org/zap`.
- **Secure Webhooks**: Webhook payloads are signed using HMAC SHA-256, allowing consumers to verify the authenticity of the webhook payload.

## Technology Stack

- **Language**: Go 1.21+
- **Database**: PostgreSQL 18
- **Router**: `go-chi/chi`
- **Database Driver/Mapper**: `github.com/lib/pq` & `github.com/jmoiron/sqlx`
- **Migrations**: `golang-migrate/migrate`
- **Logging**: `go.uber.org/zap`

## Project Structure

```
.
├── cmd/
│   └── api/                # Main application entrypoint
├── internal/
│   ├── config/             # Application configuration loader (viper/env)
│   ├── database/           # PostgreSQL connection initialization
│   ├── domain/             # Core domain models and enums
│   ├── http/               # Chi Router, Handlers, and Middlewares
│   ├── observability/      # Zap Logger & Prometheus Metrics
│   ├── repository/         # Data Access Layer (sqlx implementations)
│   └── service/            # Core Business Logic (WorkerPool, WebhookDispatcher)
├── migrations/             # PostgreSQL up/down migration files (.sql)
├── docker-compose.yml      # Local development infrastructure
├── Makefile                # Build, run, and migration commands
└── .env.example            # Example environment variables
```

## Getting Started

### Prerequisites
- Go 1.21+
- Docker & Docker Compose
- `golang-migrate` CLI installed locally

### 1. Environment Setup

Copy the example environment file:
```bash
cp .env.example .env
```

### 2. Start Infrastructure

Launch the PostgreSQL 18 database using Docker Compose:
```bash
docker-compose up -d postgres
```

### 3. Run Migrations

Ensure your database schema is fully up-to-date:
```bash
make migrate-up
```

### 4. Run the Application

Start the API and Background Workers:
```bash
make run
```
By default, the API will be available at `http://localhost:8080`.

## API Documentation

### Create a Batch
`POST /v1/batches`

Headers:
- `Idempotency-Key` (Required): Unique key to prevent duplicate processing.

Body:
```json
{
  "correlation_id": "req-12345",
  "source_system": "billing_service",
  "webhook_url": "https://api.example.com/webhooks/batches",
  "webhook_secret": "super_secret_signing_key",
  "items": [
    {
      "external_id": "invoice_778",
      "payload": "{\"amount\": 100.00, \"currency\": \"USD\"}"
    }
  ]
}
```

### Cancel a Batch
`POST /v1/batches/{batch_id}/cancel`
Cancels a batch. Any items that are currently `pending` will be marked as `cancelled`.

### Batch Status Webhook Payload
When a batch finishes processing (either success, partial success, or failure), a POST request is sent to the configured `webhook_url` with a `X-Signature` header (HMAC SHA-256) for verification.

```json
{
  "event": "batch_completed",
  "batch_id": "018f6c58-...-uuidv7",
  "correlation_id": "req-12345",
  "status": "done",
  "summary": {
    "total": 1,
    "success": 1,
    "failed": 0
  },
  "failed_items": [],
  "timestamp": "2026-05-20T12:00:00Z"
}
```

## License
MIT License
