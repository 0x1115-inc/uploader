package storage

import (
	"context"
	"io"
	"time"
)

type Provider interface {
	Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (etag string, err error)
	GetSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}

