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
	"database/sql"
	"errors"
	"net/http"

	"uploader/internal/logx"
	"uploader/internal/model"

	"github.com/go-chi/chi/v5"
)

// handleUserListFiles returns a paginated list of files owned by the
// authenticated user. Guest uploads (owner_id == "") are never returned.
//
// GET /v1/user/files?limit=50&offset=0
func (s *Server) handleUserListFiles(w http.ResponseWriter, r *http.Request) {
	email := authEmailFromContext(r.Context())

	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := parsePositiveInt(limitStr, 50); err == nil {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := parseNonNegativeInt(offsetStr, 0); err == nil {
			offset = o
		}
	}
	if limit > 100 {
		limit = 100
	}

	files, err := s.db.ListFilesByOwner(r.Context(), email, limit, offset)
	if err != nil {
		logx.Errorf("user list files failed: owner=%q err=%v", email, err)
		writeError(w, http.StatusInternalServerError, "failed to list files")
		return
	}

	if files == nil {
		files = make([]model.FileRecord, 0)
	}

	resp := map[string]any{
		"files":  toUserFileViews(files),
		"limit":  limit,
		"offset": offset,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleUserDeleteFile deletes a file that belongs to the authenticated user.
// Returns 404 both when the file does not exist and when it belongs to a
// different owner — this prevents leaking whether a given file_id is valid
// for another user (IDOR prevention).
//
// DELETE /v1/user/files/{file_id}
func (s *Server) handleUserDeleteFile(w http.ResponseWriter, r *http.Request) {
	email := authEmailFromContext(r.Context())
	fileID := chi.URLParam(r, "file_id")
	if fileID == "" {
		writeError(w, http.StatusBadRequest, "file_id is required")
		return
	}

	file, err := s.db.GetFile(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		logx.Errorf("user delete: db get error file_id=%s owner=%q err=%v", fileID, email, err)
		writeError(w, http.StatusInternalServerError, "failed to fetch file")
		return
	}

	// Respond with 404 even when the file exists but belongs to another user.
	// Do not distinguish "not found" from "wrong owner" to avoid information
	// disclosure about file ownership (IDOR prevention).
	if file.OwnerID != email {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	// Delete from storage first. If the DB delete subsequently fails the
	// object is orphaned in storage but the metadata record survives, so a
	// client retry will succeed on the storage step (idempotent object delete)
	// and then succeed on the DB step.
	if err := s.storage.Delete(r.Context(), file.ObjectKey); err != nil {
		logx.Errorf("user delete: storage delete failed file_id=%s object_key=%q err=%v", fileID, file.ObjectKey, err)
		writeError(w, http.StatusInternalServerError, "failed to delete file")
		return
	}

	if err := s.db.DeleteFile(r.Context(), fileID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		logx.Errorf("user delete: db delete failed file_id=%s err=%v", fileID, err)
		writeError(w, http.StatusInternalServerError, "failed to delete file metadata")
		return
	}

	logx.Infof("user delete: file deleted file_id=%s owner=%q", fileID, email)
	w.WriteHeader(http.StatusNoContent)
}

// handleUserFileStats returns metadata and download statistics for a single
// file owned by the authenticated user.
//
// GET /v1/user/files/{file_id}/stats
func (s *Server) handleUserFileStats(w http.ResponseWriter, r *http.Request) {
	email := authEmailFromContext(r.Context())
	fileID := chi.URLParam(r, "file_id")
	if fileID == "" {
		writeError(w, http.StatusBadRequest, "file_id is required")
		return
	}

	file, err := s.db.GetFile(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		logx.Errorf("user stats: db get error file_id=%s owner=%q err=%v", fileID, email, err)
		writeError(w, http.StatusInternalServerError, "failed to fetch file")
		return
	}

	// Same IDOR prevention as handleUserDeleteFile.
	if file.OwnerID != email {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	writeJSON(w, http.StatusOK, toUserFileView(file))
}

// userFileView is the safe, user-facing JSON representation of a FileRecord.
// Fields that must not be exposed externally (password_hash, object_key,
// bucket, owner_id) are intentionally omitted.
type userFileView struct {
	FileID        string `json:"file_id"`
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	SizeBytes     int64  `json:"size_bytes"`
	DownloadCount int64  `json:"download_count"`
	HasPassword   bool   `json:"has_password"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func toUserFileView(f model.FileRecord) userFileView {
	return userFileView{
		FileID:        f.ID,
		Filename:      f.Filename,
		ContentType:   f.ContentType,
		SizeBytes:     f.SizeBytes,
		DownloadCount: f.DownloadCount,
		HasPassword:   f.PasswordHash != "",
		ExpiresAt:     f.ExpiresAt,
		CreatedAt:     f.CreatedAt,
	}
}

func toUserFileViews(files []model.FileRecord) []userFileView {
	views := make([]userFileView, len(files))
	for i, f := range files {
		views[i] = toUserFileView(f)
	}
	return views
}
