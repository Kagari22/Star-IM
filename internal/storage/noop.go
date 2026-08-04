package storage

import (
	"context"
	"errors"
	"io"
)

type NoopUploader struct{}

func (NoopUploader) Upload(context.Context, string, io.Reader, int64, string) (ObjectInfo, error) {
	return ObjectInfo{}, errors.New("storage is disabled")
}

func (NoopUploader) PresignGet(context.Context, string) (string, error) {
	return "", errors.New("storage is disabled")
}
