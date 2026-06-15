package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"uploader/internal/model"
)

func TestUploadWithoutPasswordAllowed(t *testing.T) {
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, newStubStore(), &stubStorage{})
	body, contentType := newMultipartUpload(t, "hello.txt", "hello world")
	req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
	req.Header.Set("Content-Type", contentType)

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.Code)
	}
}

func TestDownloadRequiresMatchingPassword(t *testing.T) {
	store := newStubStore()
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, &stubStorage{})
	body, contentType := newMultipartUploadWithFields(t, "hello.txt", "hello world", false, map[string]string{
		uploadPasswordField: "secret123",
	})
	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/files", body)
	uploadReq.Header.Set("Content-Type", contentType)

	uploadResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadResp, uploadReq)
	if uploadResp.Code != http.StatusCreated {
		t.Fatalf("expected upload status %d, got %d: %s", http.StatusCreated, uploadResp.Code, uploadResp.Body.String())
	}

	var created struct {
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal(uploadResp.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}

	missingPasswordReq := httptest.NewRequest(http.MethodGet, "/v1/files/"+created.FileID+"/download", nil)
	missingPasswordResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(missingPasswordResp, missingPasswordReq)
	if missingPasswordResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing password status %d, got %d", http.StatusUnauthorized, missingPasswordResp.Code)
	}

	wrongPasswordReq := httptest.NewRequest(http.MethodPost, "/v1/files/"+created.FileID+"/download", strings.NewReader(`{"password":"wrong"}`))
	wrongPasswordReq.Header.Set("Content-Type", "application/json")
	wrongPasswordResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongPasswordResp, wrongPasswordReq)
	if wrongPasswordResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong password status %d, got %d", http.StatusUnauthorized, wrongPasswordResp.Code)
	}

	correctPasswordReq := httptest.NewRequest(http.MethodPost, "/v1/files/"+created.FileID+"/download", strings.NewReader(`{"password":"secret123"}`))
	correctPasswordReq.Header.Set("Content-Type", "application/json")
	correctPasswordResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(correctPasswordResp, correctPasswordReq)
	if correctPasswordResp.Code != http.StatusFound {
		t.Fatalf("expected correct password status %d, got %d", http.StatusFound, correctPasswordResp.Code)
	}
	if location := correctPasswordResp.Header().Get("Location"); location == "" {
		t.Fatal("expected Location header to be set")
	}

	file, err := store.GetFile(context.Background(), created.FileID)
	if err != nil {
		t.Fatalf("get stored file: %v", err)
	}
	if file.PasswordHash == "" || file.PasswordHash == "secret123" {
		t.Fatalf("expected stored password hash, got %q", file.PasswordHash)
	}
}

func TestDownloadAllowsPasswordViaFormBody(t *testing.T) {
	store := newStubStore()
	store.files["protected-file"] = model.FileRecord{
		ID:           "protected-file",
		ObjectKey:    "files/2026/05/protected-file",
		PasswordHash: mustHashPassword(t, "secret123"),
	}
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, &stubStorage{})
	form := url.Values{}
	form.Set("password", "secret123")
	req := httptest.NewRequest(http.MethodPost, "/v1/files/protected-file/download", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, resp.Code)
	}
}

func TestDownloadWithInvalidBodyRejected(t *testing.T) {
	store := newStubStore()
	store.files["protected-file"] = model.FileRecord{
		ID:           "protected-file",
		ObjectKey:    "files/2026/05/protected-file",
		PasswordHash: mustHashPassword(t, "secret123"),
	}
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, &stubStorage{})
	req := httptest.NewRequest(http.MethodPost, "/v1/files/protected-file/download", strings.NewReader(`{"password":`))
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.Code)
	}
}

func TestDownloadWithoutStoredPasswordAllowsAccess(t *testing.T) {
	store := newStubStore()
	store.files["legacy-file"] = model.FileRecord{
		ID:        "legacy-file",
		ObjectKey: "files/2026/05/legacy-file",
	}
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, &stubStorage{})
	req := httptest.NewRequest(http.MethodGet, "/v1/files/legacy-file/download", nil)

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, resp.Code)
	}
}

func TestUploadWithExpirationField(t *testing.T) {
	store := newStubStore()
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, &stubStorage{})
	expiresAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	body, contentType := newMultipartUploadWithFields(t, "hello.txt", "hello world", false, map[string]string{
		uploadExpiresAtField: expiresAt,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
	req.Header.Set("Content-Type", contentType)

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, resp.Code, resp.Body.String())
	}

	var created struct {
		FileID    string `json:"file_id"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}
	if created.ExpiresAt == "" {
		t.Fatal("expected expires_at in upload response")
	}

	rec, err := store.GetFile(context.Background(), created.FileID)
	if err != nil {
		t.Fatalf("get stored file: %v", err)
	}
	if rec.ExpiresAt == "" {
		t.Fatal("expected expires_at to be persisted")
	}
}

func TestUploadWithInvalidExpirationFieldRejected(t *testing.T) {
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, newStubStore(), &stubStorage{})
	body, contentType := newMultipartUploadWithFields(t, "hello.txt", "hello world", false, map[string]string{
		uploadExpiresAtField: "not-a-timestamp",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
	req.Header.Set("Content-Type", contentType)

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.Code)
	}
}

func TestUploadAcceptsMetadataFieldsAfterFile(t *testing.T) {
	store := newStubStore()
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, &stubStorage{})
	expiresAt := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	body, contentType := newMultipartUploadWithFields(t, "hello.txt", "hello world", true, map[string]string{
		uploadPasswordField:  "secret123",
		uploadExpiresAtField: expiresAt,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/files", body)
	req.Header.Set("Content-Type", contentType)

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, resp.Code, resp.Body.String())
	}

	var created struct {
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal upload response: %v", err)
	}

	rec, err := store.GetFile(context.Background(), created.FileID)
	if err != nil {
		t.Fatalf("get stored file: %v", err)
	}
	if rec.PasswordHash == "" {
		t.Fatal("expected password hash to be persisted")
	}
	if rec.ExpiresAt == "" {
		t.Fatal("expected expires_at to be persisted")
	}
}

func TestDownloadExpiredFileReturnsNotFoundAndCleansUp(t *testing.T) {
	store := newStubStore()
	storage := &stubStorage{}
	store.files["expired-file"] = model.FileRecord{
		ID:        "expired-file",
		ObjectKey: "files/2026/05/expired-file",
		ExpiresAt: time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
	}

	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, storage)
	req := httptest.NewRequest(http.MethodGet, "/v1/files/expired-file/download", nil)

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, resp.Code)
	}
	if _, err := store.GetFile(context.Background(), "expired-file"); err != sql.ErrNoRows {
		t.Fatalf("expected metadata deleted, got err=%v", err)
	}
	if len(storage.deletedKeys) != 1 || storage.deletedKeys[0] != "files/2026/05/expired-file" {
		t.Fatalf("expected object cleanup, got %+v", storage.deletedKeys)
	}
}

func newMultipartUpload(t *testing.T, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	return newMultipartUploadWithFields(t, filename, content, false, nil)
}

func newMultipartUploadWithFields(t *testing.T, filename, content string, fieldsAfterFile bool, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writeFields := func() {
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatalf("write field %s: %v", key, err)
			}
		}
	}
	writeFile := func() {
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := io.WriteString(part, content); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}
	if !fieldsAfterFile {
		writeFields()
	}
	writeFile()
	if fieldsAfterFile {
		writeFields()
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return hash
}

type stubStore struct {
	files map[string]model.FileRecord
}

func newStubStore() *stubStore {
	return &stubStore{files: make(map[string]model.FileRecord)}
}

func (s *stubStore) CreateFile(_ context.Context, f model.FileRecord) error {
	s.files[f.ID] = f
	return nil
}

func (s *stubStore) GetFile(_ context.Context, id string) (model.FileRecord, error) {
	f, ok := s.files[id]
	if !ok {
		return model.FileRecord{}, sql.ErrNoRows
	}
	return f, nil
}

func (s *stubStore) ListFiles(_ context.Context, _, _ int) ([]model.FileRecord, error) {
	return nil, nil
}

func (s *stubStore) ListFilesByOwner(_ context.Context, ownerID string, _, _ int) ([]model.FileRecord, error) {
	var out []model.FileRecord
	for _, f := range s.files {
		if f.OwnerID == ownerID {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s *stubStore) UpdateFile(_ context.Context, _ model.FileRecord) error {
	return nil
}

func (s *stubStore) DeleteFile(_ context.Context, id string) error {
	delete(s.files, id)
	return nil
}

func (s *stubStore) IncrementDownloadCount(_ context.Context, id string) error {
	if f, ok := s.files[id]; ok {
		f.DownloadCount++
		s.files[id] = f
	}
	return nil
}

type stubStorage struct {
	deletedKeys []string
}

func (s *stubStorage) Put(_ context.Context, _ string, reader io.Reader, _ int64, _ string) (string, error) {
	_, err := io.Copy(io.Discard, reader)
	return "etag", err
}

func (s *stubStorage) GetSignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	return "https://example.test/download/" + key + "?ttl=" + ttl.String(), nil
}

func (s *stubStorage) Delete(_ context.Context, key string) error {
	s.deletedKeys = append(s.deletedKeys, key)
	return nil
}
