package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"uploader/internal/model"
	"uploader/internal/storage"
	"uploader/internal/db"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var ErrTooLarge = errors.New("file too large")

type Server struct {
	cfg     Config
	db      db.Store
	storage storage.Provider
}

func NewServer(cfg Config, dbConn db.Store, storageClient storage.Provider) *Server {
	return &Server{cfg: cfg, db: dbConn, storage: storageClient}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Post("/v1/files", s.handleUpload)
	r.Get("/v1/files/{file_id}/download", s.handleDownload)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return r
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	// Allow a bit of multipart overhead.
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(2*1024*1024))

	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	var filePart *multipart.Part
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}
		if part.FormName() == "file" {
			filePart = part
			break
		}
		_ = part.Close()
	}
	if filePart == nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer filePart.Close()

	filename := sanitizeFilename(filePart.FileName())
	if filename == "" {
		writeError(w, http.StatusBadRequest, "file name is required")
		return
	}
	contentType := filePart.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	id := uuid.NewString()
	objectKey := path.Join(id, filename)

	counting := &countingReader{r: filePart, max: maxBytes}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	_, err = s.storage.Put(ctx, objectKey, counting, -1, contentType)
	if err != nil {
		if errors.Is(err, ErrTooLarge) || strings.Contains(err.Error(), "entity too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds max size")
			return
		}
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	record := model.FileRecord{
		ID:          id,
		Filename:    filename,
		ContentType: contentType,
		SizeBytes:   counting.n,
		Bucket:      "stub-bucket",
		ObjectKey:   objectKey,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.db.CreateFile(r.Context(), record); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist metadata")
		return
	}

	resp := map[string]any{
		"file_id":      record.ID,
		"filename":     record.Filename,
		"content_type": record.ContentType,
		"size_bytes":   record.SizeBytes,
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "file_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "file_id is required")
		return
	}

	file, err := s.db.GetFile(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch metadata")
		return
	}

	url, err := s.storage.GetSignedURL(r.Context(), file.ObjectKey, 60*time.Second)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to sign url")
		return
	}

	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusFound)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func sanitizeFilename(name string) string {
	name = path.Clean("/" + name)
	name = strings.TrimPrefix(name, "/")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "." || name == "" {
		return ""
	}
	return name
}

type countingReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
		if c.n > c.max {
			return n, ErrTooLarge
		}
	}
	return n, err
}

