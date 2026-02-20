package storage

import (
	"context"
	"fmt"
	"io"
	"time"
)

type Stub struct{}

func NewStub() *Stub {
	return &Stub{}
}

func (s *Stub) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (string, error) {
	_, _ = io.Copy(io.Discard, reader)
	return "etag-stub", nil
}

func (s *Stub) GetSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return fmt.Sprintf("https://example.com/download/%s?ttl=%ds", key, int(ttl.Seconds())), nil
}

