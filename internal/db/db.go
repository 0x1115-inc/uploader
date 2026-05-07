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

package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"uploader/internal/model"

	_ "modernc.org/sqlite"
)

type Store interface {
	CreateFile(ctx context.Context, f model.FileRecord) error
	GetFile(ctx context.Context, id string) (model.FileRecord, error)
	ListFiles(ctx context.Context, limit, offset int) ([]model.FileRecord, error)
	UpdateFile(ctx context.Context, f model.FileRecord) error
	DeleteFile(ctx context.Context, id string) error
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLite(path string) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetConnMaxLifetime(5 * time.Minute)

	store := &SQLiteStore{db: conn}
	if err := store.migrate(context.Background()); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS files (
	id TEXT PRIMARY KEY,
	filename TEXT NOT NULL,
	content_type TEXT NOT NULL,
	size_bytes INTEGER NOT NULL,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	password_hash TEXT NOT NULL DEFAULT '',
	expires_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_files_created_at ON files(created_at DESC);
`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE files ADD COLUMN password_hash TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE files ADD COLUMN expires_at TEXT NOT NULL DEFAULT ''`)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		return err
	}
	return nil
}

func (s *SQLiteStore) CreateFile(ctx context.Context, f model.FileRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO files (id, filename, content_type, size_bytes, bucket, object_key, password_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, f.ID, f.Filename, f.ContentType, f.SizeBytes, f.Bucket, f.ObjectKey, f.PasswordHash, f.ExpiresAt, f.CreatedAt)
	return err
}

func (s *SQLiteStore) GetFile(ctx context.Context, id string) (model.FileRecord, error) {
	var f model.FileRecord
	err := s.db.QueryRowContext(ctx, `
SELECT id, filename, content_type, size_bytes, bucket, object_key, password_hash, expires_at, created_at
FROM files
WHERE id = ?
`, id).Scan(&f.ID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Bucket, &f.ObjectKey, &f.PasswordHash, &f.ExpiresAt, &f.CreatedAt)
	if err != nil {
		return model.FileRecord{}, err
	}
	return f, nil
}

func (s *SQLiteStore) ListFiles(ctx context.Context, limit, offset int) ([]model.FileRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, filename, content_type, size_bytes, bucket, object_key, password_hash, expires_at, created_at
FROM files
ORDER BY created_at DESC
LIMIT ? OFFSET ?
`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]model.FileRecord, 0, limit)
	for rows.Next() {
		var f model.FileRecord
		if err := rows.Scan(&f.ID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Bucket, &f.ObjectKey, &f.PasswordHash, &f.ExpiresAt, &f.CreatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func (s *SQLiteStore) UpdateFile(ctx context.Context, f model.FileRecord) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE files
SET filename = ?, content_type = ?, size_bytes = ?, bucket = ?, object_key = ?, password_hash = ?, expires_at = ?
WHERE id = ?
`, f.Filename, f.ContentType, f.SizeBytes, f.Bucket, f.ObjectKey, f.PasswordHash, f.ExpiresAt, f.ID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteStore) DeleteFile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

var _ Store = (*SQLiteStore)(nil)
