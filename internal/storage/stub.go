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

func (s *Stub) Delete(ctx context.Context, key string) error {
	return nil
}
