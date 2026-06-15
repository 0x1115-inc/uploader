# Uploader v1.2

Minimal file upload microservice with S3/MinIO-compatible storage and SQL metadata.

## Features
- Guest uploads require no authentication
- Registered users authenticated via oauth2-proxy (`X-Auth-Email` header) can list their files, view download counts, and delete files
- Optional password-protected downloads via request body on `POST /v1/files/:file_id/download`
- Optional file expiration via `expires_at` multipart field (RFC3339)
- Download counter incremented on each successful redirect
- Max upload size: 50MB (configurable)
- Streaming multipart upload (no full in-memory buffering)
- S3/MinIO storage via `StorageProvider`
- Metadata in a single `files` table (Postgres in production, SQLite fallback for local/dev)

## Endpoints

### Public (no authentication required)
- `POST /v1/files` — multipart upload with `file` and optional `password` / `expires_at` form fields
- `GET /v1/files/:file_id/download` — returns `302` for files without a password
- `POST /v1/files/:file_id/download` — accepts a body `password` and returns `302` for protected files
- `GET /healthz`

### Protected (requires authentication via oauth2-proxy)
- `GET /v1/user/files` — list files uploaded by the authenticated user (`?limit=50&offset=0`)
- `GET /v1/user/files/:file_id/stats` — file metadata and download count
- `DELETE /v1/user/files/:file_id` — delete a file (only the owner can delete)

> **Note:** Authenticated users who upload via `POST /v1/files` with a valid `X-Auth-Email` header will have their files attributed to them automatically.

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
- Uploads made while authenticated (valid `X-Auth-Email`) are attributed to that user; uploads without the header are anonymous guest uploads.
- The download counter (`download_count`) is incremented on every successful `302` redirect regardless of whether the downloader is authenticated.

## Authentication & oauth2-proxy Setup

User-scoped endpoints (`/v1/user/*`) rely on the `X-Auth-Email` header injected by
[oauth2-proxy](https://oauth2-proxy.github.io/oauth2-proxy/) after session validation.
No JWT libraries or session tables are needed in this service.

Minimal oauth2-proxy configuration:

```ini
# Forward the authenticated user's email to the upstream service.
--pass-user-headers=true
--set-xauthrequest=true

# Protect only the user-scoped path; let the rest pass through.
--skip-auth-regex=^/v1/files
--skip-auth-regex=^/healthz
```

With this configuration:
- Unauthenticated requests to `/v1/files` and `/healthz` pass through as-is.
- Requests to `/v1/user/*` without a valid session are rejected by oauth2-proxy before reaching the service.
- For requests with a valid session, oauth2-proxy strips any client-supplied `X-Auth-Email` header and re-injects the verified value.

## Security Considerations

### Header spoofing
The `X-Auth-Email` header is trusted unconditionally by this service. An attacker
with direct network access to the service (bypassing oauth2-proxy) could forge this
header and impersonate any user.

**Mitigation:** ensure the service is not directly reachable from the internet.
Use a firewall rule, Kubernetes `NetworkPolicy`, or similar control so that only
the oauth2-proxy pod/container can reach the service.

### IDOR (Insecure Direct Object Reference)
User-scoped endpoints return `404` for both non-existent files and files owned by
a different user. This prevents an authenticated user from discovering whether a
given `file_id` belongs to another user.

### Sensitive field exposure
The `password_hash`, `object_key`, `bucket`, and `owner_id` fields are never
included in API responses. The `has_password` boolean field is provided instead
of the raw hash.

## Deployment
- DigitalOcean Kubernetes + Spaces guide: `docs/digitalocean-k8s-spaces.md`
