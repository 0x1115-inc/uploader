/**
uploader - A simple file upload server with S3-compatible storage support
Copyright (C) 2026 0x1115 Inc.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
**/

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

	"uploader/internal/db"
	"uploader/internal/logx"
	"uploader/internal/model"
	"uploader/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const filePasswordHeader = "X-File-Password"
const fileExpiresAtHeader = "X-File-Expires-At"

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
	r.Use(requestLogger)
	r.Post("/v1/files", s.handleUpload)
	r.Get("/v1/files/{file_id}/download", s.handleDownload)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return r
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	password := r.Header.Get(filePasswordHeader)
	expiresAtHeader := strings.TrimSpace(r.Header.Get(fileExpiresAtHeader))

	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	// HTTP-layer hard cap for the full request (file + multipart overhead).
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(2*1024*1024))

	mr, err := r.MultipartReader()
	if err != nil {
		if isTooLargeErr(err) {
			logx.Warnf("upload rejected: too large while reading multipart reader")
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds max size")
			return
		}
		logx.Warnf("upload rejected: invalid multipart form: %v", err)
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
			if isTooLargeErr(err) {
				logx.Warnf("upload rejected: too large while reading multipart part")
				writeError(w, http.StatusRequestEntityTooLarge, "file exceeds max size")
				return
			}
			logx.Warnf("upload rejected: multipart read error: %v", err)
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
	// Per-file cap while streaming the multipart part.
	limitedFile := http.MaxBytesReader(w, filePart, maxBytes)
	defer limitedFile.Close()

	filename := sanitizeFilename(filePart.FileName())
	if filename == "" {
		writeError(w, http.StatusBadRequest, "file name is required")
		return
	}
	contentType := filePart.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	passwordHash := ""
	if strings.TrimSpace(password) != "" {
		passwordHash, err = hashPassword(password)
		if err != nil {
			logx.Errorf("upload failed: password hash error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to secure file")
			return
		}
	}

	expiresAt := ""
	if expiresAtHeader != "" {
		expiresAt, err = parseAndValidateExpiration(expiresAtHeader, nowUTC)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	id := uuid.NewString()
	now := nowUTC()
	objectKey := path.Join("files", now.Format("2006"), now.Format("01"), id)

	counting := &countingReader{r: limitedFile}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	_, err = s.storage.Put(ctx, objectKey, counting, -1, contentType)
	if err != nil {
		if isTooLargeErr(err) {
			logx.Warnf("upload rejected: too large file_id=%s filename=%q", id, filename)
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds max size")
			return
		}
		logx.Errorf("upload failed: storage put error file_id=%s object_key=%q err=%v", id, objectKey, err)
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	record := model.FileRecord{
		ID:           id,
		Filename:     filename,
		ContentType:  contentType,
		SizeBytes:    counting.n,
		Bucket:       s.cfg.S3Bucket,
		ObjectKey:    objectKey,
		PasswordHash: passwordHash,
		ExpiresAt:    expiresAt,
		CreatedAt:    now.Format(time.RFC3339Nano),
	}
	if err := s.db.CreateFile(r.Context(), record); err != nil {
		logx.Errorf("upload failed: db create error file_id=%s err=%v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to persist metadata")
		return
	}

	resp := map[string]any{
		"file_id":      record.ID,
		"filename":     record.Filename,
		"content_type": record.ContentType,
		"size_bytes":   record.SizeBytes,
	}
	if record.ExpiresAt != "" {
		resp["expires_at"] = record.ExpiresAt
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "file_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "file_id is required")
		return
	}
	password := r.Header.Get(filePasswordHeader)

	file, err := s.db.GetFile(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logx.Warnf("download miss: file_id=%s not found", id)
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		logx.Errorf("download failed: db get error file_id=%s err=%v", id, err)
		writeError(w, http.StatusInternalServerError, "failed to fetch metadata")
		return
	}

	if file.PasswordHash != "" {
		if strings.TrimSpace(password) == "" {
			writeError(w, http.StatusUnauthorized, "password is required")
			return
		}
		if err := comparePassword(file.PasswordHash, password); err != nil {
			logx.Warnf("download rejected: invalid password file_id=%s", id)
			writeError(w, http.StatusUnauthorized, "invalid password")
			return
		}
	}

	if isExpired(file.ExpiresAt) {
		s.cleanupExpiredFile(r.Context(), file)
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	url, err := s.storage.GetSignedURL(r.Context(), file.ObjectKey, 60*time.Second)
	if err != nil {
		logx.Errorf("download failed: sign url error file_id=%s object_key=%q err=%v", id, file.ObjectKey, err)
		writeError(w, http.StatusInternalServerError, "failed to sign url")
		return
	}

	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusFound)
}

func (s *Server) cleanupExpiredFile(ctx context.Context, file model.FileRecord) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := s.storage.Delete(cleanupCtx, file.ObjectKey); err != nil {
		logx.Warnf("expired file cleanup: storage delete failed file_id=%s object_key=%q err=%v", file.ID, file.ObjectKey, err)
	}

	if err := s.db.DeleteFile(cleanupCtx, file.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		logx.Warnf("expired file cleanup: metadata delete failed file_id=%s err=%v", file.ID, err)
	}
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

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func comparePassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func parseAndValidateExpiration(raw string, nowFn func() time.Time) (string, error) {
	expiresAt, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", errors.New("invalid expiration; use RFC3339 timestamp")
	}

	expiresAt = expiresAt.UTC()
	if !expiresAt.After(nowFn()) {
		return "", errors.New("expiration must be in the future")
	}

	return expiresAt.Format(time.RFC3339Nano), nil
}

func isExpired(expiresAt string) bool {
	if strings.TrimSpace(expiresAt) == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return false
	}
	return !nowUTC().Before(parsed)
}

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
	}
	return n, err
}

func isTooLargeErr(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr) || strings.Contains(err.Error(), "entity too large")
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logx.Infof("%s %s status=%d duration=%s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
