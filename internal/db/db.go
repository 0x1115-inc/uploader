package db

import (
	"context"
	"database/sql"
	"time"

	"uploader/internal/model"
)

type Store interface {
	CreateFile(ctx context.Context, f model.FileRecord) error
	GetFile(ctx context.Context, id string) (model.FileRecord, error)
}

type Stub struct{}

func NewStub() *Stub {
	return &Stub{}
}

func (s *Stub) CreateFile(ctx context.Context, f model.FileRecord) error {
	return nil
}

func (s *Stub) GetFile(ctx context.Context, id string) (model.FileRecord, error) {
	return model.FileRecord{
		ID:          id,
		Filename:    "example.txt",
		ContentType: "text/plain",
		SizeBytes:   1234,
		Bucket:      "stub-bucket",
		ObjectKey:   "stub/" + id + "/example.txt",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

var _ Store = (*Stub)(nil)
var _ = sql.ErrNoRows

