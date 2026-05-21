# Changelog

## [1.2.1] - May 21, 2026

### Changed
- Cleaned up OpenAPI operation IDs for download endpoints to avoid duplicates:
  - `GET /v1/files/{file_id}/download` -> `downloadFileGet`
  - `POST /v1/files/{file_id}/download` -> `downloadFilePost`
- Consolidated duplicated `400` response definitions for `POST /v1/files/{file_id}/download` into a single response with explicit examples for:
  - missing file id
  - invalid download request body

## [1.2.0] - May 20, 2026

### Added

#### File Listing Endpoint
- **Endpoint**: `GET /v1/files`
- **Purpose**: Retrieve a paginated list of all uploaded files
- **Query Parameters**:
  - `limit` (int, default: 50, max: 1000) - Number of files to return
  - `offset` (int, default: 0) - Number of files to skip for pagination
- **Response**: JSON object with `files` array, `limit`, and `offset`
- **Ordering**: Files ordered by creation date (newest first)
- **Status Code**: 200 OK on success, 500 Internal Server Error on failure

#### HTTP Handler
- Added `handleListFiles()` method in `internal/http/handlers.go`
- Parses and validates query parameters (`limit`, `offset`)
- Enforces reasonable limits (max 1000) to prevent excessive memory usage
- Returns paginated file list with metadata

#### Helper Functions
- `parsePositiveInt(s string, defaultVal int) (int, error)` - Parse and validate positive integers
- `parseNonNegativeInt(s string, defaultVal int) (int, error)` - Parse and validate non-negative integers

#### API Specification
- Updated OpenAPI 3.0.3 specification to version 1.2.0
- Added `GET /v1/files` endpoint definition
- Added `FileListResponse` schema to components
- Included example responses for empty and populated file lists

#### Documentation
- **Feature Documentation** (`docs/features/file-listing.md`):
  - User-facing guide with examples
  - Parameter descriptions and usage patterns
  - Performance considerations
  - Error handling information

- **Architecture Documentation** (`docs/architect/file-listing.md`):
  - Architecture decision record (ADR)
  - Implementation details and request flow
  - Database schema and indexing strategy
  - Future evolution paths

### Modified

#### Dependencies
- Added `strconv` package import to `internal/http/handlers.go` for integer parsing

#### Router Configuration
- Added route registration: `r.Get("/v1/files", s.handleListFiles)` in HTTP server setup

#### API Specification Version
- Bumped version from 1.1.0 to 1.2.0

### Technical Details

- Query parameters are parsed with graceful fallback to defaults on invalid input
- Database query leverages existing `ListFiles()` method in Store interface
- Results are sorted using database index on `created_at DESC` for efficiency
- Empty result sets return empty array (not null) for consistency
- CORS headers already configured to support this endpoint

### Migration Notes

**No breaking changes.** This is a purely additive feature:
- Existing `POST /v1/files` (upload) functionality unchanged
- Existing `GET/POST /v1/files/{file_id}/download` functionality unchanged
- Existing database schema remains compatible
- No database migrations required

### Testing Recommendations

1. **Happy Path**: List with default parameters, custom limits, pagination
2. **Edge Cases**: Empty database, max limit (1000), large offsets
3. **Validation**: Invalid query parameters, negative values, non-integer values
4. **Performance**: Time listing queries on databases with many files
5. **Ordering**: Verify results are sorted by creation date (descending)

### Future Considerations

- Add filtering by date range, filename, or content type
- Implement cursor-based pagination for very large datasets
- Add response caching if listing becomes a performance bottleneck
- Consider extracting to separate service only if listing volume exceeds upload/download traffic
