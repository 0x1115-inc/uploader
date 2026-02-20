# Uploader v1.0.0

Minimal file upload microservice with S3/MinIO storage and SQLite metadata.

## Features
- No auth, no checksum
- Max upload size: 50MB (configurable)
- Streaming multipart upload (no full in-memory buffering)
- S3/MinIO storage via `StorageProvider`
- SQLite metadata in a single `files` table

## Endpoints
- `POST /v1/files` multipart upload (field `file`)
- `GET /v1/files/:file_id/download` returns `302` to a signed URL (TTL=60s)
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
MAX_UPLOAD_MB=50

S3_ENDPOINT=minio:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=uploads
S3_REGION=
S3_USE_SSL=false
```

## Examples

Upload a file:

```bash
curl -s -X POST http://localhost:8080/v1/files \
  -F "file=@./path/to/file.pdf"
```

Response:

```json
{"file_id":"...","filename":"file.pdf","content_type":"application/pdf","size_bytes":12345}
```

Download (302 redirect to signed URL, 60s TTL):

```bash
curl -v http://localhost:8080/v1/files/<file_id>/download
```

## Notes
- If you build locally, `go mod tidy` will fetch dependencies and generate `go.sum`.
- The service enforces a 50MB file limit using a streaming counter and HTTP body limit.
