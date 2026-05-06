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
	"testing"
	"time"

	"uploader/internal/model"
)

func TestUploadWithoutPasswordAllowed(t *testing.T) {
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, newStubStore(), stubStorage{})
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
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, stubStorage{})
	body, contentType := newMultipartUpload(t, "hello.txt", "hello world")
	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/files", body)
	uploadReq.Header.Set("Content-Type", contentType)
	uploadReq.Header.Set(filePasswordHeader, "secret123")

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

	wrongPasswordReq := httptest.NewRequest(http.MethodGet, "/v1/files/"+created.FileID+"/download", nil)
	wrongPasswordReq.Header.Set(filePasswordHeader, "wrong")
	wrongPasswordResp := httptest.NewRecorder()
	server.Handler().ServeHTTP(wrongPasswordResp, wrongPasswordReq)
	if wrongPasswordResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong password status %d, got %d", http.StatusUnauthorized, wrongPasswordResp.Code)
	}

	correctPasswordReq := httptest.NewRequest(http.MethodGet, "/v1/files/"+created.FileID+"/download", nil)
	correctPasswordReq.Header.Set(filePasswordHeader, "secret123")
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

func TestDownloadWithoutStoredPasswordAllowsAccess(t *testing.T) {
	store := newStubStore()
	store.files["legacy-file"] = model.FileRecord{
		ID:        "legacy-file",
		ObjectKey: "files/2026/05/legacy-file",
	}
	server := NewServer(Config{MaxUploadMB: 5, S3Bucket: "uploads"}, store, stubStorage{})
	req := httptest.NewRequest(http.MethodGet, "/v1/files/legacy-file/download", nil)

	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, resp.Code)
	}
}

func newMultipartUpload(t *testing.T, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatalf("write file content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
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

func (s *stubStore) UpdateFile(_ context.Context, _ model.FileRecord) error {
	return nil
}

func (s *stubStore) DeleteFile(_ context.Context, _ string) error {
	return nil
}

type stubStorage struct{}

func (stubStorage) Put(_ context.Context, _ string, reader io.Reader, _ int64, _ string) (string, error) {
	_, err := io.Copy(io.Discard, reader)
	return "etag", err
}

func (stubStorage) GetSignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	return "https://example.test/download/" + key + "?ttl=" + ttl.String(), nil
}
