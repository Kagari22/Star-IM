package storage

import (
	"context"
	"io"
)

type ObjectInfo struct {
	Key         string
	URL         string
	ContentType string
	Size        int64
}

type Uploader interface {
	Upload(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (ObjectInfo, error)
	PresignGet(ctx context.Context, objectName string) (string, error)
}
