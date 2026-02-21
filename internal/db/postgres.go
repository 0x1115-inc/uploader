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
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_files_created_at ON files(created_at DESC);
`)
	return err
}

func (s *PostgresStore) CreateFile(ctx context.Context, f model.FileRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO files (id, filename, content_type, size_bytes, bucket, object_key, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, f.ID, f.Filename, f.ContentType, f.SizeBytes, f.Bucket, f.ObjectKey, f.CreatedAt)
	return err
}

func (s *PostgresStore) GetFile(ctx context.Context, id string) (model.FileRecord, error) {
	var f model.FileRecord
	err := s.db.QueryRowContext(ctx, `
SELECT id, filename, content_type, size_bytes, bucket, object_key, created_at
FROM files
WHERE id = $1
`, id).Scan(&f.ID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Bucket, &f.ObjectKey, &f.CreatedAt)
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
SELECT id, filename, content_type, size_bytes, bucket, object_key, created_at
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
		if err := rows.Scan(&f.ID, &f.Filename, &f.ContentType, &f.SizeBytes, &f.Bucket, &f.ObjectKey, &f.CreatedAt); err != nil {
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
SET filename = $1, content_type = $2, size_bytes = $3, bucket = $4, object_key = $5
WHERE id = $6
`, f.Filename, f.ContentType, f.SizeBytes, f.Bucket, f.ObjectKey, f.ID)
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

var _ Store = (*PostgresStore)(nil)
