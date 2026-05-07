# Uploader v1.1

Minimal file upload microservice with S3/MinIO-compatible storage and SQL metadata.

## Features
- No auth, no checksum
- Optional password-protected downloads via request body on `POST /v1/files/:file_id/download`
- Optional file expiration via `expires_at` multipart field (RFC3339)
- Max upload size: 50MB (configurable)
- Streaming multipart upload (no full in-memory buffering)
- S3/MinIO storage via `StorageProvider`
- Metadata in a single `files` table (Postgres in production, SQLite fallback for local/dev)

## Endpoints
- `POST /v1/files` multipart upload with `file` and optional `password` / `expires_at` form fields
- `GET /v1/files/:file_id/download` returns `302` for files without a password
- `POST /v1/files/:file_id/download` accepts a body `password` and returns `302` for protected files
- `GET /healthz`

## Run with Docker Compose (MinIO + service)

```bash
docker compose up --build
```

Service: http://localhost:8080
MinIO console: http://localhost:9001 (user/pass: `minioadmin` / `minioadmin`)

## Local run

```bash
go run ./cmd/server
```

## Configuration

Environment variables (defaults shown):

```bash
PORT=8080
DB_PATH=./uploader.db
DATABASE_URL=
MAX_UPLOAD_MB=50
LOG_LEVEL=info

S3_ENDPOINT=minio:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=uploads
S3_REGION=
S3_USE_SSL=false

# Comma-separated list of allowed CORS origins. Use * to allow all origins.
CORS_ALLOWED_ORIGINS=*
```

## Examples

Upload a file:

```bash
curl -s -X POST http://localhost:8080/v1/files \
  -F "file=@./path/to/file.pdf"
```

Upload with password protection:

```bash
curl -s -X POST http://localhost:8080/v1/files \
  -F "password=secret123" \
  -F "file=@./path/to/file.pdf"
```

Upload with expiration (optional):

```bash
curl -s -X POST http://localhost:8080/v1/files \
  -F "expires_at=2026-12-31T23:59:59Z" \
  -F "file=@./path/to/file.pdf"
```

Response:

```json
{"file_id":"...","filename":"file.pdf","content_type":"application/pdf","size_bytes":12345,"expires_at":"2026-12-31T23:59:59Z"}
```

Download (302 redirect to signed URL, 60s TTL):

```bash
curl -v http://localhost:8080/v1/files/<file_id>/download

# For password-protected files, send the password in the request body:
curl -v -X POST http://localhost:8080/v1/files/<file_id>/download \
  -H "Content-Type: application/json" \
  -d '{"password":"secret123"}'
```

## Notes
- If you build locally, `go mod tidy` will fetch dependencies and generate `go.sum`.
- The service enforces a 50MB file limit using a streaming counter and HTTP body limit.
- For files uploaded with a `password` form field, protected downloads require the same password in the POST request body.
- For files uploaded with an `expires_at` form field, downloads return `404` after expiration and cleanup may remove object + metadata.
- `DATABASE_URL` set: uses Postgres.
- `DATABASE_URL` empty: uses SQLite at `DB_PATH`.

## Deployment
- DigitalOcean Kubernetes + Spaces guide: `docs/digitalocean-k8s-spaces.md`
