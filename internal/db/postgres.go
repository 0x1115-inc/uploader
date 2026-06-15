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
	"time"

	"uploader/internal/model"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgres(databaseURL string) (*PostgresStore, error) {
	conn, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(20)
	conn.SetMaxIdleConns(5)
	conn.SetConnMaxLifetime(5 * time.Minute)

	store := &PostgresStore{db: conn}
	if err := store.migrate(context.Background()); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS files (
	id TEXT PRIMARY KEY,
	filename TEXT NOT NULL,
	content_type TEXT NOT NULL,
	size_bytes BIGINT NOT NULL,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	password_hash TEXT NOT NULL DEFAULT '',
	expires_at TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
ALTER TABLE files ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE files ADD COLUMN IF NOT EXISTS expires_at TEXT NOT NULL DEFAULT '';
ALTER TABLE files ADD COLUMN IF NOT EXISTS owner_id TEXT NOT NULL DEFAULT '';
ALTER TABLE files ADD COLUMN IF NOT EXISTS download_count BIGINT NOT NULL DEFAULT 0;
`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_files_created_at ON files(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_files_owner_id ON files(owner_id, created_at DESC);
`)
	return err
}

func (s *PostgresStore) CreateFile(ctx context.Context, f model.FileRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO files (id, filename, content_type, size_bytes, bucket, object_key, password_hash, expires_at, created_at, owner_id, download_count)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, f.ID, f.Filename, f.ContentType, f.SizeBytes, f.Bucket, f.ObjectKey, f.PasswordHash, f.ExpiresAt, f.CreatedAt, f.OwnerID, f.DownloadCount)
	return err
}

func (s *PostgresStore) GetFile(ctx context.Context, id string) (model.FileRecord, error) {
	var f model.FileRecord
	err := s.db.QueryRowContext(ctx, `
SELECT id, filename, content_type, size_bytes, bucket, object_key, password_hash, expires_at, created_at, owner_id, download_count
FROM files
WHERE id = $1
`, id).Scan(&f.ID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Bucket, &f.ObjectKey, &f.PasswordHash, &f.ExpiresAt, &f.CreatedAt, &f.OwnerID, &f.DownloadCount)
	if err != nil {
		return model.FileRecord{}, err
	}
	return f, nil
}

func (s *PostgresStore) ListFiles(ctx context.Context, limit, offset int) ([]model.FileRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, filename, content_type, size_bytes, bucket, object_key, password_hash, expires_at, created_at, owner_id, download_count
FROM files
ORDER BY created_at DESC
LIMIT $1 OFFSET $2
`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]model.FileRecord, 0, limit)
	for rows.Next() {
		var f model.FileRecord
		if err := rows.Scan(&f.ID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Bucket, &f.ObjectKey, &f.PasswordHash, &f.ExpiresAt, &f.CreatedAt, &f.OwnerID, &f.DownloadCount); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func (s *PostgresStore) ListFilesByOwner(ctx context.Context, ownerID string, limit, offset int) ([]model.FileRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, filename, content_type, size_bytes, bucket, object_key, password_hash, expires_at, created_at, owner_id, download_count
FROM files
WHERE owner_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3
`, ownerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := make([]model.FileRecord, 0, limit)
	for rows.Next() {
		var f model.FileRecord
		if err := rows.Scan(&f.ID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Bucket, &f.ObjectKey, &f.PasswordHash, &f.ExpiresAt, &f.CreatedAt, &f.OwnerID, &f.DownloadCount); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func (s *PostgresStore) UpdateFile(ctx context.Context, f model.FileRecord) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE files
SET filename = $1, content_type = $2, size_bytes = $3, bucket = $4, object_key = $5, password_hash = $6, expires_at = $7
WHERE id = $8
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

func (s *PostgresStore) DeleteFile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id = $1`, id)
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

func (s *PostgresStore) IncrementDownloadCount(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE files SET download_count = download_count + 1 WHERE id = $1`, id)
	return err
}

var _ Store = (*PostgresStore)(nil)
