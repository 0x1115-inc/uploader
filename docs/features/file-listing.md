# File Listing Feature

## Overview

The File Listing endpoint enables clients to retrieve a paginated list of all uploaded files in the system, ordered by most recently created first.

## Endpoint

```
GET /v1/files
```

## Query Parameters

| Parameter | Type    | Default | Max     | Description                                      |
|-----------|---------|---------|---------|--------------------------------------------------|
| `limit`   | integer | 50      | 1000    | Number of files to return per page               |
| `offset`  | integer | 0       | -       | Number of files to skip (for pagination)         |

## Response

**Status Code:** `200 OK`

**Body:**
```json
{
  "files": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "filename": "document.pdf",
      "content_type": "application/pdf",
      "size_bytes": 24576,
      "expires_at": "2026-05-21T10:30:00Z",
      "created_at": "2026-05-20T10:00:00Z"
    }
  ],
  "limit": 50,
  "offset": 0
}
```

## Field Descriptions

- **id**: Unique file identifier (UUID)
- **filename**: Sanitized original filename
- **content_type**: MIME type of the file
- **size_bytes**: File size in bytes
- **expires_at**: (Optional) When the file will no longer be downloadable
- **created_at**: ISO 8601 timestamp of upload

## Usage Examples

### List first 50 files
```bash
curl http://localhost:8080/v1/files
```

### List with custom pagination
```bash
curl "http://localhost:8080/v1/files?limit=25&offset=50"
```

### Get all files (in chunks of 100)
```bash
# First page
curl "http://localhost:8080/v1/files?limit=100&offset=0"

# Second page
curl "http://localhost:8080/v1/files?limit=100&offset=100"
```

## Ordering

Files are ordered by creation date in descending order (newest first).

## Error Handling

| Status | Error                  | Description                           |
|--------|------------------------|---------------------------------------|
| 500    | failed to list files   | Database query or internal error      |

## Implementation Notes

- The database query includes an index on `created_at DESC` for efficient sorting
- Empty result sets return an empty array, not null
- Query parameter values outside valid ranges use defaults or are clamped to limits
- The endpoint is CORS-enabled and accepts OPTIONS requests

## Performance Considerations

- Default limit of 50 balances response size and network performance
- Hard maximum limit of 1000 prevents excessive memory usage
- Database index on `created_at` ensures O(1) query planning
- Large offset values may require full table scans; consider cursor-based pagination for very large datasets
