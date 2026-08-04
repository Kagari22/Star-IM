package minio

import (
	"context"
	"io"
	"net/url"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"IM_Chat_System/internal/storage"
)

type Uploader struct {
	client *miniogo.Client
	bucket string
	urlTTL time.Duration
}

// 初始化 MinIO 客户端并准备 Bucket
// 创建 MinIO Client
// → 检查 Bucket 是否存在
// → 不存在则创建 Bucket
// → 设置私有 Bucket 策略
// → 返回 Uploader
func New(endpoint, accessKey, secretKey, bucket string, useSSL bool, urlTTL time.Duration) (*Uploader, error) {
	client, err := miniogo.New(endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}

	u := &Uploader{
		client: client,
		bucket: bucket,
		urlTTL: urlTTL,
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, miniogo.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	if err := client.SetBucketPolicy(ctx, bucket, privateBucketPolicy); err != nil {
		return nil, err
	}
	return u, nil
}

// 将文件流上传到 MinIO
func (u *Uploader) Upload(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (storage.ObjectInfo, error) {
	info, err := u.client.PutObject(ctx, u.bucket, objectName, reader, size, miniogo.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return storage.ObjectInfo{}, err
	}

	return storage.ObjectInfo{
		Key:         objectName,
		ContentType: contentType,
		Size:        info.Size,
	}, nil
}

// 为私有对象生成临时下载/访问 URL
func (u *Uploader) PresignGet(ctx context.Context, objectName string) (string, error) {
	presigned, err := u.client.PresignedGetObject(ctx, u.bucket, objectName, u.urlTTL, url.Values{})
	if err != nil {
		return "", err
	}
	return presigned.String(), nil
}

const privateBucketPolicy = `{"Version":"2012-10-17","Statement":[]}`
