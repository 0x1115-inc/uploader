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
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const uploadPasswordField = "password"
const uploadExpiresAtField = "expires_at"
const maxMultipartFieldBytes = 4 * 1024

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
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.CORSOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		ExposedHeaders:   []string{"Content-Disposition", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	r.Post("/v1/files", s.handleUpload)
	r.Get("/v1/files/{file_id}/download", s.handleDownload)
	r.Post("/v1/files/{file_id}/download", s.handleDownload)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return r
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	password := ""
	expiresAt := ""

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

	var (
		filename    string
		contentType string
		sizeBytes   int64
		objectKey   string
		uploaded    bool
	)
	cleanupUploadedObject := func() {
		if !uploaded {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := s.storage.Delete(cleanupCtx, objectKey); err != nil {
			logx.Warnf("upload cleanup: storage delete failed object_key=%q err=%v", objectKey, err)
		}
	}

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

		switch part.FormName() {
		case uploadPasswordField:
			value, err := readMultipartFieldValue(part, maxMultipartFieldBytes)
			_ = part.Close()
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			password = value
		case uploadExpiresAtField:
			value, err := readMultipartFieldValue(part, maxMultipartFieldBytes)
			_ = part.Close()
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if value == "" {
				expiresAt = ""
				continue
			}
			expiresAt, err = parseAndValidateExpiration(value, nowUTC)
			if err != nil {
				cleanupUploadedObject()
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "file":
			if uploaded {
				_ = part.Close()
				cleanupUploadedObject()
				writeError(w, http.StatusBadRequest, "multiple file fields are not supported")
				return
			}

			filename = sanitizeFilename(part.FileName())
			if filename == "" {
				_ = part.Close()
				writeError(w, http.StatusBadRequest, "file name is required")
				return
			}

			contentType = part.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			id := uuid.NewString()
			now := nowUTC()
			objectKey = path.Join("files", now.Format("2006"), now.Format("01"), id)

			limitedFile := http.MaxBytesReader(w, part, maxBytes)
			counting := &countingReader{r: limitedFile}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
			_, err = s.storage.Put(ctx, objectKey, counting, -1, contentType)
			cancel()
			_ = limitedFile.Close()
			_ = part.Close()
			if err != nil {
				if isTooLargeErr(err) {
					logx.Warnf("upload rejected: too large filename=%q", filename)
					writeError(w, http.StatusRequestEntityTooLarge, "file exceeds max size")
					return
				}
				logx.Errorf("upload failed: storage put error object_key=%q err=%v", objectKey, err)
				writeError(w, http.StatusInternalServerError, "upload failed")
				return
			}

			sizeBytes = counting.n
			uploaded = true
		default:
			_ = part.Close()
		}
	}
	if !uploaded {
		writeError(w, http.StatusBadRequest, "missing file field")
		return
	}

	passwordHash := ""
	if strings.TrimSpace(password) != "" {
		passwordHash, err = hashPassword(password)
		if err != nil {
			cleanupUploadedObject()
			logx.Errorf("upload failed: password hash error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to secure file")
			return
		}
	}

	id := uuid.NewString()
	now := nowUTC()

	record := model.FileRecord{
		ID:           id,
		Filename:     filename,
		ContentType:  contentType,
		SizeBytes:    sizeBytes,
		Bucket:       s.cfg.S3Bucket,
		ObjectKey:    objectKey,
		PasswordHash: passwordHash,
		ExpiresAt:    expiresAt,
		CreatedAt:    now.Format(time.RFC3339Nano),
	}
	if err := s.db.CreateFile(r.Context(), record); err != nil {
		cleanupUploadedObject()
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
	password, err := readDownloadPassword(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

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

func readDownloadPassword(r *http.Request) (string, error) {
	if r.Method != http.MethodPost {
		return "", nil
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return "", nil
	}

	if strings.HasPrefix(contentType, "application/json") {
		var payload struct {
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, maxMultipartFieldBytes+1))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			return "", errors.New("invalid download request body")
		}
		return strings.TrimSpace(payload.Password), nil
	}

	if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		r.Body = http.MaxBytesReader(nil, r.Body, maxMultipartFieldBytes)
		if err := r.ParseForm(); err != nil {
			return "", errors.New("invalid download request body")
		}
		return strings.TrimSpace(r.FormValue("password")), nil
	}

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(maxMultipartFieldBytes); err != nil {
			return "", errors.New("invalid download request body")
		}
		return strings.TrimSpace(r.FormValue("password")), nil
	}

	return "", errors.New("unsupported content type")
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

func readMultipartFieldValue(part *multipart.Part, maxBytes int64) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, maxBytes+1))
	if err != nil {
		if isTooLargeErr(err) {
			return "", errors.New("multipart field exceeds max size")
		}
		return "", errors.New("invalid multipart form")
	}
	if int64(len(data)) > maxBytes {
		return "", errors.New("multipart field exceeds max size")
	}
	return strings.TrimSpace(string(data)), nil
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
