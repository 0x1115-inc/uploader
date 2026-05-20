# File Listing Architecture

## Decision: Endpoint Addition vs. New Service

This document explains the architectural decision to implement file listing as an endpoint within the existing **Uploader** service rather than creating a separate microservice.

## Architecture Decision Record (ADR)

### Context

The uploader service initially supported file upload and download operations. The requirement to list uploaded files presented two options:

1. **Add endpoint to existing service** - Implement `GET /v1/files` in the uploader service
2. **Create new microservice** - Build a separate "File Manager" or "File Query" service

### Decision

**Add endpoint to existing service** ✅

### Rationale

#### 1. Single Business Responsibility
- The uploader service manages the complete file lifecycle: upload, download, and now retrieval
- File listing is intrinsically related to file management, not a separate business domain
- Microservices should align with business capabilities, not technical operations

#### 2. Reduced Operational Complexity
- One less service to deploy, monitor, and scale
- One less database connection to manage
- Fewer potential points of failure

#### 3. No Performance Concerns Yet
- Read-heavy operations (listing) don't compete with upload/download under normal load
- Database queries are indexed (`created_at DESC`)
- If listing becomes a bottleneck, can be optimized or split later without breaking the API

#### 4. Simplified Data Consistency
- Direct access to the same database; no eventual consistency issues
- No data synchronization concerns
- Query results are always current

#### 5. API Surface Consistency
- Both operations use the same authentication model (if any)
- Same CORS configuration applies
- Unified error handling and logging

#### 6. YAGNI Principle
- No need to over-engineer prematurely
- Evolution path is clear if needs change

## Implementation Details

### HTTP Handler

Located in [internal/http/handlers.go](../../internal/http/handlers.go#L367):

```go
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
    // Parses limit/offset query parameters
    // Calls s.db.ListFiles()
    // Returns JSON response
}
```

### Database Layer

The database layer already defined `ListFiles()` in the Store interface:

```go
type Store interface {
    ListFiles(ctx context.Context, limit, offset int) ([]model.FileRecord, error)
}
```

Implementation in [internal/db/db.go](../../internal/db/db.go):
- Orders by `created_at DESC`
- Enforces limit (default 50, max 1000)
- Applies offset for pagination

### Request Flow

```
GET /v1/files?limit=50&offset=0
    ↓
chi router → handleListFiles()
    ↓
parsePositiveInt(limit), parseNonNegativeInt(offset)
    ↓
s.db.ListFiles(ctx, limit, offset)
    ↓
SQLite query with index lookup
    ↓
Scan into []model.FileRecord
    ↓
JSON response with {"files": [...], "limit": 50, "offset": 0}
```

## Database Schema

The `files` table includes:

```sql
CREATE TABLE files (
    id TEXT PRIMARY KEY,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    bucket TEXT NOT NULL,
    object_key TEXT NOT NULL,
    password_hash TEXT DEFAULT '',
    expires_at TEXT DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX idx_files_created_at ON files(created_at DESC);
```

The index on `created_at DESC` ensures efficient retrieval of recent files.

## Query Parameters

| Parameter | Validation                      | Limits      |
|-----------|----------------------------------|-------------|
| limit     | Positive integer                | 1-1000      |
| offset    | Non-negative integer            | 0+          |

Invalid values default to safe defaults (limit=50, offset=0).

## Response Schema

```json
{
  "files": [
    {
      "id": "uuid",
      "filename": "string",
      "content_type": "string",
      "size_bytes": "int64",
      "expires_at": "RFC3339 (optional)",
      "created_at": "RFC3339"
    }
  ],
  "limit": "int",
  "offset": "int"
}
```

## Error Handling

- **500 Internal Server Error**: Database query failure
- Returns `{"error": "failed to list files"}` on database errors

## Future Evolution Path

If requirements change:

### If listing volume grows significantly:
- Add caching layer (Redis) before database
- Implement cursor-based pagination instead of offset
- Add filtering (by date range, filename pattern, etc.)

### If listing becomes a separate concern:
- Extract to `ListingService` microservice
- Existing API remains unchanged (routing changes internally)
- Use service discovery or API gateway to route requests

### If real-time events become important:
- Add WebSocket support for file updates
- Emit events when files are uploaded/deleted
- Separate event stream from HTTP queries

## Related Documentation

- [API Specification](../../apis/api-specs.yml) - OpenAPI definition
- [Feature Documentation](../features/file-listing.md) - User-facing guide
- [Database Layer](../../internal/db/db.go) - Implementation details
- [HTTP Handlers](../../internal/http/handlers.go) - Request handling
